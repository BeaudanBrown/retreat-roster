package db

import (
	"errors"
	"log"
	"sort"
	"strings"
	"time"

	"roster/cmd/utils"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const CONFIG_REFRESH_TIME = time.Hour * 1

type StaffMember struct {
	ID            uuid.UUID
	IsAdmin       bool
	IsTrial       bool
	IsHidden      bool
	IsKitchen     bool
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
	Tokens        []uuid.UUID
	LeaveRequests []LeaveRequest
	Config        StaffConfig
}

type StaffConfig struct {
	LastVisit          time.Time
	TimesheetStartDate time.Time
	RosterStartDate    time.Time
	HideByIdeal        bool
	HideByPrefs        bool
	HideByLeave        bool
	HideApproved       bool
	ShowAll            bool
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

func (cd *CustomDate) UnmarshalJSON(input []byte) error {
	strInput := strings.Trim(string(input), `"`)
	// Try parsing the date in the expected formats
	formats := []string{
		"02/01/2006",
		"2006-01-02",
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
	// Marshall as UTC
	year, month, day := aux.Config.RosterStartDate.Date()
	rosterStartDateLocal := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	aux.Config.RosterStartDate = rosterStartDateLocal.UTC()

	year, month, day = aux.Config.TimesheetStartDate.Date()
	timesheetStartDateLocal := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	aux.Config.TimesheetStartDate = timesheetStartDateLocal.UTC()
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

	// Unmarshal in this locale
	s.Config.RosterStartDate = s.Config.RosterStartDate.In(time.Local)
	s.Config.TimesheetStartDate = s.Config.TimesheetStartDate.In(time.Local)

	return nil
}

func (d *Database) SaveStaffMembers(staffMembers []*StaffMember) error {
	collection := d.DB.Collection("staff")
	bulkWriteModels := make([]mongo.WriteModel, len(staffMembers))
	for i, staffMember := range staffMembers {
		filter := bson.M{"id": staffMember.ID}
		update := bson.M{"$set": *staffMember}
		bulkWriteModels[i] = mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update).SetUpsert(true)
	}

	opts := options.BulkWrite().SetOrdered(false)
	results, err := collection.BulkWrite(d.Context, bulkWriteModels, opts)
	if err != nil {
		log.Printf("Failed to save staff members: %v\n", err)
		return err
	}
	log.Printf("Saved %v staff members, Upserted %v staff members", results.ModifiedCount, results.UpsertedCount)
	return nil
}

func (d *Database) RefreshStaffConfig(staffMember StaffMember) StaffMember {
	if time.Now().Sub(staffMember.Config.LastVisit) > CONFIG_REFRESH_TIME {
		// Reset staff config
		staffMember.Config.TimesheetStartDate = utils.GetLastTuesday()
		staffMember.Config.RosterStartDate = utils.GetNextTuesday()
	}
	staffMember.Config.LastVisit = time.Now()
	collection := d.DB.Collection("staff")
	filter := bson.M{"id": staffMember.ID}
	update := bson.M{"$set": staffMember}
	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(d.Context, filter, update, opts)
	if err != nil {
		log.Println("Failed to save staffMember")
	} else {
		log.Println("Staff config refreshed")
	}
	return staffMember
}

func (d *Database) SaveStaffMember(staffMember StaffMember) error {
	collection := d.DB.Collection("staff")
	filter := bson.M{"id": staffMember.ID}
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

func (d *Database) LoadAllStaff() []*StaffMember {
	collection := d.DB.Collection("staff")
	cursor, err := collection.Find(d.Context, bson.M{})
	if err != nil {
		log.Printf("Error executing query: %v", err)
		return []*StaffMember{}
	}
	defer cursor.Close(d.Context)

	allStaff := []*StaffMember{}

	for cursor.Next(d.Context) {
		var staffMember StaffMember
		if err := cursor.Decode(&staffMember); err != nil {
			log.Printf("Error loading staff state: %v", err)
		}
		if staffMember.FirstName != "" {
			allStaff = append(allStaff, &staffMember)
		}
	}
	sort.Slice(allStaff, func(i, j int) bool {
		name1 := allStaff[i].FirstName
		if allStaff[i].NickName != "" {
			name1 = allStaff[i].NickName
		}
		name2 := allStaff[j].FirstName
		if allStaff[j].NickName != "" {
			name2 = allStaff[j].NickName
		}
		return name1 < name2
	})
	return allStaff
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
	filter := bson.M{"tokens": bson.M{"$elemMatch": bson.M{"$eq": token}}}
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

func (staff *StaffMember) GetConflict(slot string, offset int) Highlight {
	if !staff.Availability[offset].Early &&
		!staff.Availability[offset].Mid &&
		!staff.Availability[offset].Late {
		return PrefRefuse
	}
	switch slot {
	case "Early":
		if !staff.Availability[offset].Early {
			return PrefConflict
		}
	case "Mid":
		if !staff.Availability[offset].Mid {
			return PrefConflict
		}
	case "Late":
		if !staff.Availability[offset].Late {
			return PrefConflict
		}
	}
	return None
}

func (staff *StaffMember) IsAway(date time.Time) bool {
	for _, req := range staff.LeaveRequests {
		if !req.StartDate.After(date) && req.EndDate.After(date) {
			return true
		}
	}
	return false
}

func AddToken(slice []uuid.UUID, value uuid.UUID) []uuid.UUID {
	for _, v := range slice {
		if v == value {
			// Value already exists, return the original slice
			return slice
		}
	}
	// Value does not exist, append it to the slice
	return append(slice, value)
}

func (d *Database) UpdateStaffToken(staffMember *StaffMember, token uuid.UUID) error {
	staffMember.Tokens = AddToken(staffMember.Tokens, token)
	err := d.SaveStaffMember(*staffMember)
	if err != nil {
		return err
	}
	return nil
}

func (d *Database) CreateStaffMember(googleId string, token uuid.UUID) error {
	staffMember := d.GetStaffByGoogleID(googleId)
	if staffMember != nil {
		return errors.New("Staff exists")
	}
	isAdmin := len(d.LoadAllStaff()) == 0
	staffMember = &StaffMember{
		ID:           uuid.New(),
		GoogleID:     googleId,
		FirstName:    "",
		IsAdmin:      isAdmin,
		Tokens:       []uuid.UUID{token},
		Availability: emptyAvailability,
		Config: StaffConfig{
			LastVisit:          time.Now(),
			TimesheetStartDate: utils.GetLastTuesday(),
			RosterStartDate:    utils.GetNextTuesday(),
		},
	}
	err := d.SaveStaffMember(*staffMember)
	if err != nil {
		return err
	}
	return nil
}

func GetStaffFromList(staffID uuid.UUID, allStaff []*StaffMember) *StaffMember {
	for _, staff := range allStaff {
		if staff.ID == staffID {
			return staff
		}
	}
	return nil
}

func (d *Database) DeleteLeaveReqByID(staffMember StaffMember, leaveReqID uuid.UUID) {
	temp := []LeaveRequest{}
	for _, leaveReq := range staffMember.LeaveRequests {
		if leaveReq.ID == leaveReqID {
			continue
		}
		temp = append(temp, leaveReq)
	}
	staffMember.LeaveRequests = temp
	err := d.SaveStaffMember(staffMember)
	if err != nil {
		log.Printf("Error deleting leave request: %v", err)
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
		IdealShifts:  7,
	}
	err := d.SaveStaffMember(newStaff)
	if err != nil {
		log.Printf("")
	}
}
