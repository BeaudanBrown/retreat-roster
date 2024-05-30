package db

import (
	"log"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type TimesheetEntry struct {
	ID          uuid.UUID
	StaffID     uuid.UUID      `bson:"days"`
	StartDate   time.Time      `bson:"startDate"`
	ShiftStart  *time.Time     `json:"shiftStart"`
	ShiftEnd    *time.Time     `json:"shiftEnd"`
	BreakStart  *time.Time     `json:"breakStart"`
	BreakEnd    *time.Time     `json:"breakEnd"`
	BreakLength float64        `json:"breakLength"`
	ShiftLength float64        `json:"shiftLength"`
	Status      ApprovalStatus `json:"status"`
	ShiftType   ShiftType      `json:"shiftType"`
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
	if s.ShiftStart != nil {
		shiftStartWithLoc := s.ShiftStart.In(time.Now().Location())
		s.ShiftStart = &shiftStartWithLoc
	}
	if s.ShiftEnd != nil {
		shiftEndWithLoc := s.ShiftEnd.In(time.Now().Location())
		s.ShiftEnd = &shiftEndWithLoc
	}
	if s.BreakStart != nil {
		breakStartWithLoc := s.BreakStart.In(time.Now().Location())
		s.BreakStart = &breakStartWithLoc
	}
	if s.BreakEnd != nil {
		breakEndWithLoc := s.BreakEnd.In(time.Now().Location())
		s.BreakEnd = &breakEndWithLoc
	}
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
	DayManager
	NightManager
	Admin
)

func ShiftTypeToString(shiftType ShiftType) string {
	switch shiftType {
	case Bar:
		return "Bar"
	case DayManager:
		return "Day Manager"
	case NightManager:
		return "Night Manager"
	case Admin:
		return "Admin"
	default:
		return "Bar"
	}
}

func StringToShiftType(typeStr string) ShiftType {
	switch typeStr {
	case "0":
		return Bar
	case "1":
		return DayManager
	case "2":
		return NightManager
	case "3":
		return Admin
	default:
		return Bar
	}
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

func (d *Database) CreateTimesheetEntry(startDate time.Time, staffID uuid.UUID) error {
	collection := d.DB.Collection("timesheets")
	year, month, day := startDate.Date()
	dateOnly := time.Date(year, month, day, 0, 0, 0, 0, time.Now().Location())
	newEntry := TimesheetEntry{
		ID:        uuid.New(),
		StaffID:   staffID,
		StartDate: dateOnly,
	}
	_, err := collection.InsertOne(d.Context, newEntry)
	if err != nil {
		return err
	}
	return nil
}
