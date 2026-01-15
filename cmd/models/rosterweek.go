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
	ID       uuid.UUID
	DayName  string
	Rows     []*Row
	Colour   string
	Offset   int
	IsClosed bool
}

type Row struct {
	ID    uuid.UUID
	Early Slot
	Mid   Slot
	Late  Slot
}

func (r Row) GetSlot(slotName string) *Slot {
	switch slotName {
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

func GetHighlightDesc(flag Highlight) string {
	switch flag {
	case Duplicate:
		return "Duplicate shifts on this day"
	case PrefConflict:
		return "Conflict with staff preference"
	case LateToEarly:
		return "Late shift followed by early shift"
	case LeaveConflict:
		return "Conflict with leave request"
	case PrefRefuse:
		return "Staff refused this shift preference"
	case IdealMet:
		return "Ideal number of shifts met"
	case IdealExceeded:
		return "Ideal number of shifts exceeded"
	default:
		return ""
	}
}

func (week *RosterWeek) CheckFlags(allStaff []*StaffMember) RosterWeek {
	staffMap := make(map[uuid.UUID]*StaffMember, len(allStaff))
	shiftCounts := make(map[uuid.UUID][]int, len(allStaff))

	for _, staff := range allStaff {
		staffMap[staff.ID] = staff
		shiftCounts[staff.ID] = make([]int, 7)
	}

	for _, day := range week.Days {
		if day != nil {
			day.CountShifts(shiftCounts)
		}
	}

	// Pre-calculate total weekly shifts for each staff member
	weeklyShiftTotals := make(map[uuid.UUID]int, len(shiftCounts))
	for staffID, dailyCounts := range shiftCounts {
		weeklyShiftTotals[staffID] = SumArray(dailyCounts)
	}

	for i := range week.Days {
		if week.Days[i] != nil {
			assignFlags(week.Days[i], week.StartDate.AddDate(0, 0, i), shiftCounts, weeklyShiftTotals, staffMap, week.Days[i].Offset)
		}
	}

	for i := 0; i < len(week.Days)-1; i++ {
		currentDay := week.Days[i]
		nextDay := week.Days[i+1]
		if currentDay.IsClosed || nextDay.IsClosed {
			continue
		}
		checkLateToEarly(currentDay, nextDay)
	}
	return *week
}

func (day *RosterDay) CountShifts(shiftCounts map[uuid.UUID][]int) {
	recordShifts := func(slot *Slot) {
		if slot != nil && slot.AssignedStaff != nil {
			staffID := *slot.AssignedStaff
			if dailyCounts, ok := shiftCounts[staffID]; ok {
				dailyCounts[day.Offset]++
			}
		}
	}

	for _, row := range day.Rows {
		if row == nil {
			continue
		}
		recordShifts(&row.Early)
		recordShifts(&row.Mid)
		recordShifts(&row.Late)
	}
}

func SumArray(arr []int) int {
	total := 0
	for _, value := range arr {
		total += value
	}
	return total
}

func assignFlags(day *RosterDay, date time.Time, shiftCounts map[uuid.UUID][]int, weeklyShiftTotals map[uuid.UUID]int, staffMap map[uuid.UUID]*StaffMember, dayIdx int) {
	processSlot := func(row *Row, slotStr string, dayIndex int) Highlight {
		slot := row.GetSlot(slotStr)
		if slot == nil || slot.AssignedStaff == nil {
			return None
		}
		staffID := *slot.AssignedStaff

		if dailyCounts, ok := shiftCounts[staffID]; ok {
			if dayIndex >= 0 && dayIndex < len(dailyCounts) && dailyCounts[dayIndex] > 1 {
				return Duplicate
			}
		}
		if staff, ok := staffMap[staffID]; ok {
			for _, req := range staff.LeaveRequests {
				if req.Status != LeaveApproved {
					continue
				}
				if !req.StartDate.After(date) && req.EndDate.After(date) {
					return LeaveConflict
				}
			}
			conflict := staff.GetConflict(slotStr, dayIndex)
			if conflict != None {
				return conflict
			}
			currentShifts := weeklyShiftTotals[staffID]
			if currentShifts == staff.IdealShifts {
				//TODO: move this to a better place for viewing
				// return IdealMet
				return None
			}
			if currentShifts > staff.IdealShifts {
				return IdealExceeded
			}
		}
		return None
	}

	for _, row := range day.Rows {
		if row == nil {
			continue
		} // Added nil check for row
		row.Early.Flag = processSlot(row, "Early", day.Offset)
		row.Mid.Flag = processSlot(row, "Mid", day.Offset)
		row.Late.Flag = processSlot(row, "Late", day.Offset)
	}
}

func checkLateToEarly(day *RosterDay, nextDay *RosterDay) {
	for _, row := range day.Rows {
		if row.Late.Flag > LateToEarly {
			// Don't overwrite more important flags
			continue
		}
		staffID := row.Late.AssignedStaff
		if staffID == nil {
			continue
		}
		for _, row2 := range nextDay.Rows {
			if row2.Early.Flag > LateToEarly {
				// Don't overwrite more important flags
				continue
			}
			if row2.Early.HasThisStaff(*staffID) {
				row2.Early.Flag = LateToEarly
				row.Late.Flag = LateToEarly
			}
		}
	}
}

// CountShiftsForStaff returns the total number of shifts assigned to a specific staff member in this roster week.
func (week *RosterWeek) CountShiftsForStaff(staffID uuid.UUID) int {
	totalShifts := 0
	for _, day := range week.Days {
		if day == nil {
			continue
		}
		for _, row := range day.Rows {
			if row == nil {
				continue
			}
			// Count Early, Mid, Late shifts
			if row.Early.AssignedStaff != nil && *row.Early.AssignedStaff == staffID {
				totalShifts++
			}
			if row.Mid.AssignedStaff != nil && *row.Mid.AssignedStaff == staffID {
				totalShifts++
			}
			if row.Late.AssignedStaff != nil && *row.Late.AssignedStaff == staffID {
				totalShifts++
			}
		}
	}
	return totalShifts
}
