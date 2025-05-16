package models

import (
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// Domain entities for roster weeks, days, rows, and slots.
// Any custom JSON/BSON marshalling that is domain–specific is located here.

type RosterWeek struct {
	ID         uuid.UUID    `bson:"id"`
	StartDate  time.Time    `bson:"startDate"`
	WeekOffset int          `bson:"weekOffset"`
	Days       []*RosterDay `bson:"days"`
	IsLive     bool         `bson:"isLive"`
}

type RosterDay struct {
	ID         uuid.UUID
	DayName    string
	Rows       []*Row
	Colour     string
	Offset     int
	IsClosed   bool
	AmeliaOpen bool
}

type Row struct {
	ID     uuid.UUID
	Amelia Slot
	Early  Slot
	Mid    Slot
	Late   Slot
}

func (r Row) GetSlot(slotName string) *Slot {
	switch slotName {
	case "Amelia":
		return &r.Amelia
	case "Early":
		return &r.Early
	case "Mid":
		return &r.Mid
	case "Late":
		return &r.Late
	default:
		return nil
	}
}

type Slot struct {
	ID            uuid.UUID
	StartTime     string
	AssignedStaff *uuid.UUID
	StaffString   *string
	Flag          Highlight
	Description   string
}

type Highlight int

const (
	None Highlight = iota
	IdealMet
	IdealExceeded
	PrefConflict
	LateToEarly
	PrefRefuse
	Duplicate
	LeaveConflict
)

func (rw RosterWeek) MarshalBSON() ([]byte, error) {
	type Alias RosterWeek
	aux := &struct {
		*Alias `bson:",inline"`
	}{
		Alias: (*Alias)(&rw),
	}
	year, month, day := aux.StartDate.Date()
	startDateLocal := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	aux.StartDate = startDateLocal.UTC()
	return bson.Marshal(aux)
}

func (rw *RosterWeek) UnmarshalBSON(data []byte) error {
	type Alias RosterWeek
	aux := &struct {
		*Alias `bson:",inline"`
	}{
		Alias: (*Alias)(rw),
	}
	if err := bson.Unmarshal(data, aux); err != nil {
		return err
	}
	aux.StartDate = aux.StartDate.In(time.Local)
	return nil
}

func (week *RosterWeek) GetSlotByID(slotID uuid.UUID) *Slot {
	for _, day := range week.Days {
		for j := range day.Rows {
			row := day.Rows[j]
			if row.Amelia.ID == slotID {
				return &row.Amelia
			}
			if row.Early.ID == slotID {
				return &row.Early
			}
			if row.Mid.ID == slotID {
				return &row.Mid
			}
			if row.Late.ID == slotID {
				return &row.Late
			}
		}
	}
	return nil
}

func (week *RosterWeek) GetDayByID(dayID uuid.UUID) *RosterDay {
	for _, day := range week.Days {
		if day.ID == dayID {
			return day
		}
	}
	return nil
}

func (s *Slot) HasThisStaff(staffId uuid.UUID) bool {
	if s.AssignedStaff != nil && *s.AssignedStaff == staffId {
		return true
	}
	return false
}

func GetHighlightCol(defaultCol string, flag Highlight) string {
	if flag == Duplicate {
		return "#FFA07A"
	}
	if flag == PrefConflict {
		return "#FF9999"
	}
	if flag == LateToEarly {
		return "#117593"
	}
	if flag == LeaveConflict || flag == PrefRefuse {
		return "#CC3333"
	}
	if flag == IdealMet {
		return "#B2E1B0"
	}
	if flag == IdealExceeded {
		return "#D7A9A9"
	}
	return defaultCol
}
