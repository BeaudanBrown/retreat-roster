package models

import (
	"encoding/json"
	"roster/cmd/utils"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// StaffMember represents a staff member in your application.
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
	IsDeleted     bool
}

// StaffConfig defines configuration data for staff.
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

// DayAvailability represents a staff member's availability on a given day.
type DayAvailability struct {
	Name  string
	Early bool
	Mid   bool
	Late  bool
}

// LeaveRequest defines a leave request.
type LeaveRequest struct {
	ID           uuid.UUID
	CreationDate CustomDate
	Reason       string     `json:"reason"`
	StartDate    CustomDate `json:"start-date"`
	EndDate      CustomDate `json:"end-date"`
}

// CustomDate wraps *time.Time to support custom unmarshalling.
type CustomDate struct {
	*time.Time
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

	utcRequests := []LeaveRequest{}
	for _, leaveReq := range s.LeaveRequests {
		year, month, day = leaveReq.StartDate.Date()
		leaveStartDateLocal := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
		utcStart := leaveStartDateLocal.UTC()

		year, month, day = leaveReq.EndDate.Date()
		leaveEndDateLocal := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
		utcEnd := leaveEndDateLocal.UTC()

		year, month, day = leaveReq.CreationDate.Date()
		leaveCreationDateLocal := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
		utcCreation := leaveCreationDateLocal.UTC()

		utcReq := LeaveRequest{
			ID:           leaveReq.ID,
			StartDate:    CustomDate{&utcStart},
			EndDate:      CustomDate{&utcEnd},
			Reason:       leaveReq.Reason,
			CreationDate: CustomDate{&utcCreation},
		}
		utcRequests = append(utcRequests, utcReq)
	}
	aux.LeaveRequests = utcRequests

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
	localRequests := []LeaveRequest{}
	for _, leaveReq := range s.LeaveRequests {
		localStart := leaveReq.StartDate.In(time.Local)
		localEnd := leaveReq.EndDate.In(time.Local)
		localCreation := leaveReq.CreationDate.In(time.Local)
		localReq := LeaveRequest{
			ID:           leaveReq.ID,
			StartDate:    CustomDate{&localStart},
			EndDate:      CustomDate{&localEnd},
			Reason:       leaveReq.Reason,
			CreationDate: CustomDate{&localCreation},
		}
		localRequests = append(localRequests, localReq)
	}
	s.LeaveRequests = localRequests
	s.Config.RosterStartDate = s.Config.RosterStartDate.In(time.Local)
	s.Config.TimesheetStartDate = s.Config.TimesheetStartDate.In(time.Local)

	return nil
}

// UnmarshalJSON implements custom JSON unmarshalling for CustomDate.
func (cd *CustomDate) UnmarshalJSON(input []byte) error {
	// Unmarshal into a string first.
	var strInput string
	if err := json.Unmarshal(input, &strInput); err != nil {
		return err
	}
	// Try several expected date formats.
	formats := []string{
		"02/01/2006",
		"2006-01-02",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"15:04",
		"03:04 PM",
	}
	var parseErr error
	for _, format := range formats {
		if newTime, err := time.Parse(format, strInput); err == nil {
			cd.Time = &newTime
			return nil
		} else {
			parseErr = err
		}
	}
	utils.PrintError(parseErr, "Invalid time in CustomDate")
	cd.Time = nil
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
