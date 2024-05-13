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
const STATE_FILE = "./data/state.json"
const DEV_MODE = false

type Server struct {
  CacheBust string
  Templates *template.Template
  ServerDisc
}

type Highlight int

const (
  None Highlight = iota
  Duplicate
  PrefConflict
  LeaveConflict
)

func GetHighlightCol(defaultCol string, flag Highlight) string {
  if flag == Duplicate {
    return "#FFA07A"
  }
  if flag == PrefConflict {
    return "#FF9999"
  }
  if flag == LeaveConflict {
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

func SaveState(s *Server) error {
  s.CheckFlags()
  data, err := json.Marshal(s.ServerDisc)
  if err != nil {
    return err
  }
  log.Println("Saving state")
  if err := os.WriteFile(STATE_FILE, data, 0666); err != nil {
    return err
  }
  return nil
}

func LoadState(filename string) (*Server, error) {
  var state *Server
  var err error
  if _, err = os.Stat(filename); err != nil {
    state = newState()
    SaveState(state)
  } else {
    var data []byte
    if data, err = os.ReadFile(filename); err != nil {
      return nil, err
    }
    if err = json.Unmarshal(data, &state); err != nil {
      return nil, err
    }
  }

  log.Println("Loaded state")
  state.CacheBust = fmt.Sprintf("%v", time.Now().UnixNano())
  state.Templates = template.New("").Funcs(template.FuncMap{
    "MakeDayStruct": MakeDayStruct,
    "GetHighlightCol": GetHighlightCol,
    "MakeRootStruct": MakeRootStruct,
    "MakeProfileStruct": MakeProfileStruct,
    "MemberIsAssigned": MemberIsAssigned,
    // "CleanAndSortLeaveReqs": CleanAndSortLeaveReqs,
  })
  state.Templates, err = state.Templates.ParseGlob("./www/*.html")
  if err != nil {
    return nil, err
  }
  return state, nil
}

type ServerDisc struct {
  StartDate  time.Time   `json:"startDate"`
  TimesheetStartDate  time.Time   `json:"timesheetStartDate"`
  Days  []*RosterDay   `json:"days"`
  Staff *[]*StaffMember `json:"staff"`
  IsLive         bool `json:"isLive"`
  HideByIdeal         bool `json:"hideByIdeal"`
  HideByPrefs         bool `json:"hideByPrefs"`
  HideByLeave         bool `json:"hideByLeave"`
  HideApproved  bool `json:"hideIdeal"`
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

func newState() *Server {
  dayNames := []string{"Tues", "Wed", "Thurs", "Fri", "Sat", "Sun", "Mon"}
  today := time.Now()
  daysUntilTuesday := int(time.Tuesday - today.Weekday())
  if daysUntilTuesday <= 0 {
    daysUntilTuesday += 7
  }
  nextTuesday := today.AddDate(0, 0, daysUntilTuesday)

  var Days []*RosterDay
  staff := []*StaffMember{}

  // Loop over dayNames to fill Days slice
  for i, dayName := range dayNames {
    var colour string
    if i%2 == 0 {
      colour = "#b7b7b7"
    } else {
      colour = "#ffffff"
    }
    Days = append(Days, &RosterDay{
      ID:      uuid.New(), // Generates a new UUID
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

  s := &Server{
    CacheBust: fmt.Sprintf("%v", time.Now().UnixNano()),
    ServerDisc: ServerDisc{
      Days:  Days,
      Staff: &staff,
      StartDate: nextTuesday,
    },
  }
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

func (s *Server) GetStaffByID(staffID uuid.UUID) *StaffMember {
  for i := range *s.Staff {
    if (*s.Staff)[i].ID == staffID {
      return (*s.Staff)[i]
    }
  }
  return nil
}

func (s *Server) GetSlotByID(slotID uuid.UUID) *Slot {
  for i := range s.Days {
    day := s.Days[i]
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

func (s *Server) GetDayByID(dayID uuid.UUID) *RosterDay {
  for i := range s.Days {
    if s.Days[i].ID == dayID {
      return s.Days[i]
    }
  }
  return nil
}

func (s *Server) HandleLanding(w http.ResponseWriter, r *http.Request) {
  s.renderTemplate(w, "landing", s.CacheBust)
}

func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
  s.renderTemplate(w, "index", s.CacheBust)
}
