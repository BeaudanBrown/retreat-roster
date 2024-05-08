package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

const DATA_DIR = "./data/timesheets/"

type TimesheetWeekState struct {
  ID uuid.UUID
  StartDate  time.Time   `json:"startDate"`
  StaffTimesheets map[uuid.UUID]*TimesheetWeek
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

type TimesheetData struct {
  TimesheetWeek
  StartDate  time.Time
  StaffMember StaffMember
}

func MakeTimesheetStruct(staffMember StaffMember, week TimesheetWeek, startDate time.Time) TimesheetData {
  return TimesheetData{
    TimesheetWeek: week,
    StartDate: startDate,
    StaffMember: staffMember,
  }
}

func getStaffTimesheetWeek(startDate time.Time, staffID uuid.UUID) *TimesheetWeek {
  t, err := LoadWeek(startDate)
  if err != nil {
    log.Printf("Failed to load timesheet week: %v", err)
    return nil
  }
  thisStaffWeek := t.getTimesheetWeek(staffID)
  return thisStaffWeek
}

func (t *TimesheetWeek) getDayByID(dayID uuid.UUID) *TimesheetDay {
  for _, day := range t.Days {
    if day.ID == dayID {
      return day
    }
  }
  return nil
}

func (t *TimesheetWeekState) getTimesheetWeek(staffID uuid.UUID) *TimesheetWeek {
  if timesheet, exists := t.StaffTimesheets[staffID]; exists {
    return timesheet
  }
  newWeek := newWeek()
  t.StaffTimesheets[staffID] = &newWeek
  SaveTimesheetState(t)
  return &newWeek
}

func (s *Server) HandleTimesheet(w http.ResponseWriter, r *http.Request) {
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  thisStaffWeek := getStaffTimesheetWeek(s.TimesheetStartDate, thisStaff.ID)
  if (thisStaffWeek == nil) {
    return
  }
  data := MakeTimesheetStruct(*thisStaff, *thisStaffWeek, s.TimesheetStartDate)
  s.renderTemplate(w, "timesheet", data)
}

func newTimesheetWeekState(startDate time.Time) *TimesheetWeekState {
  s := &TimesheetWeekState{
    ID:            uuid.New(),
    StartDate: startDate,
    StaffTimesheets: map[uuid.UUID]*TimesheetWeek{},
  }
  return s
}

func newWeek() TimesheetWeek {
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

func SaveTimesheetState(s *TimesheetWeekState) error {
  data, err := json.Marshal(s)
  if err != nil {
    log.Println("Error jsonifying timesheet week")
    log.Println(err)
    return err
  }
  log.Println("Saving timesheet week")
  if err := os.WriteFile(GetWeekFilename(s.StartDate), data, 0666); err != nil {
    log.Println("Error saving timesheet week")
    log.Println(err)
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
    log.Println("Creating new timesheet week")
    s = newTimesheetWeekState(startDate)
    SaveTimesheetState(s)
  } else {
    var data []byte
    if data, err = os.ReadFile(filename); err != nil {
      log.Println("Failed to read timesheet week")
      return nil, err
    }
    if err = json.Unmarshal(data, &s); err != nil {
      log.Println("Failed to parse timesheet week")
      return nil, err
    }
  }

  log.Println("Loaded timesheet week")
  return s, nil
}

type ShiftTimesheetWindowBody struct {
  Action string `json:"action"`
}

func (s *Server) HandleShiftTimesheetWindow(w http.ResponseWriter, r *http.Request) {
  var reqBody ShiftTimesheetWindowBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  switch reqBody.Action {
  case "+":
    s.TimesheetStartDate = s.TimesheetStartDate.AddDate(0, 0, 7)
  case "-":
    s.TimesheetStartDate = s.TimesheetStartDate.AddDate(0, 0, -7)
  default:
    today := time.Now()
    daysSinceTuesday := int(today.Weekday() - time.Tuesday)
    if daysSinceTuesday < 0 {
        daysSinceTuesday += 7
    }
    s.TimesheetStartDate = today.AddDate(0, 0, -daysSinceTuesday)
  }
  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  thisStaffWeek := getStaffTimesheetWeek(s.TimesheetStartDate, thisStaff.ID)
  if (thisStaffWeek == nil) {
    return
  }
  data := MakeTimesheetStruct(*thisStaff, *thisStaffWeek, s.TimesheetStartDate)
  s.renderTemplate(w, "timesheet", data)
}

type AddTimesheetEntryBody struct {
  StaffID string `json:"staffID"`
  DayID string `json:"dayID"`
  StartDate CustomDate	`json:"start-date"`
}

func (s *Server) AddTimesheetEntry(w http.ResponseWriter, r *http.Request) {
  var reqBody AddTimesheetEntryBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { log.Printf("Error parsing body: %v", err); return }
  staffID, err := uuid.Parse(reqBody.StaffID)
  if err != nil {
    log.Printf("Invalid staffID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  dayID, err := uuid.Parse(reqBody.DayID)
  if err != nil {
    log.Printf("Invalid dayID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  t, err := LoadWeek(reqBody.StartDate.Time)
  if err != nil {
    log.Printf("Failed to load timesheet week: %v", err)
    return
  }
  week := t.getTimesheetWeek(staffID)
  day := week.getDayByID(dayID)
  if day == nil {
    log.Printf("Could not find day: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  day.Entries = append(day.Entries, &TimesheetEntry{
    ID:      uuid.New(),
  })
  SaveTimesheetState(t)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    log.Printf("Failed to find staff")
    return
  }
  thisStaffWeek := getStaffTimesheetWeek(s.TimesheetStartDate, thisStaff.ID)
  if (thisStaffWeek == nil) {
    log.Printf("Failed to find timesheet week")
    return
  }
  data := MakeTimesheetStruct(*thisStaff, *thisStaffWeek, s.TimesheetStartDate)
  s.renderTemplate(w, "timesheet", data)
}
