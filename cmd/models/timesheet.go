package models

import (
	"roster/cmd/utils"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

type TimesheetEntry struct {
	ID uuid.UUID
	// TODO: Fucked up the name
	StaffID    uuid.UUID `bson:"days"`
	WeekOffset int       `bson:"weekOffset"`
	DayOffset  int       `bson:"dayOffset"`

	StartDate   time.Time `bson:"startDate"`
	ShiftStart  time.Time `json:"shiftStart"`
	ShiftEnd    time.Time `json:"shiftEnd"`
	HasBreak    bool      `json:"hasBreak"`
	BreakStart  time.Time `json:"breakStart"`
	BreakEnd    time.Time `json:"breakEnd"`
	BreakLength float64   `json:"breakLength"`
	ShiftLength float64   `json:"shiftLength"`
	Approved    bool      `json:"approved"`
	ShiftType   ShiftType `json:"shiftType"`
}

type ApprovalStatus int

const (
	Incomplete ApprovalStatus = iota
	Complete
	Approved
)

type ShiftType int

const (
	Bar ShiftType = iota
	Deliveries
	DayManager
	AmeliaSupervisor
	NightManager
	GeneralManagement
	Kitchen
	Admin
)

func (s TimesheetEntry) MarshalBSON() ([]byte, error) {
	type Alias TimesheetEntry
	aux := &struct {
		*Alias `bson:",inline"`
	}{
		Alias: (*Alias)(&s),
	}
	year, month, day := aux.StartDate.Date()
	// Marshall as UTC
	startDateLocal := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	aux.StartDate = startDateLocal.UTC()

	aux.ShiftStart = s.ShiftStart.UTC()
	aux.ShiftEnd = s.ShiftEnd.UTC()
	aux.BreakStart = s.BreakStart.UTC()
	aux.BreakEnd = s.BreakEnd.UTC()

	return bson.Marshal(aux)
}

func (s *TimesheetEntry) UnmarshalBSON(data []byte) error {
	type Alias TimesheetEntry
	aux := &struct {
		*Alias `bson:",inline"`
	}{
		Alias: (*Alias)(s),
	}

	if err := bson.Unmarshal(data, aux); err != nil {
		return err
	}

	s.StartDate = s.StartDate.In(time.Local)
	s.ShiftStart = s.ShiftStart.In(time.Local)
	s.ShiftEnd = s.ShiftEnd.In(time.Local)
	s.BreakStart = s.BreakStart.In(time.Local)
	s.BreakEnd = s.BreakEnd.In(time.Local)
	return nil
}

func GetAllShiftTypes() []ShiftType {
	size := int(Admin-Bar) + 1
	shiftTypes := make([]ShiftType, size)
	for i := int(Bar); i < size; i++ {
		shiftTypes[i] = ShiftType(i)
	}
	return shiftTypes
}

func (s ShiftType) Int() int {
	return int(s)
}

func (s ShiftType) String() string {
	return [...]string{
		"Bar",
		"Deliveries",
		"Day Manager",
		"Amelia Supervisor",
		"Night Manager",
		"General Management",
		"Kitchen",
		"Admin"}[s]
}

func StringToShiftType(typeStr string) ShiftType {
	num, err := strconv.Atoi(typeStr)
	if err != nil {
		utils.PrintError(err, "Error converting shift type")
		return Bar
	}
	if num > int(Admin) {
		return Bar
	}
	return ShiftType(num)
}

func DisableTimesheet(timesheetDate time.Time, isAdmin bool) bool {
	lastTuesday := utils.GetLastTuesday().Add(-time.Minute) // Inclusive
	now := time.Now()
	if now.Sub(lastTuesday).Hours() < 12 {
		// 12 hour overlap between weeks
		lastTuesday = lastTuesday.AddDate(0, 0, -7)
	}
	tomorrow := time.Date(
		now.Year(),
		now.Month(),
		now.Day()+1,
		0, 0, 0, 0,
		time.Local)
	if now.Hour() < 8 {
		// early morning shift date is the day before
		tomorrow = tomorrow.AddDate(0, 0, -1)
	}
	if timesheetDate.After(lastTuesday) && timesheetDate.Before(tomorrow) {
		return false
	}
	return !isAdmin
}
