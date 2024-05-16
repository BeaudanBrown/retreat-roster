package server

import (
	"encoding/json"
	"log"
	"math"
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

func stringToShiftType(typeStr string) ShiftType {
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
  HasBreak         bool `json:"hasBreak"`
}

type TimesheetData struct {
  TimesheetWeekState
  StaffMember StaffMember
  DayNames []string
  StaffStringMap map[uuid.UUID]string
  HideApproved  bool
  RosterLive  bool
  ApprovalMode  bool
  CacheBust  string
}

func (s *Server) MakeTimesheetStruct(staffMember StaffMember) TimesheetData {
  t, err := LoadWeek(s.TimesheetStartDate)
  if err != nil {
    log.Fatalf("Failed to load timesheet week: %v", err)
  }
  return TimesheetData{
    RosterLive: s.IsLive,
    TimesheetWeekState: *t,
    StaffMember: staffMember,
    DayNames: []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"},
    HideApproved: s.HideApproved,
    ApprovalMode: s.ApprovalMode,
    CacheBust: s.CacheBust,
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
  data := s.MakeTimesheetStruct(*thisStaff)
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

func (s *Server) RenderTimesheetTemplate(w http.ResponseWriter, r *http.Request, adminView bool) {
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }

  data := s.MakeTimesheetStruct(*thisStaff)
  s.renderTemplate(w, "timesheet", data)
}

type ShiftTimesheetWindowBody struct {
  Action string `json:"action"`
  AdminView bool `json:"adminView"`
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
  s.RenderTimesheetTemplate(w, r, reqBody.AdminView)
}

type DeleteTimesheetEntryBody struct {
  StaffID string `json:"staffID"`
  EntryID string `json:"entryID"`
  StartDate CustomDate	`json:"start-date"`
  AdminView bool `json:"adminView"`
}

func (s *Server) HandleDeleteTimesheetEntry(w http.ResponseWriter, r *http.Request) {
  log.Println("Delete timesheet entry")
  var reqBody DeleteTimesheetEntryBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  entryID, err := uuid.Parse(reqBody.EntryID)
  if err != nil {
    log.Printf("Invalid entryID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  staffID, err := uuid.Parse(reqBody.StaffID)
  if err != nil {
    log.Printf("Invalid staffID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  t, err := LoadWeek(*reqBody.StartDate.Time)
  if err != nil {
    log.Printf("Failed to load timesheet week: %v", err)
    return
  }
  thisStaffWeek := t.getTimesheetWeek(staffID)

  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  found := false
  for _, day := range thisStaffWeek.Days {
    for i, entry := range day.Entries {
      if entry.ID == entryID {
        if thisStaff.IsAdmin || staffID == thisStaff.ID {
          day.Entries = append(day.Entries[:i], day.Entries[i+1:]...)
          SaveTimesheetState(t)
          found = true
        }
        break
      }
    }
    if found { break }
  }
  s.RenderTimesheetTemplate(w, r, reqBody.AdminView)
}

type AddTimesheetEntryBody struct {
  StaffID string `json:"staffID"`
  DayIdx int `json:"dayIdx"`
  StartDate CustomDate	`json:"start-date"`
  AdminView bool `json:"adminView"`
}

func (s *Server) HandleAddTimesheetEntry(w http.ResponseWriter, r *http.Request) {
  var reqBody AddTimesheetEntryBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { log.Printf("Error parsing body: %v", err); return }
  staffID, err := uuid.Parse(reqBody.StaffID)
  if err != nil {
    log.Printf("Invalid staffID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  t, err := LoadWeek(*reqBody.StartDate.Time)
  if err != nil {
    log.Printf("Failed to load timesheet week: %v", err)
    return
  }
  week := t.getTimesheetWeek(staffID)
  day := week.Days[reqBody.DayIdx]
  if day == nil {
    log.Printf("Could not find day: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  day.Entries = append(day.Entries, &TimesheetEntry{
    ID:      uuid.New(),
  })
  SaveTimesheetState(t)
  s.RenderTimesheetTemplate(w, r, reqBody.AdminView)
}

type ModifyTimesheetEntryBody struct {
    StaffID       string     `json:"staffID"`
    EntryID         string     `json:"entryID"`
    StartDate     CustomDate `json:"start-date"`
    ShiftStart CustomDate     `json:"shiftStart"`
    ShiftEnd  CustomDate     `json:"shiftEnd"`
    BreakStart CustomDate     `json:"breakStart"`
    BreakEnd  CustomDate     `json:"breakEnd"`
    HasBreak         string       `json:"hasBreak"`
    Status         ApprovalStatus       `json:"status"`
    ShiftType        string  `json:"shiftType"`
    AdminView         bool       `json:"adminView"`
}

func getAdjustedTime(t CustomDate, dayDate time.Time) (*time.Time) {
  year, month, day := dayDate.Date()
  if t.Time != nil {
    hour, min, _ := t.Clock()
    newBreakStart := time.Date(year, month, day, hour, min, 0, 0, dayDate.Location())
    return &newBreakStart
  } else {
    return nil
  }
}

func (s *Server) HandleModifyTimesheetEntry(w http.ResponseWriter, r *http.Request) {
  var reqBody ModifyTimesheetEntryBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { log.Printf("Error parsing body: %v", err); return }
  entryID, err := uuid.Parse(reqBody.EntryID)
  if err != nil {
    log.Printf("Invalid entryID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }

  t, err := LoadWeek(*reqBody.StartDate.Time)
  if err != nil {
    log.Printf("Failed to load timesheet week: %v", err)
    return
  }

  found := false
  for _, week := range t.StaffTimesheets {
    for _, day := range week.Days {
      for _, entry := range day.Entries {
        if entry.ID == entryID {
          dayDate := t.StartDate.AddDate(0, 0, day.Offset)
          found = true
          entry.HasBreak = reqBody.HasBreak == "on"
          entry.Status = reqBody.Status
          entry.ShiftType = stringToShiftType(reqBody.ShiftType)

          if entry.HasBreak {
            entry.BreakStart = getAdjustedTime(reqBody.BreakStart, dayDate)
            entry.BreakEnd = getAdjustedTime(reqBody.BreakEnd, dayDate)
            if entry.BreakStart != nil && entry.BreakEnd != nil {
              if entry.BreakStart.After(*entry.BreakEnd) {
                newBreakEnd := entry.BreakEnd.AddDate(0, 0, 1)
                entry.BreakEnd = &newBreakEnd
              }
              entry.BreakLength = math.Round(entry.BreakEnd.Sub(*entry.BreakStart).Hours() * 100) / 100
            } else {
              entry.BreakLength = 0
            }
          } else {
            entry.BreakStart = nil
            entry.BreakEnd = nil
            entry.BreakLength = 0
          }

          entry.ShiftStart = getAdjustedTime(reqBody.ShiftStart, dayDate)
          entry.ShiftEnd = getAdjustedTime(reqBody.ShiftEnd, dayDate)

          if entry.ShiftStart != nil && entry.ShiftEnd != nil {
            if entry.ShiftStart.After(*entry.ShiftEnd) {
              newShiftEnd := entry.ShiftEnd.AddDate(0, 0, 1)
              entry.ShiftEnd = &newShiftEnd
            }
            entry.ShiftLength = math.Round((entry.ShiftEnd.Sub(*entry.ShiftStart).Hours() - entry.BreakLength) * 100) / 100
          } else {
            entry.ShiftLength = 0
          }
          SaveTimesheetState(t)
          break
        }
      }
      if found { break }
    }
  }
  s.RenderTimesheetTemplate(w, r, reqBody.AdminView)
}

func (s *Server) HandleToggleApprovalMode(w http.ResponseWriter, r *http.Request) {
  s.ApprovalMode = !s.ApprovalMode
  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  s.RenderTimesheetTemplate(w, r, thisStaff.IsAdmin)
}

func (s *Server) HandleToggleHideApproved(w http.ResponseWriter, r *http.Request) {
  s.HideApproved = !s.HideApproved
  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  s.RenderTimesheetTemplate(w, r, thisStaff.IsAdmin)
}

type TimesheetEntryData struct {
  TimesheetEntry
  StartDate  time.Time
  StaffMember StaffMember
  ApprovalMode bool
}

func MakeTimesheetEntryStruct(entry TimesheetEntry, staffMember StaffMember, startDate time.Time, approvalMode bool) TimesheetEntryData {
  return TimesheetEntryData{
    TimesheetEntry: entry,
    StartDate: startDate,
    StaffMember: staffMember,
    ApprovalMode: approvalMode,
  }
}
