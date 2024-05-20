package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

const SESSION_KEY = "sessionToken"
const STAFF_STATE_FILE = "./data/staff.json"
const ROSTER_DIR = "./data/rosters/"
const DEV_MODE = false

type Server struct {
  CacheBust string
  Templates *template.Template
  StaffState
}

type StaffState struct {
  Staff *[]*StaffMember `json:"staff"`
}

type RosterWeek struct {
  ID uuid.UUID
  StartDate  time.Time   `json:"startDate"`
  Days  []*RosterDay   `json:"days"`
  IsLive         bool `json:"isLive"`
}

type Highlight int

const (
  None Highlight = iota
  Duplicate
  PrefConflict
  PrefRefuse
  LeaveConflict
)

func GetHighlightCol(defaultCol string, flag Highlight) string {
  if flag == Duplicate {
    return "#FFA07A"
  }
  if flag == PrefConflict {
    return "#FF9999"
  }
  if flag == LeaveConflict || flag == PrefRefuse {
    return "#CC3333"
  }
  return defaultCol
}

func (s *Server) renderTemplate(w http.ResponseWriter, templateName string, data interface{}) {
  err := s.Templates.ExecuteTemplate(w, templateName, data)
  if err != nil {
    log.Printf("Error executing template: %v\n", err)
    http.Error(w, "Internal Server Error", http.StatusInternalServerError)
  }
}

func (s *Server) VerifyAdmin(handler http.HandlerFunc) http.HandlerFunc {
  return s.VerifySession(func(w http.ResponseWriter, r *http.Request) {
    staff := s.GetSessionUser(w, r)
    if (staff == nil || !staff.IsAdmin) {
      http.Redirect(w, r, "/profile", http.StatusSeeOther)
      return
    }
    handler(w, r)
  })
}

func (s *Server) VerifySession(handler http.HandlerFunc) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    log.Println("Verify")
    cookie, err := r.Cookie("session_token")
    if err != nil {
      if err == http.ErrNoCookie {
        http.Redirect(w, r, "/landing", http.StatusSeeOther)
      } else {
        http.Error(w, "Bad Request", http.StatusBadRequest)
      }
      return
    }

    sessionTokenStr := cookie.Value

    sessionToken, err := uuid.Parse(sessionTokenStr)
    if err != nil {
      http.Redirect(w, r, "/landing", http.StatusSeeOther)
      return
    }

    if !s.isValidSession(sessionToken) {
      http.Redirect(w, r, "/landing", http.StatusSeeOther)
      return
    }

    ctx := context.WithValue(r.Context(), SESSION_KEY, sessionToken)
    reqWithToken := r.WithContext(ctx)
    handler(w, reqWithToken)
  }
}

func (s *Server) isValidSession(token uuid.UUID) bool {
  for i := range *s.Staff {
    if (*s.Staff)[i].Token != nil && *(*s.Staff)[i].Token == token {
      return true
    }
  }
  return false
}

func ReadAndUnmarshal(w http.ResponseWriter, r *http.Request, reqBody interface{}) error {
  bytes, err := io.ReadAll(r.Body)
  if err != nil {
    log.Printf("Error reading body: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return err
  }
  defer r.Body.Close()

  err = json.Unmarshal(bytes, reqBody)
  if err != nil {
    log.Printf("json: %v", string(bytes))
    log.Printf("Error parsing json: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return err
  }

  return nil
}

func GetRosterWeekFilename(startDate time.Time) string {
    formattedDate := startDate.Format("2006-01-02") // Go uses this specific date as the layout pattern
    return ROSTER_DIR + formattedDate + ".json"
}

func (w *RosterWeek) Save() error {
  s, err := LoadServerState()
  w.CheckFlags(s.StaffState)
  if err != nil {
    log.Println("Failed to load server state")
    return err
  }
  data, err := json.Marshal(w)
  if err != nil {
    log.Println("Failed to marshal rosterWeek")
    return err
  }
  log.Println("Saving roster week")
  filename := GetRosterWeekFilename(w.StartDate)
  if err := os.WriteFile(filename, data, 0666); err != nil {
    log.Println("Error saving roster week")
    log.Println(err)
    return err
  }
  return nil
}

func LoadRosterWeek(startDate time.Time) RosterWeek {
  var rosterWeek RosterWeek
  var err error
  filename := GetRosterWeekFilename(startDate)
  if _, err = os.Stat(filename); err != nil {
    log.Println("No file")
    rosterWeek = newRosterWeek(startDate)
    rosterWeek.Save()
  } else {
    var data []byte
    if data, err = os.ReadFile(filename); err != nil {
      log.Println("No read file")
      rosterWeek = newRosterWeek(startDate)
      rosterWeek.Save()
    } else if err = json.Unmarshal(data, &rosterWeek); err != nil {
      log.Println("No json file")
      rosterWeek = newRosterWeek(startDate)
      rosterWeek.Save()
      // TODO: check for save errors?
    }
  }

  log.Println("Loaded rosterWeek")
  return rosterWeek
}

func (s *StaffState) Save() error {
  data, err := json.Marshal(s)
  if err != nil {
    return err
  }
  log.Println("Saving staff state")
  if err := os.WriteFile(STAFF_STATE_FILE, data, 0666); err != nil {
    return err
  }
  return nil
}

func NewServerState() (*Server) {
    staffState := NewStaffState()
    staffState.Save()
    return &Server{
      CacheBust: fmt.Sprintf("%v", time.Now().UnixNano()),
      StaffState: staffState,
    }
}

func LoadStaffState() (StaffState) {
  var staffState StaffState
  var err error
  if _, err = os.Stat(STAFF_STATE_FILE); err != nil {
    log.Println(err)
    staffState = NewStaffState()
  } else {
    var data []byte
    if data, err = os.ReadFile(STAFF_STATE_FILE); err != nil {
      log.Println(err)
      staffState = NewStaffState()
    }
    if err = json.Unmarshal(data, &staffState); err != nil {
      log.Println(err)
      staffState = NewStaffState()
    }
  }
  return staffState
}

func LoadServerState() (*Server, error) {
  var serverState Server
  var err error
  serverState = Server{
    CacheBust: fmt.Sprintf("%v", time.Now().UnixNano()),
    Templates: template.New("").Funcs(template.FuncMap{
      "MakeHeaderStruct": MakeHeaderStruct,
      "MakeDayStruct": MakeDayStruct,
      "GetHighlightCol": GetHighlightCol,
      "MakeProfileStruct": MakeProfileStruct,
      "MemberIsAssigned": MemberIsAssigned,
      "MakeTimesheetEntryStruct": MakeTimesheetEntryStruct,
      "addDays": func(t time.Time, days int) time.Time {
        return t.AddDate(0, 0, days)
      },
    }),
    StaffState: LoadStaffState(),
  }
  serverState.Templates, err = serverState.Templates.ParseGlob("./www/*.html")
  if err != nil {
    return nil, err
  }
  return &serverState, nil
}

func newRow() *Row {
  return &Row{
    ID:     uuid.New(),
    Amelia:  newSlot(),
    Early:  newSlot(),
    Mid: newSlot(),
    Late:   newSlot(),
  }
}

func newSlot() Slot {
  return Slot{
    ID:            uuid.New(),
    StartTime:     "",
    AssignedStaff: nil,
  }
}

func newRosterWeek(startDate time.Time) RosterWeek {
  dayNames := []string{"Tues", "Wed", "Thurs", "Fri", "Sat", "Sun", "Mon"}
  var Days []*RosterDay

  for i, dayName := range dayNames {
    var colour string
    if i%2 == 0 {
      colour = "#b7b7b7"
    } else {
      colour = "#ffffff"
    }
    Days = append(Days, &RosterDay{
      ID:      uuid.New(),
      DayName: dayName,
      Rows: []*Row{
        newRow(),
        newRow(),
        newRow(),
        newRow(),
      },
      Colour:         colour,
      Offset:         i,
    })
  }
  week := RosterWeek{
    uuid.New(),
    startDate,
    Days,
    false,
  }
  return week
}

func NewStaffState() StaffState {
  staff := []*StaffMember{}
  s := StaffState{
    &staff,
  }
  s.Save()
  return s
}

func (s *Server) GetStaffByToken(token uuid.UUID) *StaffMember {
  for i := range *s.Staff {
    if (*s.Staff)[i].Token != nil && *(*s.Staff)[i].Token == token {
      return (*s.Staff)[i]
    }
  }
  return nil
}

func (s *Server) GetSessionUser(w http.ResponseWriter, r *http.Request) *StaffMember {
  sessionToken, ok := r.Context().Value(SESSION_KEY).(uuid.UUID)
  if !ok {
    log.Printf("Error retrieving token")
    return nil
  }
  staff := s.GetStaffByToken(sessionToken)
  if staff == nil {
    log.Printf("Error retrieving session user")
    return nil
  }
  return staff
}

func GetStaffByID(staffID uuid.UUID, allStaff []*StaffMember) *StaffMember {
  for i := range allStaff {
    if (allStaff)[i].ID == staffID {
      return (allStaff)[i]
    }
  }
  return nil
}

func (week *RosterWeek) GetSlotByID(slotID uuid.UUID) *Slot {
  for i := range week.Days {
    day := week.Days[i]
    for j := range day.Rows {
      row := day.Rows[j]
      if row.Amelia.ID == slotID {
        return &row.Amelia
      }
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
  for i := range week.Days {
    if week.Days[i].ID == dayID {
      return week.Days[i]
    }
  }
  return nil
}

func (s *Server) HandleLanding(w http.ResponseWriter, r *http.Request) {
  s.renderTemplate(w, "landing", s.CacheBust)
}

type HeaderData struct {
  RosterLive  bool
  IsAdmin  bool
}

func MakeHeaderStruct(isAdmin bool, rosterLive bool) HeaderData {
  return HeaderData{
    RosterLive: rosterLive,
    IsAdmin: isAdmin,
  }
}

func (s *Server) GetStaffMap() map[uuid.UUID]StaffMember {
  staffMap := map[uuid.UUID]StaffMember{}
  for _, staff := range *s.Staff {
    staffMap[staff.ID] = *staff
  }
  return staffMap
}
