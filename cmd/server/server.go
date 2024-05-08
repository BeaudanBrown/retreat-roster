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
	"strconv"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const SESSION_KEY = "sessionToken"
const STATE_FILE = "./data/state.json"
const DEV_MODE = false

type Server struct {
  CacheBust string
  Templates *template.Template
  ServerDisc
}

type LeaveRequest struct {
  ID uuid.UUID
  CreationDate CustomDate
  Reason string	`json:"reason"`
  StartDate CustomDate	`json:"start-date"`
  EndDate CustomDate	`json:"end-date"`
}

type CustomDate struct {
  time.Time
}

type Highlight int

type ProfileIndexData struct {
  CacheBust string
  RosterLive bool
  AdminRights bool
  StaffMember
}

type ProfileData struct {
  StaffMember
  AdminRights bool
  RosterLive bool
  ShowUpdateSuccess bool
  ShowUpdateError bool
  ShowLeaveSuccess bool
  ShowLeaveError bool
}

const (
  None Highlight = iota
  Duplicate
  PrefConflict
  LeaveConflict
)

type GoogleUserInfo struct {
  ID            string `json:"id"`
}

var emptyAvailability = []DayAvailability{
  {
    Name: "Tues",
    Early: true,
    Mid:   true,
    Late:  true,
  },
  {
    Name: "Wed",
    Early: true,
    Mid:   true,
    Late:  true,
  },
  {
    Name: "Thurs",
    Early: true,
    Mid:   true,
    Late:  true,
  },
  {
    Name: "Fri",
    Early: true,
    Mid:   true,
    Late:  true,
  },
  {
    Name: "Sat",
    Early: true,
    Mid:   true,
    Late:  true,
  },
  {
    Name: "Sun",
    Early: true,
    Mid:   true,
    Late:  true,
  },
  {
    Name: "Mon",
    Early: true,
    Mid:   true,
    Late:  true,
  },
}

func (s *Server) renderTemplate(w http.ResponseWriter, templateName string, data interface{}) {
  err := s.Templates.ExecuteTemplate(w, templateName, data)
  if err != nil {
    log.Printf("Error executing template: %v\n", err)
    http.Error(w, "Internal Server Error", http.StatusInternalServerError)
  }
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

func googleOauthConfig() *oauth2.Config {
  return &oauth2.Config{
    ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
    ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
    RedirectURL:  os.Getenv("REDIRECT_URL"),
    Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email"},
    Endpoint:     google.Endpoint,
  }
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

func MemberIsAssigned(activeID uuid.UUID, assignedID *uuid.UUID) bool {
  if assignedID == nil {
    return false
  }
  return *assignedID == activeID
}

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

type ServerDisc struct {
  StartDate  time.Time   `json:"startDate"`
  Days  []*RosterDay   `json:"days"`
  Staff *[]*StaffMember `json:"staff"`
  IsLive         bool `json:"isLive"`
  HideByIdeal         bool `json:"hideByIdeal"`
  HideByPrefs         bool `json:"hideByPrefs"`
  HideByLeave         bool `json:"hideByLeave"`
}

type RosterDay struct {
  ID             uuid.UUID
  DayName        string
  Rows           []*Row
  Colour         string
  Offset         int
  IsClosed       bool
  AmeliaOpen     bool
}

type Row struct {
  ID     uuid.UUID
  Amelia  Slot
  Early  Slot
  Mid Slot
  Late   Slot
}

type Slot struct {
  ID            uuid.UUID
  StartTime     string
  AssignedStaff *uuid.UUID
  StaffString *string
  Flag	Highlight
  Description	string
}

func (s *Slot) HasThisStaff(staffId uuid.UUID) bool {
  if s.AssignedStaff != nil && *s.AssignedStaff == staffId {
    return true
  }
  return false
}

type DayAvailability struct {
  Name   string
  Early   bool
  Mid   bool
  Late   bool
}

type StaffMember struct {
  ID   uuid.UUID
  IsAdmin   bool
  IsTrial   bool
  IsHidden   bool
  GoogleID   string
  NickName string
  FirstName string
  LastName string
  Email string
  Phone string
  ContactName string
  ContactPhone string
  IdealShifts int
  CurrentShifts int
  Availability []DayAvailability
  Token *uuid.UUID
  LeaveRequests	[]LeaveRequest
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

func (s *Server) CheckFlags() {
  for _, staff := range *s.Staff {
    staff.CurrentShifts = 0
  }
  for i, day := range s.Days {
    // Create a new map for each day to track occurrences of staff IDs within that day
    staffIDOccurrences := make(map[uuid.UUID]int)

    for _, row := range day.Rows {
      if day.AmeliaOpen && row.Amelia.AssignedStaff != nil {
        staffIDOccurrences[*row.Amelia.AssignedStaff]++
        staff := s.GetStaffByID(*row.Amelia.AssignedStaff)
        if staff != nil {
          staff.CurrentShifts += 1
        }
      }
      if row.Early.AssignedStaff != nil {
        staffIDOccurrences[*row.Early.AssignedStaff]++
        staff := s.GetStaffByID(*row.Early.AssignedStaff)
        if staff != nil {
          staff.CurrentShifts += 1
        }
      }
      if row.Mid.AssignedStaff != nil {
        staffIDOccurrences[*row.Mid.AssignedStaff]++
        staff := s.GetStaffByID(*row.Mid.AssignedStaff)
        if staff != nil {
          staff.CurrentShifts += 1
        }
      }
      if row.Late.AssignedStaff != nil {
        staffIDOccurrences[*row.Late.AssignedStaff]++
        staff := s.GetStaffByID(*row.Late.AssignedStaff)
        if staff != nil {
          staff.CurrentShifts += 1
        }
      }
    }

    for _, row := range day.Rows {
      row.Amelia.Flag = None
      row.Early.Flag = None
      row.Mid.Flag = None
      row.Late.Flag = None
      date := s.StartDate.AddDate(0, 0, day.Offset)

      if day.AmeliaOpen && row.Amelia.AssignedStaff != nil {
        if staffIDOccurrences[*row.Amelia.AssignedStaff] > 1 {
          row.Amelia.Flag = Duplicate
        } else {
          staff := s.GetStaffByID(*row.Amelia.AssignedStaff)
          for _, req := range staff.LeaveRequests {
            if !req.StartDate.After(date) && req.EndDate.After(date) {
              row.Amelia.Flag = LeaveConflict
              break
            }
          }
          if staff != nil {
            if !staff.Availability[i].Late {
              row.Amelia.Flag = PrefConflict
            }
          }
        }
      }

      if row.Early.AssignedStaff != nil {
        if staffIDOccurrences[*row.Early.AssignedStaff] > 1 {
          row.Early.Flag = Duplicate
        } else {
          staff := s.GetStaffByID(*row.Early.AssignedStaff)
          for _, req := range staff.LeaveRequests {
            if !req.StartDate.After(date) && req.EndDate.After(date) {
              row.Early.Flag = LeaveConflict
              break
            }
          }
          if staff != nil {
            if !staff.Availability[i].Early {
              row.Early.Flag = PrefConflict
            }
          }
        }
      }

      if row.Mid.AssignedStaff != nil {
        if staffIDOccurrences[*row.Mid.AssignedStaff] > 1 {
          row.Mid.Flag = Duplicate
        } else {
          staff := s.GetStaffByID(*row.Mid.AssignedStaff)
          if staff != nil {
            for _, req := range staff.LeaveRequests {
              if !req.StartDate.After(date) && req.EndDate.After(date) {
                row.Mid.Flag = LeaveConflict
                break
              }
            }
            if !staff.Availability[i].Mid {
              row.Mid.Flag = PrefConflict
            }
          }
        }
      }

      if row.Late.AssignedStaff != nil {
        if staffIDOccurrences[*row.Late.AssignedStaff] > 1 {
          row.Late.Flag = Duplicate
        } else {
          staff := s.GetStaffByID(*row.Late.AssignedStaff)
          for _, req := range staff.LeaveRequests {
            if !req.StartDate.After(date) && req.EndDate.After(date) {
              row.Late.Flag = LeaveConflict
              break
            }
          }
          if staff != nil {
            if !staff.Availability[i].Late {
              row.Late.Flag = PrefConflict
            }
          }
        }
      }
    }
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

type DeleteLeaveBody struct {
  ID string `json:"id"`
}

func (staff *StaffMember) HasConflict(slot string, offset int) bool {
  switch slot {
  case "Early":
    if !staff.Availability[offset].Early {
      return true
    }
  case "Mid":
    if !staff.Availability[offset].Mid {
      return true
    }
  case "Late":
    if !staff.Availability[offset].Late {
      return true
    }
  }
  return false
}

func (staff *StaffMember) IsAway(date time.Time) bool {
  for _, req := range staff.LeaveRequests {
    if !req.StartDate.After(date) && req.EndDate.After(date) {
      return true
    }
  }
  return false
}

func (s *Server) HandleDeleteLeaveReq(w http.ResponseWriter, r *http.Request) {
  log.Println("Delete leave request")
  var reqBody DeleteLeaveBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  leaveID, err := uuid.Parse(reqBody.ID)
  if err != nil {
    log.Printf("Invalid leaveID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }

  found := false
  adminDelete := false
  for _, staff := range *s.Staff {
    for i, leaveReq := range staff.LeaveRequests {
      if leaveReq.ID == leaveID {
        if thisStaff.IsAdmin && staff.ID == thisStaff.ID {
          adminDelete = true
        }
        if thisStaff.IsAdmin || staff.ID == thisStaff.ID {
          staff.LeaveRequests = append(staff.LeaveRequests[:i], staff.LeaveRequests[i+1:]...)
          SaveState(s)
          found = true
        }
        break
      }
    }
    if found {
      break
    }
  }
  if adminDelete {
    s.renderTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
  } else {
    data := ProfileData{
      StaffMember: *thisStaff,
      AdminRights: thisStaff.IsAdmin,
      RosterLive: s.IsLive,
    }
    s.renderTemplate(w, "profile", data)
  }
}

func (s *Server) HandleSubmitLeave(w http.ResponseWriter, r *http.Request) {
  log.Println("Submit leave request")
  var reqBody LeaveRequest
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  reqBody.ID = uuid.New()
  reqBody.CreationDate = CustomDate{time.Now()}
  staff := s.GetSessionUser(w, r)
  if (staff == nil) {
    return
  }
  data := ProfileData{
    AdminRights: staff.IsAdmin,
    RosterLive: s.IsLive,
  }
  if reqBody.StartDate.After(reqBody.EndDate.Time) {
    data.ShowLeaveError = true
  } else {
    data.ShowLeaveSuccess = true
    staff.LeaveRequests = append(staff.LeaveRequests, reqBody)
    SaveState(s)
  }
  data.StaffMember = *staff
  s.renderTemplate(w, "profile", data)
}

func (s *Server) HandleLanding(w http.ResponseWriter, r *http.Request) {
  s.renderTemplate(w, "landing", s.CacheBust)
}

func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
  s.renderTemplate(w, "index", s.CacheBust)
}

type ProfileIndexBody struct {
  ID            string `json:"editStaffId"`
}

func (s *Server) HandleProfileIndex(w http.ResponseWriter, r *http.Request) {
  editStaff := s.GetSessionUser(w, r)
  if editStaff == nil {
    w.WriteHeader(http.StatusUnauthorized)
    return
  }
  adminRights := editStaff.IsAdmin

  if r.Method == http.MethodGet {
    editStaffIdParam := r.URL.Query().Get("editStaffId")
    if editStaffIdParam != "" {
      editStaffId, err := uuid.Parse(editStaffIdParam)
      if err != nil {
        log.Printf("Invalid UUID: %v", err)
        w.WriteHeader(http.StatusBadRequest)
        return
      }
      staff := s.GetStaffByID(editStaffId)
      if staff != nil {
        editStaff = staff
      } else {
        w.WriteHeader(http.StatusNotFound)
        return
      }
    }
  } else {
    w.WriteHeader(http.StatusMethodNotAllowed)
    return
  }

  data := ProfileIndexData{
    CacheBust: s.CacheBust,
    StaffMember: *editStaff,
    AdminRights: adminRights,
    RosterLive: s.IsLive,
  }

  err := s.Templates.ExecuteTemplate(w, "profileIndex", data)
  if err != nil {
    log.Fatalf("Error executing template: %v", err)
    return
  }
}

func (s *Server) HandleProfile(w http.ResponseWriter, r *http.Request) {
  staff := s.GetSessionUser(w, r)
  if (staff == nil) {
    return
  }

  data := ProfileData{
    AdminRights: staff.IsAdmin,
    StaffMember: *staff,
    RosterLive: s.IsLive,
  }
  s.renderTemplate(w, "profile", data)
}

func (s *Server) GetStaffByToken(token uuid.UUID) *StaffMember {
  for i := range *s.Staff {
    if (*s.Staff)[i].Token != nil && *(*s.Staff)[i].Token == token {
      return (*s.Staff)[i]
    }
  }
  return nil
}

type ModifyRows struct {
  Action string `json:"action"`
  DayID  string `json:"dayID"`
}

type ModifyProfileBody struct {
  ID string `json:"id"`
  FirstName string `json:"firstName"`
  LastName  string `json:"lastName"`
  NickName string `json:"nickName"`
  IdealShifts  string `json:"ideal-shifts"`
  Email  string `json:"email"`
  Phone  string `json:"phone"`
  ContactName  string `json:"contactName"`
  ContactPhone  string `json:"contactPhone"`
  TuesEarly  string `json:"Tues-early-avail"`
  TuesMid  string `json:"Tues-mid-avail"`
  TuesLate  string `json:"Tues-late-avail"`
  WedEarly  string `json:"Wed-early-avail"`
  WedMid  string `json:"Wed-mid-avail"`
  WedLate  string `json:"Wed-late-avail"`
  ThursEarly  string `json:"Thurs-early-avail"`
  ThursMid  string `json:"Thurs-mid-avail"`
  ThursLate  string `json:"Thurs-late-avail"`
  FriEarly  string `json:"Fri-early-avail"`
  FriMid  string `json:"Fri-mid-avail"`
  FriLate  string `json:"Fri-late-avail"`
  SatEarly  string `json:"Sat-early-avail"`
  SatMid  string `json:"Sat-mid-avail"`
  SatLate  string `json:"Sat-late-avail"`
  SunEarly  string `json:"Sun-early-avail"`
  SunMid  string `json:"Sun-mid-avail"`
  SunLate  string `json:"Sun-late-avail"`
  MonEarly  string `json:"Mon-early-avail"`
  MonMid  string `json:"Mon-mid-avail"`
  MonLate  string `json:"Mon-late-avail"`
}

func (s *Server) GetSessionUser(w http.ResponseWriter, r *http.Request) *StaffMember {
  sessionToken, ok := r.Context().Value(SESSION_KEY).(uuid.UUID)
  if !ok {
    log.Printf("Error retrieving token")
    return nil
  }
  staff := s.GetStaffByToken(sessionToken)
  if staff == nil {
    log.Printf("Error modifying profile")
    return nil
  }
  return staff
}

func (s *Server) HandleModifyProfile(w http.ResponseWriter, r *http.Request) {
  log.Println("Modify profile")
  var reqBody ModifyProfileBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  staffID, err := uuid.Parse(reqBody.ID)
  if err != nil {
    log.Printf("Invalid staffID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  staff := s.GetStaffByID(staffID)
  if (staff == nil) {
    return
  }
  activeStaff := s.GetSessionUser(w, r)
  if (activeStaff == nil) {
    return
  }
  if activeStaff.ID != staff.ID && !activeStaff.IsAdmin {
    log.Printf("Insufficient privilledges: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  staff.NickName = reqBody.NickName
  staff.FirstName = reqBody.FirstName
  staff.LastName = reqBody.LastName
  staff.Email = reqBody.Email
  staff.Phone = reqBody.Phone
  staff.ContactName = reqBody.ContactName
  staff.ContactPhone = reqBody.ContactPhone
  // This can fail but not from me
  staff.IdealShifts, _ = strconv.Atoi(reqBody.IdealShifts)

  staff.Availability = []DayAvailability{
    {
      Name: "Tues",
      Early: reqBody.TuesEarly == "on",
      Mid: reqBody.TuesMid == "on",
      Late: reqBody.TuesLate == "on",
    },
    {
      Name: "Wed",
      Early: reqBody.WedEarly == "on",
      Mid: reqBody.WedMid == "on",
      Late: reqBody.WedLate == "on",
    },
    {
      Name: "Thurs",
      Early: reqBody.ThursEarly == "on",
      Mid: reqBody.ThursMid == "on",
      Late: reqBody.ThursLate == "on",
    },
    {
      Name: "Fri",
      Early: reqBody.FriEarly == "on",
      Mid: reqBody.FriMid == "on",
      Late: reqBody.FriLate == "on",
    },
    {
      Name: "Sat",
      Early: reqBody.SatEarly == "on",
      Mid: reqBody.SatMid == "on",
      Late: reqBody.SatLate == "on",
    },
    {
      Name: "Sun",
      Early: reqBody.SunEarly == "on",
      Mid: reqBody.SunMid == "on",
      Late: reqBody.SunLate == "on",
    },
    {
      Name: "Mon",
      Early: reqBody.MonEarly == "on",
      Mid: reqBody.MonMid == "on",
      Late: reqBody.MonLate == "on",
    },
  }
  data := ProfileData{
    AdminRights: activeStaff.IsAdmin,
    StaffMember: *staff,
    RosterLive: s.IsLive,
    ShowUpdateSuccess: true,
  }
  SaveState(s)
  s.renderTemplate(w, "profile", data)
}

type ShiftWindow struct {
  Action string `json:"action"`
}


func (s *Server) HandleShiftWindow(w http.ResponseWriter, r *http.Request) {
  var reqBody ShiftWindow
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  switch reqBody.Action {
  case "+":
    s.StartDate = s.StartDate.AddDate(0, 0, 7)
  case "-":
    s.StartDate = s.StartDate.AddDate(0, 0, -7)
  default:
    today := time.Now()
    daysUntilTuesday := int(time.Tuesday - today.Weekday())
    if daysUntilTuesday <= 0 {
      daysUntilTuesday += 7
    }
    s.StartDate = today.AddDate(0, 0, daysUntilTuesday)
  }
  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  s.renderTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
}


func (s *Server) HandleModifyRows(w http.ResponseWriter, r *http.Request) {
  var reqBody ModifyRows
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }

  dayID, err := uuid.Parse(reqBody.DayID)
  if err != nil {
    log.Printf("Invalid dayID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }

  for i := range s.Days {
    if s.Days[i].ID == dayID {
      if reqBody.Action == "+" {
        s.Days[i].Rows = append(s.Days[i].Rows, newRow())
      } else {
        if len(s.Days[i].Rows) > 4 {
          s.Days[i].Rows = s.Days[i].Rows[:len(s.Days[i].Rows)-1]
        }
      }
      SaveState(s)
      s.renderTemplate(w, "rosterDay", MakeDayStruct(*s.Days[i], *s, *thisStaff))
      break
    }
  }
}

type DeleteAccountBody struct {
  ID            string `json:"id"`
}

func (s *Server) HandleDeleteAccount(w http.ResponseWriter, r *http.Request) {
  var reqBody DeleteAccountBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  accID, err := uuid.Parse(reqBody.ID)
  if err != nil {
    log.Printf("Invalid accID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  thisStaff := s.GetSessionUser(w, r)
  selfDelete := thisStaff.ID == accID
  for i, staff := range *s.Staff {
    if staff.ID == accID {
      newStaff := append((*s.Staff)[:i], (*s.Staff)[i+1:]...)
      s.Staff = &newStaff
      break
    }
  }

  for _, day := range s.Days {
    for _, row := range day.Rows {
      if row.Amelia.AssignedStaff != nil && *row.Amelia.AssignedStaff == accID {
        row.Amelia.AssignedStaff = nil
        row.Amelia.StaffString = nil
      }
      if row.Early.AssignedStaff != nil && *row.Early.AssignedStaff == accID {
        row.Early.AssignedStaff = nil
        row.Early.StaffString = nil
      }
      if row.Mid.AssignedStaff != nil && *row.Mid.AssignedStaff == accID {
        row.Mid.AssignedStaff = nil
        row.Mid.StaffString = nil
      }
      if row.Late.AssignedStaff != nil && *row.Late.AssignedStaff == accID {
        row.Late.AssignedStaff = nil
        row.Late.StaffString = nil
      }
    }
  }

  SaveState(s)
  if selfDelete {
    s.HandleGoogleLogout(w, r)
  } else {
    thisStaff := s.GetSessionUser(w, r)
    if (thisStaff == nil) {
      return
    }
    w.Header().Set("HX-Redirect", "/")
    w.WriteHeader(http.StatusOK)
  }
}

func MakeProfileStruct(rosterLive bool, staffMember StaffMember, adminRights bool) ProfileData {
  return ProfileData{
    StaffMember: staffMember,
    AdminRights: adminRights,
    RosterLive: rosterLive,
  }
}
