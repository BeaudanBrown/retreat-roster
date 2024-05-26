package db

import (
	"log"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type TimesheetWeekState struct {
  ID uuid.UUID `bson:"_id"`
  StartDate  time.Time   `bson:"startDate"`
  StaffTimesheets map[uuid.UUID]*TimesheetWeek `bson:"staffTimesheets"`
}

type TimesheetWeek struct {
  ID uuid.UUID
  Days  []*TimesheetDay   `json:"days"`
}

type TimesheetDay struct {
  ID uuid.UUID
  DayName        string
  Offset       int
  Entries      []*TimesheetEntry
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
    Managing
    Admin
)

type TimesheetEntry struct {
  ID uuid.UUID
  ShiftStart  *time.Time   `json:"shiftStart"`
  ShiftEnd  *time.Time   `json:"shiftEnd"`
  BreakStart  *time.Time   `json:"breakStart"`
  BreakEnd  *time.Time   `json:"breakEnd"`
  BreakLength float64 `json:"breakLength"`
  ShiftLength float64 `json:"shiftLength"`
  Status        ApprovalStatus  `json:"status"`
  ShiftType        ShiftType  `json:"shiftType"`
}

func StringToShiftType(typeStr string) ShiftType {
  switch typeStr {
  case "0":
    return Bar
  case "1":
    return Managing
  case "2":
    return Admin
  default:
    return Bar
  }
}

func (d *Database) SaveTimesheetState(timesheetWeek *TimesheetWeekState) error {
  collection := d.DB.Collection("timesheets")
  filter := bson.M{"_id": timesheetWeek.ID}
  update := bson.M{"$set": timesheetWeek}
  opts := options.Update().SetUpsert(true)
  _, err := collection.UpdateOne(d.Context, filter, update, opts)
  if err != nil {
      log.Println("Failed to save timesheet week")
      return err
  }
  log.Println("Saved staff timesheet week")
  return nil
}

func (d *Database) LoadTimesheetWeek(startDate time.Time) (*TimesheetWeekState) {
  var weekState *TimesheetWeekState
  filter := bson.M{"startDate": startDate}
  collection := d.DB.Collection("timesheets")
  err := collection.FindOne(d.Context, filter).Decode(&weekState)
  if err == nil {
    log.Printf("Found timesheet week")
    return weekState
  }

  if err != mongo.ErrNoDocuments {
    log.Printf("Error loading timesheet week: %v", err)
    return nil
  }

  log.Printf("Making new timesheet week")
  weekState = newTimesheetWeekState(startDate)
  if saveErr := d.SaveTimesheetWeek(*weekState); saveErr != nil {
    log.Printf("Error saving timesheet week: %v", saveErr)
    return nil
  }

  return weekState
}

func newTimesheetWeekState(startDate time.Time) *TimesheetWeekState {
  s := &TimesheetWeekState{
    ID:            uuid.New(),
    StartDate: startDate,
    StaffTimesheets: map[uuid.UUID]*TimesheetWeek{},
  }
  return s
}

func NewTimesheetWeek() TimesheetWeek {
  dayNames := []string{"Tues", "Wed", "Thurs", "Fri", "Sat", "Sun", "Mon"}

  var Days []*TimesheetDay

  // Loop over dayNames to fill Days slice
  for i, dayName := range dayNames {
    Days = append(Days, &TimesheetDay{
      ID:      uuid.New(),
      DayName: dayName,
      Offset:  i,
      Entries:  []*TimesheetEntry{},
    })
  }

  w := TimesheetWeek{
    ID:            uuid.New(),
    Days:  Days,
    // StaffName: staffName,
  }
  return w
}

func (d *Database) SaveTimesheetWeek (w TimesheetWeekState) error {
  collection := d.DB.Collection("timesheets")
  filter := bson.M{"_id": w.ID}
  update := bson.M{"$set": w}
  opts := options.Update().SetUpsert(true)
  _, err := collection.UpdateOne(d.Context, filter, update, opts)
  if err != nil {
      log.Println("Failed to save timesheet week")
      return err
  }
  log.Println("Saved timesheet week")
  return nil
}

func (d *Database) GetStaffTimesheetWeek(staffID uuid.UUID, t *TimesheetWeekState) *TimesheetWeek {
  if timesheet, exists := t.StaffTimesheets[staffID]; exists {
    return timesheet
  }
  newWeek := NewTimesheetWeek()
  t.StaffTimesheets[staffID] = &newWeek
  d.SaveTimesheetState(t)
  return &newWeek
}

func (d *Database) GetTimesheetEntryByIDAndStartDate(startDate time.Time, entryID uuid.UUID) (*TimesheetEntry) {
    filter := bson.M{
        "startDate": startDate,
        "staffTimesheets.entries.id": entryID,
    }

    var result TimesheetWeekState
    collection := d.DB.Collection("timesheets")
    err := collection.FindOne(d.Context, filter).Decode(&result)
    if err != nil {
        return nil
    }

    for _, week := range result.StaffTimesheets {
        for _, day := range week.Days {
            for _, entry := range day.Entries {
                if entry.ID == entryID {
                    return entry
                }
            }
        }
    }

    return nil
}

// Function to insert or update a TimesheetEntry by ID and StartDate
func (d *Database) UpsertTimesheetEntry(client *mongo.Client, dbName, collectionName string, startDate time.Time, entry *TimesheetEntry) error {
    collection := client.Database(dbName).Collection(collectionName)

    filter := bson.M{
        "startDate": startDate,
        "staffTimesheets.entries.id": entry.ID,
    }

    update := bson.M{
        "$set": bson.M{
            "staffTimesheets.$[week].days.$[day].entries.$[entry]": entry,
        },
    }

    arrayFilters := options.ArrayFilters{
        Filters: []interface{}{
            bson.M{"week.days.entries.id": entry.ID},
            bson.M{"day.entries.id": entry.ID},
            bson.M{"entry.id": entry.ID},
        },
    }

    opts := options.Update().SetUpsert(true)
    opts.ArrayFilters = &arrayFilters

    _, err := collection.UpdateOne(d.Context, filter, update, opts)
    return err
}
