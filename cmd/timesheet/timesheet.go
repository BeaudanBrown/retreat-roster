package timesheet

import (
  "encoding/json"
  "log"
  "os"
  "time"

  "github.com/google/uuid"
)

const DATA_DIR = "./data/timesheets/"

type TimesheetWeekState struct {
  ID uuid.UUID
  StartDate  time.Time   `json:"startDate"`
  Entries map[uuid.UUID][]*TimesheetWeek
}

type TimesheetWeek struct {
  ID uuid.UUID
  StaffName        string
  Days  []*TimesheetDay   `json:"days"`
}

type TimesheetDay struct {
  ID uuid.UUID
  DayName        string
  Offset       int
  Entries      []*TimesheetEntry
  Approved       bool
}

type TimesheetEntry struct {
  ID uuid.UUID
  ShiftStart  time.Time   `json:"shiftStart"`
  ShiftEnd  time.Time   `json:"shiftEnd"`
  BreakStart  time.Time   `json:"breakStart"`
  BreakEnd  time.Time   `json:"breakEnd"`
  BreakLength time.Duration `json:"breakLength"`
  ShiftLength time.Duration `json:"shiftLength"`
  Managing         bool `json:"managing"`
  Admin         bool `json:"admin"`
}

func newState(startDate time.Time) *TimesheetWeekState {
  daysUntilTuesday := int(time.Tuesday - startDate.Weekday())
  if daysUntilTuesday <= 0 {
    daysUntilTuesday += 7
  }
  nextTuesday := startDate.AddDate(0, 0, daysUntilTuesday)
  s := &TimesheetWeekState{
    ID:            uuid.New(),
    StartDate: nextTuesday,
  }
  return s
}

func newWeek(staffName string) *TimesheetWeek {
  dayNames := []string{"Tues", "Wed", "Thurs", "Fri", "Sat", "Sun", "Mon"}

  var Days []*TimesheetDay

  // Loop over dayNames to fill Days slice
  for i, dayName := range dayNames {
    Days = append(Days, &TimesheetDay{
      ID:      uuid.New(),
      DayName: dayName,
      Offset:  i,
    })
  }

  w := &TimesheetWeek{
    ID:            uuid.New(),
    Days:  Days,
    StaffName: staffName,
  }
  return w
}

func SaveState(s *TimesheetWeekState) error {
  data, err := json.Marshal(s)
  if err != nil {
    return err
  }
  log.Println("Saving timesheet week")
  if err := os.WriteFile(GetWeekFilename(s.StartDate), data, 0666); err != nil {
    return err
  }
  return nil
}

func GetWeekFilename(startDate time.Time) string {
    formattedDate := startDate.Format("2006-01-02") // Go uses this specific date as the layout pattern
    return DATA_DIR + formattedDate + ".json"
}

func LoadWeek(startDate time.Time) (*TimesheetWeekState, error) {
  var s *TimesheetWeekState
  var err error
  filename := GetWeekFilename(startDate)
  if _, err = os.Stat(filename); err != nil {
    s = newState(startDate)
    SaveState(s)
  } else {
    var data []byte
    if data, err = os.ReadFile(filename); err != nil {
      return nil, err
    }
    if err = json.Unmarshal(data, &s); err != nil {
      return nil, err
    }
  }

  log.Println("Loaded timesheet week")
  return s, nil
}
