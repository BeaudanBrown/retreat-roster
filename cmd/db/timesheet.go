package db

import (
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type TimesheetEntry struct {
	ID          uuid.UUID
	StaffID     uuid.UUID `bson:"days"`
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

func (s TimesheetEntry) MarshalBSON() ([]byte, error) {
	type Alias TimesheetEntry
	aux := &struct {
		*Alias `bson:",inline"`
	}{
		Alias: (*Alias)(&s),
	}
	year, month, day := aux.StartDate.Date()
	aux.StartDate = time.Date(year, month, day, 0, 0, 0, 0, aux.StartDate.Location())
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

	s.StartDate = s.StartDate.In(time.Now().Location())
	s.ShiftStart = s.ShiftStart.In(time.Now().Location())
	s.ShiftEnd = s.ShiftEnd.In(time.Now().Location())
	s.BreakStart = s.BreakStart.In(time.Now().Location())
	s.BreakEnd = s.BreakEnd.In(time.Now().Location())
	return nil
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
	Admin
)

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
		"Admin"}[s]
}

func StringToShiftType(typeStr string) ShiftType {
	num, err := strconv.Atoi(typeStr)
	if err != nil {
		log.Printf("Error converting shift type: %v", err)
		return Bar
	}
	if num > int(Admin) {
		return Bar
	}
	return ShiftType(num)
}

func (d *Database) SaveTimesheetEntry(e TimesheetEntry) error {
	collection := d.DB.Collection("timesheets")
	filter := bson.M{"id": e.ID}
	update := bson.M{"$set": e}
	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(d.Context, filter, update, opts)
	if err != nil {
		log.Println("Failed to save timesheet entry")
		return err
	}
	log.Println("Saved timesheet entry")
	return nil
}

func (d *Database) GetTimesheetEntryByID(entryID uuid.UUID) *TimesheetEntry {
	collection := d.DB.Collection("timesheets")
	filter := bson.M{"id": entryID}

	var timesheetEntry TimesheetEntry
	err := collection.FindOne(d.Context, filter).Decode(&timesheetEntry)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("No timesheet entry with id: %v", err)
			return nil
		}
		log.Printf("Error getting timesheet entry: %v", err)
		return nil
	}
	return &timesheetEntry
}

func (d *Database) GetTimesheetWeek(startDate time.Time) *[]TimesheetEntry {
	collection := d.DB.Collection("timesheets")
	year, month, day := startDate.Date()
	weekStart := time.Date(year, month, day, 0, 0, 0, 0, time.Now().Location())
	weekEnd := weekStart.AddDate(0, 0, 7)
	filter := bson.M{
		"startDate": bson.M{
			"$gte": weekStart,
			"$lt":  weekEnd,
		},
	}

	cursor, err := collection.Find(d.Context, filter)
	if err != nil {
		log.Printf("Error finding timesheet week: %v", err)
		return nil
	}
	defer cursor.Close(d.Context)
	var entries []TimesheetEntry
	if err = cursor.All(d.Context, &entries); err != nil {
		log.Printf("Error decoding timesheet weeks: %v", err)
		return nil
	}
	return &entries
}

func (d *Database) DeleteTimesheetEntry(entryID uuid.UUID) error {
	collection := d.DB.Collection("timesheets")

	filter := bson.M{"id": entryID}
	_, err := collection.DeleteOne(d.Context, filter)
	if err != nil {
		return err
	}
	return nil
}

func LastWholeHour() time.Time {
	t := time.Now()
	return t.Truncate(time.Hour)
}

func NextWholeHour() time.Time {
	t := time.Now()
	return t.Truncate(time.Hour).Add(time.Hour)
}

func (d *Database) CreateTimesheetEntry(startDate time.Time, staffID uuid.UUID) (*TimesheetEntry, error) {
	collection := d.DB.Collection("timesheets")
	year, month, day := startDate.Date()
	dateOnly := time.Date(year, month, day, 0, 0, 0, 0, time.Now().Location())
	start := LastWholeHour()
	end := NextWholeHour()
	newEntry := TimesheetEntry{
		ID:          uuid.New(),
		StaffID:     staffID,
		StartDate:   dateOnly,
		ShiftStart:  start,
		ShiftEnd:    end,
		ShiftLength: end.Sub(start).Hours(),
	}
	_, err := collection.InsertOne(d.Context, newEntry)
	if err != nil {
		return nil, err
	}
	return &newEntry, nil
}
