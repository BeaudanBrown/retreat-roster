package server

import (
	"log"
	"math"
	"net/http"
	"time"

  "roster/cmd/db"
  "roster/cmd/utils"

	"github.com/google/uuid"
)

type TimesheetData struct {
  db.TimesheetWeekState
  StaffMember db.StaffMember
  DayNames []string
  StaffMap map[uuid.UUID]db.StaffMember
  RosterLive  bool
  CacheBust  string
}

func (s *Server) MakeTimesheetStruct(activeStaff db.StaffMember) TimesheetData {
  timesheetWeek := s.LoadTimesheetWeek(activeStaff.Config.TimesheetStartDate)
  if timesheetWeek == nil {
    log.Printf("Failed to load timesheet week when making struct")
  }

  return TimesheetData{
    TimesheetWeekState: *timesheetWeek,
    StaffMember: activeStaff,
    DayNames: []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"},
    StaffMap: s.GetStaffMap(),
    CacheBust: s.CacheBust,
  }
}

func (s *Server) HandleTimesheet(w http.ResponseWriter, r *http.Request) {
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  data := s.MakeTimesheetStruct(*thisStaff)
  s.renderTemplate(w, "timesheet", data)
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
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  var reqBody ShiftTimesheetWindowBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  switch reqBody.Action {
  case "+":
    thisStaff.Config.TimesheetStartDate = thisStaff.Config.TimesheetStartDate.AddDate(0, 0, 7)
  case "-":
    thisStaff.Config.TimesheetStartDate = thisStaff.Config.TimesheetStartDate.AddDate(0, 0, -7)
  default:
    thisStaff.Config.TimesheetStartDate = utils.GetLastTuesday()
  }
  s.SaveStaffMember(*thisStaff)
  s.RenderTimesheetTemplate(w, r, reqBody.AdminView)
}

type DeleteTimesheetEntryBody struct {
  StaffID string `json:"staffID"`
  EntryID string `json:"entryID"`
  StartDate db.CustomDate	`json:"start-date"`
  AdminView bool `json:"adminView"`
}

func (s *Server) HandleDeleteTimesheetEntry(w http.ResponseWriter, r *http.Request) {
  log.Println("Delete timesheet entry")
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
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
  timesheetWeek := s.LoadTimesheetWeek(*reqBody.StartDate.Time)
  if timesheetWeek == nil {
    log.Printf("Failed to load timesheet week when deleting")
    return
  }
  thisStaffWeek := s.GetStaffTimesheetWeek(staffID, timesheetWeek)
  found := false
  for _, day := range thisStaffWeek.Days {
    for i, entry := range day.Entries {
      if entry.ID == entryID {
        if thisStaff.IsAdmin || staffID == thisStaff.ID {
          day.Entries = append(day.Entries[:i], day.Entries[i+1:]...)
          s.SaveTimesheetWeek(*timesheetWeek)
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
  StartDate db.CustomDate	`json:"start-date"`
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
  timesheetWeek := s.LoadTimesheetWeek(*reqBody.StartDate.Time)
  if timesheetWeek == nil {
    log.Printf("Failed to load timesheet week: %v", err)
    return
  }
  staffTimesheetWeek := s.GetStaffTimesheetWeek(staffID, timesheetWeek)
  day := staffTimesheetWeek.Days[reqBody.DayIdx]
  if day == nil {
    log.Printf("Could not find day: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  day.Entries = append(day.Entries, &db.TimesheetEntry{
    ID:      uuid.New(),
  })
  s.SaveTimesheetState(timesheetWeek)
  s.RenderTimesheetTemplate(w, r, reqBody.AdminView)
}

type ModifyTimesheetEntryBody struct {
    StaffID       string     `json:"staffID"`
    EntryID         string     `json:"entryID"`
    StartDate     db.CustomDate `json:"start-date"`
    ShiftStart db.CustomDate     `json:"shiftStart"`
    ShiftEnd  db.CustomDate     `json:"shiftEnd"`
    BreakStart db.CustomDate     `json:"breakStart"`
    BreakEnd  db.CustomDate     `json:"breakEnd"`
    Status         db.ApprovalStatus       `json:"status"`
    ShiftType        string  `json:"shiftType"`
    AdminView         bool       `json:"adminView"`
}

func getAdjustedTime(t db.CustomDate, dayDate time.Time) (*time.Time) {
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

  timesheetWeek := s.LoadTimesheetWeek(*reqBody.StartDate.Time)
  if timesheetWeek == nil {
    log.Printf("Failed to load timesheet week when modifying")
    return
  }

  found := false
  for _, week := range timesheetWeek.StaffTimesheets {
    for _, day := range week.Days {
      for _, entry := range day.Entries {
        if entry.ID == entryID {
          dayDate := timesheetWeek.StartDate.AddDate(0, 0, day.Offset)
          found = true
          entry.Status = reqBody.Status
          entry.ShiftType = db.StringToShiftType(reqBody.ShiftType)
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
          s.SaveTimesheetState(timesheetWeek)
          break
        }
      }
      if found { break }
    }
  }
  s.RenderTimesheetTemplate(w, r, reqBody.AdminView)
}

func (s *Server) HandleToggleApprovalMode(w http.ResponseWriter, r *http.Request) {
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  thisStaff.Config.ApprovalMode = !thisStaff.Config.ApprovalMode
  s.SaveStaffMember(*thisStaff)
  s.RenderTimesheetTemplate(w, r, thisStaff.IsAdmin)
}

func (s *Server) HandleToggleHideApproved(w http.ResponseWriter, r *http.Request) {
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  thisStaff.Config.HideApproved = !thisStaff.Config.HideApproved
  s.SaveStaffMember(*thisStaff)
  s.RenderTimesheetTemplate(w, r, thisStaff.IsAdmin)
}

type TimesheetEntryData struct {
  db.TimesheetEntry
  StartDate  time.Time
  StaffMember db.StaffMember
  ApprovalMode bool
  IsAdmin  bool
}

func MakeTimesheetEntryStruct(entry db.TimesheetEntry, staffMember db.StaffMember, startDate time.Time, approvalMode bool, isAdmin bool) TimesheetEntryData {
  return TimesheetEntryData{
    TimesheetEntry: entry,
    StartDate: startDate,
    StaffMember: staffMember,
    ApprovalMode: approvalMode,
    IsAdmin: isAdmin,
  }
}
