package db

import (
	"log"
	"roster/cmd/utils"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type TimesheetEntry struct {
	ID uuid.UUID
	// TODO: Fucked up the name
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

func (d *Database) SortTimesheetEntries(entries []*TimesheetEntry) []*TimesheetEntry {
	copiedEntries := make([]*TimesheetEntry, len(entries))
	copy(copiedEntries, entries)
	sort.Slice(copiedEntries, func(i, j int) bool {
		entry1 := copiedEntries[i]
		entry2 := copiedEntries[j]
		if entry1.ShiftType == entry2.ShiftType {
			staff1 := d.GetStaffByID(entry1.StaffID)
			staff2 := d.GetStaffByID(entry2.StaffID)
			if staff1 == nil || staff2 == nil {
				return false
			}
			name1 := staff1.FirstName
			if staff1.NickName != "" {
				name1 = staff1.NickName
			}
			name2 := staff2.FirstName
			if staff2.NickName != "" {
				name2 = staff2.NickName
			}
			return name1 < name2
		}
		return entry1.ShiftType > entry2.ShiftType
	})
	return copiedEntries
}

func (d *Database) GetAllTimesheetEntries() *[]*TimesheetEntry {
	collection := d.DB.Collection("timesheets")
	cursor, err := collection.Find(d.Context, bson.M{})
	if err != nil {
		log.Printf("Error executing query: %v", err)
		return nil
	}
	defer cursor.Close(d.Context)
	var entries []*TimesheetEntry
	if err = cursor.All(d.Context, &entries); err != nil {
		log.Printf("Error decoding timesheet entries: %v", err)
		return nil
	}
	entries = d.SortTimesheetEntries(entries)
	return &entries
}

func (d *Database) SaveAllTimesheetEntries(entries []*TimesheetEntry) error {
	collection := d.DB.Collection("timesheets")
	bulkWriteModels := make([]mongo.WriteModel, len(entries))
	for i, entry := range entries {
		filter := bson.M{"id": entry.ID}
		update := bson.M{"$set": *entry}
		bulkWriteModels[i] = mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update).SetUpsert(true)
	}

	opts := options.BulkWrite().SetOrdered(false)
	results, err := collection.BulkWrite(d.Context, bulkWriteModels, opts)
	if err != nil {
		log.Printf("Failed to save timesheet entries: %v\n", err)
		return err
	}
	log.Printf("Saved %v timesheet entries, Upserted %v timesheet entries", results.ModifiedCount, results.UpsertedCount)
	return nil
}

func (d *Database) GetStaffTimesheetWeek(staffID uuid.UUID, startDate time.Time) *[]*TimesheetEntry {
	collection := d.DB.Collection("timesheets")
	year, month, day := startDate.Date()
	weekStart := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	weekEnd := weekStart.AddDate(0, 0, 7)
	filter := bson.M{
		"startDate": bson.M{
			"$gte": weekStart.UTC(),
			"$lt":  weekEnd.UTC(),
		},
		// TODO: Fucked up the bson name
		"days": staffID,
	}

	cursor, err := collection.Find(d.Context, filter)
	if err != nil {
		log.Printf("Error finding timesheet week: %v", err)
		return nil
	}
	defer cursor.Close(d.Context)
	var entries []*TimesheetEntry
	if err = cursor.All(d.Context, &entries); err != nil {
		log.Printf("Error decoding timesheet weeks: %v", err)
		return nil
	}
	return &entries
}

func (d *Database) GetTimesheetWeek(startDate time.Time) *[]*TimesheetEntry {
	collection := d.DB.Collection("timesheets")
	year, month, day := startDate.Date()
	weekStart := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	weekEnd := weekStart.AddDate(0, 0, 7)
	filter := bson.M{
		"startDate": bson.M{
			"$gte": weekStart.UTC(),
			"$lt":  weekEnd.UTC(),
		},
	}

	cursor, err := collection.Find(d.Context, filter)
	if err != nil {
		log.Printf("Error finding timesheet week: %v", err)
		return nil
	}
	defer cursor.Close(d.Context)
	var entries []*TimesheetEntry
	if err = cursor.All(d.Context, &entries); err != nil {
		log.Printf("Error decoding timesheet weeks: %v", err)
		return nil
	}
	entries = d.SortTimesheetEntries(entries)
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
