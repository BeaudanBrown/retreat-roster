package db

import (
	"log"
	"strings"
	"time"

	"roster/cmd/utils"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type StaffMember struct {
	ID            uuid.UUID
	IsAdmin       bool
	IsTrial       bool
	IsHidden      bool
	GoogleID      string
	NickName      string
	FirstName     string
	LastName      string
	Email         string
	Phone         string
	ContactName   string
	ContactPhone  string
	IdealShifts   int
	CurrentShifts int
	Availability  []DayAvailability
	Token         *uuid.UUID
	LeaveRequests []LeaveRequest
	Config        StaffConfig
}

type StaffConfig struct {
	TimesheetStartDate time.Time
	RosterStartDate    time.Time
	HideByIdeal        bool
	HideByPrefs        bool
	HideByLeave        bool
	HideApproved       bool
	ApprovalMode       bool
}

type DayAvailability struct {
	Name  string
	Early bool
	Mid   bool
	Late  bool
}

type LeaveRequest struct {
	ID           uuid.UUID
	CreationDate CustomDate
	Reason       string     `json:"reason"`
	StartDate    CustomDate `json:"start-date"`
	EndDate      CustomDate `json:"end-date"`
}

type CustomDate struct {
	*time.Time
}

type StaffState struct {
	Staff *[]*StaffMember `bson:"staff"`
}

func NewStaffState() StaffState {
	staff := []*StaffMember{}
	s := StaffState{
		Staff: &staff,
	}
	return s
}

func (cd *CustomDate) UnmarshalJSON(input []byte) error {
	strInput := strings.Trim(string(input), `"`)
	// Try parsing the date in the expected formats
	formats := []string{
		"2006-01-02",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"15:04",
		"03:04 PM",
	}
	var parseErr error
	for _, format := range formats {
		var newTime time.Time
		newTime, parseErr = time.Parse(format, strInput)
		if parseErr == nil {
			cd.Time = &newTime
			return nil
		}
	}
	log.Printf("Invalid time: %v", parseErr)
	cd.Time = nil
	return nil
}

var emptyAvailability = []DayAvailability{
	{
		Name:  "Tues",
		Early: true,
		Mid:   true,
		Late:  true,
	},
	{
		Name:  "Wed",
		Early: true,
		Mid:   true,
		Late:  true,
	},
	{
		Name:  "Thurs",
		Early: true,
		Mid:   true,
		Late:  true,
	},
	{
		Name:  "Fri",
		Early: true,
		Mid:   true,
		Late:  true,
	},
	{
		Name:  "Sat",
		Early: true,
		Mid:   true,
		Late:  true,
	},
	{
		Name:  "Sun",
		Early: true,
		Mid:   true,
		Late:  true,
	},
	{
		Name:  "Mon",
		Early: true,
		Mid:   true,
		Late:  true,
	},
}

func (s StaffMember) MarshalBSON() ([]byte, error) {
	type Alias StaffMember
	aux := &struct {
		*Alias `bson:",inline"`
	}{
		Alias: (*Alias)(&s),
	}
	year, month, day := aux.Config.RosterStartDate.Date()
	aux.Config.RosterStartDate = time.Date(year, month, day, 0, 0, 0, 0, aux.Config.RosterStartDate.Location())
	year, month, day = aux.Config.TimesheetStartDate.Date()
	aux.Config.TimesheetStartDate = time.Date(year, month, day, 0, 0, 0, 0, aux.Config.TimesheetStartDate.Location())
	return bson.Marshal(aux)
}

func (s *StaffMember) UnmarshalBSON(data []byte) error {
	type Alias StaffMember
	aux := &struct {
		*Alias `bson:",inline"`
	}{
		Alias: (*Alias)(s),
	}

	if err := bson.Unmarshal(data, aux); err != nil {
		return err
	}

	s.Config.RosterStartDate = s.Config.RosterStartDate.In(time.Now().Location())
	s.Config.TimesheetStartDate = s.Config.TimesheetStartDate.In(time.Now().Location())

	return nil
}

func (d *Database) SaveStaffMember(staffMember StaffMember) error {
	collection := d.DB.Collection("staff")
	filter := bson.M{"_id": staffMember.ID}
	update := bson.M{"$set": staffMember}
	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(d.Context, filter, update, opts)
	if err != nil {
		log.Println("Failed to save staffMember")
		return err
	}
	log.Println("Saved staff member")
	return nil
}

func (d *Database) LoadStaffState() StaffState {
	collection := d.DB.Collection("staff")
	cursor, err := collection.Find(d.Context, bson.M{})
	if err != nil {
		log.Printf("Error executing query: %v", err)
		return StaffState{}
	}
	defer cursor.Close(d.Context)

	newStaff := []*StaffMember{}

	for cursor.Next(d.Context) {
		var staffMember StaffMember
		if err := cursor.Decode(&staffMember); err != nil {
			log.Printf("Error loading staff state: %v", err)
		}
		newStaff = append(newStaff, &staffMember)
	}
	staffState := StaffState{
		&newStaff,
	}
	return staffState
}

func (d *Database) GetStaffByGoogleID(googleID string) *StaffMember {
	collection := d.DB.Collection("staff")
	filter := bson.M{"googleid": googleID}
	var staffMember StaffMember
	err := collection.FindOne(d.Context, filter).Decode(&staffMember)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("No staff with google id found: %v", err)
			return nil
		}
		log.Printf("Error getting staff by google id: %v", err)
		return nil
	}
	return &staffMember
}

func (d *Database) GetStaffByID(staffID uuid.UUID) *StaffMember {
	collection := d.DB.Collection("staff")
	filter := bson.M{"id": staffID}
	var staffMember StaffMember
	err := collection.FindOne(d.Context, filter).Decode(&staffMember)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil
		}
		return nil
	}
	return &staffMember
}

func (d *Database) GetStaffByToken(token uuid.UUID) *StaffMember {
	collection := d.DB.Collection("staff")
	filter := bson.M{"token": token}
	var staffMember StaffMember
	err := collection.FindOne(d.Context, filter).Decode(&staffMember)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil
		}
		return nil
	}
	return &staffMember
}

func (staff *StaffMember) HasConflict(slot string, offset int) bool {
	switch slot {
	case "Early":
		if !staff.Availability[offset].Early {
			return true
		}
	case "Mid":
		if !staff.Availability[offset].Mid {
			return true
		}
	case "Late":
		if !staff.Availability[offset].Late {
			return true
		}
	}
	return false
}

func (staff *StaffMember) IsAway(date time.Time) bool {
	for _, req := range staff.LeaveRequests {
		if !req.StartDate.After(date) && req.EndDate.After(date) {
			return true
		}
	}
	return false
}

func (d *Database) CreateOrUpdateStaffGoogleID(googleId string, token uuid.UUID) error {
	staffMember := d.GetStaffByGoogleID(googleId)
	if staffMember != nil {
		staffMember.Token = &token
		err := d.SaveStaffMember(*staffMember)
		if err != nil {
			return err
		}
	} else {
		isAdmin := len(*d.LoadStaffState().Staff) == 0
		staffMember = &StaffMember{
			ID:           uuid.New(),
			GoogleID:     googleId,
			FirstName:    "",
			IsAdmin:      isAdmin,
			Token:        &token,
			Availability: emptyAvailability,
			Config: StaffConfig{
				TimesheetStartDate: utils.GetLastTuesday(),
				RosterStartDate:    utils.GetNextTuesday(),
			},
		}
		err := d.SaveStaffMember(*staffMember)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Database) GetStaffMap() map[uuid.UUID]StaffMember {
	staffMap := map[uuid.UUID]StaffMember{}
	staffState := d.LoadStaffState().Staff
	for _, staff := range *staffState {
		staffMap[staff.ID] = *staff
	}
	return staffMap
}

func (d *Database) DeleteLeaveReqByID(staffMember StaffMember, leaveReqID uuid.UUID) {
	for i, leaveReq := range staffMember.LeaveRequests {
		if leaveReq.ID != leaveReqID {
			continue
		}
		staffMember.LeaveRequests = append(
			staffMember.LeaveRequests[:1],
			staffMember.LeaveRequests[i+1:]...)
		err := d.SaveStaffMember(staffMember)
		if err != nil {
			log.Printf("Error deleting leave request: %v", err)
		}
		return
	}
}

func (d *Database) GetStaffByLeaveReqID(leaveReqID uuid.UUID) *StaffMember {
	collection := d.DB.Collection("staff")
	var staffMember StaffMember
	filter := bson.M{
		"leaveRequests": bson.M{
			"$elemMatch": bson.M{
				"id": leaveReqID,
			},
		},
	}
	err := collection.FindOne(d.Context, filter).Decode(&staffMember)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil
		}
		return nil
	}
	return &staffMember
}

func (d *Database) CreateTrial(trialName string) {
	newStaff := StaffMember{
		ID:           uuid.New(),
		GoogleID:     "Trial",
		IsTrial:      true,
		FirstName:    trialName,
		Availability: emptyAvailability,
	}
	err := d.SaveStaffMember(newStaff)
	if err != nil {
		log.Printf("")
	}
}
