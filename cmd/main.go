package main

import (
  "context"
  "strconv"
  "encoding/json"
  "fmt"
  "html/template"
  "io"
  "log"
  "net/http"
  "os"
  "strings"
  "time"

  "github.com/google/uuid"
  "github.com/joho/godotenv"
  "golang.org/x/oauth2"
  "golang.org/x/oauth2/google"
)

const STATE_FILE = "./data/state.json"
const SESSION_KEY = "sessionToken"
var DEV_MODE = false
const DEV_UUID = "00000000-0000-0000-0000-000000000000"

func googleOauthConfig() *oauth2.Config {
  return &oauth2.Config{
    ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
    ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
    RedirectURL:  os.Getenv("REDIRECT_URL"),
    Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email"},
    Endpoint:     google.Endpoint,
  }
}

type Highlight int

const (
  None Highlight = iota
  Duplicate
  PrefConflict
  LeaveConflict
)

type Server struct {
  CacheBust string
  Templates *template.Template
  ServerDisc
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

type DayAvailability struct {
  Name   string
  Early   bool
  Mid   bool
  Late   bool
}

type CustomDate struct {
  time.Time
}

func (cd *CustomDate) UnmarshalJSON(input []byte) error {
  strInput := strings.Trim(string(input), `"`)
  // Try parsing the date in the expected formats
  formats := []string{"2006-01-02", "2006-01-02T15:04:05Z", "2006-01-02T15:04:05.999999999Z07:00"}
  var parseErr error
  for _, format := range formats {
    var newTime time.Time
    newTime, parseErr = time.Parse(format, strInput)
    if parseErr == nil {
      cd.Time = newTime
      return nil
    }
  }

  // If none of the formats worked, return the last error
  return parseErr
}

type LeaveRequest struct {
  ID uuid.UUID
  CreationDate CustomDate
  Reason string	`json:"reason"`
  StartDate CustomDate	`json:"start-date"`
  EndDate CustomDate	`json:"end-date"`
}

type ProfileData struct {
  StaffMember
  RosterLive bool
  ShowUpdateSuccess bool
  ShowUpdateError bool
  ShowLeaveSuccess bool
  ShowLeaveError bool
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

type GoogleUserInfo struct {
  ID            string `json:"id"`
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
    "MemberIsAssigned": MemberIsAssigned,
  })
  state.Templates, err = state.Templates.ParseGlob("./www/*.html")
  if err != nil {
    return nil, err
  }
  return state, nil
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

func main() {
  if err := godotenv.Load(); err != nil {
    log.Printf("No .env file found")
  }
  s, err := LoadState(STATE_FILE)
  if err != nil {
    log.Fatalf("Error loading state: %v", err)
  }
  http.HandleFunc("/", s.VerifySession(s.HandleIndex))
  http.HandleFunc("/landing", s.HandleLanding)

  http.HandleFunc("/app.css", func(w http.ResponseWriter, r *http.Request) {
    http.ServeFile(w, r, "./www/app.css")
  })
  http.HandleFunc("/root", s.VerifySession(s.HandleRoot))
  http.HandleFunc("/submitLeave", s.VerifySession(s.HandleSubmitLeave))
  http.HandleFunc("/profile", s.VerifySession(s.HandleProfileIndex))
  http.HandleFunc("/profileBody", s.VerifySession(s.HandleProfile))
  http.HandleFunc("/auth/login", s.handleGoogleLogin)
  http.HandleFunc("/auth/logout", s.handleGoogleLogout)
  http.HandleFunc("/auth/callback", s.handleGoogleCallback)

  http.HandleFunc("/toggleHideByIdeal", s.VerifySession(s.handleToggleHideByIdeal))
  http.HandleFunc("/toggleHideByPreferences", s.VerifySession(s.handleToggleHideByPreferences))
  http.HandleFunc("/toggleHideByLeave", s.VerifySession(s.handleToggleHideByLeave))

  http.HandleFunc("/toggleAdmin", s.VerifySession(s.handleToggleAdmin))
  http.HandleFunc("/toggleHidden", s.VerifySession(s.handleToggleHidden))
  http.HandleFunc("/toggleLive", s.VerifySession(s.handleToggleLive))
  http.HandleFunc("/toggleAmelia", s.VerifySession(s.handleToggleAmelia))
  http.HandleFunc("/toggleClosed", s.VerifySession(s.handleToggleClosed))
  http.HandleFunc("/deleteAcc", s.VerifySession(s.handleDeleteAccount))
  http.HandleFunc("/addTrial", s.VerifySession(s.handleAddTrial))
  http.HandleFunc("/shiftWindow", s.VerifySession(s.HandleShiftWindow))
  http.HandleFunc("/modifyProfile", s.VerifySession(s.HandleModifyProfile))
  http.HandleFunc("/modifyRows", s.VerifySession(s.HandleModifyRows))
  http.HandleFunc("/modifySlot", s.VerifySession(s.HandleModifySlot))
  http.HandleFunc("/modifyTimeSlot", s.VerifySession(s.HandleModifyTimeSlot))
  http.HandleFunc("/modifyDescriptionSlot", s.VerifySession(s.HandleModifyDescriptionSlot))
  http.HandleFunc("/deleteLeaveReq", s.VerifySession(s.HandleDeleteLeaveReq))
  http.HandleFunc("/deleteAdminLeaveReq", s.VerifyAdmin(s.HandleAdminDeleteLeaveReq))

  log.Println(http.ListenAndServe(":6969", nil))
}

func (s *Slot) HasThisStaff(staffId uuid.UUID) bool {
  if s.AssignedStaff != nil && *s.AssignedStaff == staffId {
    return true
  }
  return false
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

type RootStruct struct {
  Server
  ActiveStaff StaffMember
}

func MemberIsAssigned(activeID uuid.UUID, assignedID *uuid.UUID) bool {
  if assignedID == nil {
    return false
  }
  return *assignedID == activeID
}

func MakeRootStruct(server Server, activeStaff StaffMember) RootStruct {
  return RootStruct{
    server,
    activeStaff,
  }
}

type DayStruct struct {
  RosterDay
  Staff *[]*StaffMember
  Date time.Time
  IsLive bool
  ActiveStaff StaffMember
  HideByIdeal bool
  HideByPrefs bool
  HideByLeave bool
}

func MakeDayStruct(day RosterDay, s Server, activeStaff StaffMember) DayStruct {
  date :=  s.StartDate.AddDate(0, 0, day.Offset)
  return DayStruct{
    day,
    s.Staff,
    date,
    s.IsLive,
    activeStaff,
    s.HideByIdeal,
    s.HideByPrefs,
    s.HideByLeave,
  }
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

func (s *Server) HandleAdminDeleteLeaveReq(w http.ResponseWriter, r *http.Request) {
  log.Println("Delete leave request")
  var reqBody DeleteLeaveBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  leaveID, err := uuid.Parse(reqBody.ID)
  if err != nil {
    log.Printf("Invalid leaveID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }

  found := false
  for _, staff := range *s.Staff {
    for i, leaveReq := range staff.LeaveRequests {
      if leaveReq.ID == leaveID {
        staff.LeaveRequests = append(staff.LeaveRequests[:i], staff.LeaveRequests[i+1:]...)
        SaveState(s)
        found = true
        break
      }
    }
    if found {
      break
    }
  }
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  err = s.Templates.ExecuteTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
  if err != nil {
    log.Printf("Error executing template: %v\n", err)
    http.Error(w, "Internal Server Error", http.StatusInternalServerError)
  }
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

  staff := s.GetSessionUser(w, r)
  if (staff == nil) {
    return
  }
  for i, leaveReq := range staff.LeaveRequests {
    if leaveReq.ID == leaveID {
      staff.LeaveRequests = append(staff.LeaveRequests[:i], staff.LeaveRequests[i+1:]...)
      break
    }
  }
  data := ProfileData{
    StaffMember: *staff,
    RosterLive: s.IsLive,
  }
  SaveState(s)
  err = s.Templates.ExecuteTemplate(w, "profile", data)
  if err != nil {
    log.Printf("Error executing template: %v\n", err)
    http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
  data := ProfileData{}
  if reqBody.StartDate.After(reqBody.EndDate.Time) {
    data.ShowLeaveError = true
  } else {
    data.ShowLeaveSuccess = true
    staff.LeaveRequests = append(staff.LeaveRequests, reqBody)
    SaveState(s)
  }
  data.StaffMember = *staff
  data.RosterLive = s.IsLive
  err := s.Templates.ExecuteTemplate(w, "profile", data)
  if err != nil {
    log.Printf("Error executing template: %v\n", err)
    http.Error(w, "Internal Server Error", http.StatusInternalServerError)
  }
}

func (s *Server) HandleProfileIndex(w http.ResponseWriter, r *http.Request) {
  err := s.Templates.ExecuteTemplate(w, "profileIndex", s.CacheBust)
  if err != nil {
    log.Fatalf("Error executing template: %v", err)
  }
}

func (s *Server) HandleLanding(w http.ResponseWriter, r *http.Request) {
  err := s.Templates.ExecuteTemplate(w, "landing", s.CacheBust)
  if err != nil {
    log.Fatalf("Error executing template: %v", err)
  }
}

func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
  err := s.Templates.ExecuteTemplate(w, "index", s.CacheBust)
  if err != nil {
    log.Fatalf("Error executing template: %v", err)
  }
}

func (s *Server) HandleProfile(w http.ResponseWriter, r *http.Request) {
  staff := s.GetSessionUser(w, r)
  if (staff == nil) {
    return
  }

  data := ProfileData{
    StaffMember: *staff,
    RosterLive: s.IsLive,
  }
  err := s.Templates.ExecuteTemplate(w, "profile", data)
  if err != nil {
    log.Printf("Error executing template: %v\n", err)
    http.Error(w, "Internal Server Error", http.StatusInternalServerError)
  }
}

func (s *Server) HandleRoot(w http.ResponseWriter, r *http.Request) {
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  if !thisStaff.IsAdmin && !s.IsLive {
    w.Header().Set("HX-Redirect", "/profile")
    w.WriteHeader(http.StatusOK)
    return
  }
  err := s.Templates.ExecuteTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
  if err != nil {
    log.Fatalf("Error executing template: %v", err)
  }
}

func (s *Server) GetStaffByToken(token uuid.UUID) *StaffMember {
  for i := range *s.Staff {
    if (*s.Staff)[i].Token != nil && *(*s.Staff)[i].Token == token {
      return (*s.Staff)[i]
    }
  }
  return nil
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

func (s *Server) HandleModifyDescriptionSlot(w http.ResponseWriter, r *http.Request) {
  if err := r.ParseForm(); err != nil {
    log.Printf("Error parsing form: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  slotIDStr := r.FormValue("slotID")
  descVal := r.FormValue("descVal")
  slotID, err := uuid.Parse(slotIDStr)
  if err != nil {
    log.Printf("Invalid SlotID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  slot := s.GetSlotByID(slotID)
  if slot == nil {
    log.Printf("Invalid slotID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }

  slot.Description = descVal
  SaveState(s)
}

func (s *Server) HandleModifyTimeSlot(w http.ResponseWriter, r *http.Request) {
  if err := r.ParseForm(); err != nil {
    log.Printf("Error parsing form: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  slotIDStr := r.FormValue("slotID")
  timeVal := r.FormValue("timeVal")
  slotID, err := uuid.Parse(slotIDStr)
  if err != nil {
    log.Printf("Invalid SlotID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  log.Printf("Modify %v timeslot id: %v", slotID, timeVal)
  slot := s.GetSlotByID(slotID)
  if slot == nil {
    log.Printf("Invalid slotID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }

  slot.StartTime = timeVal
  SaveState(s)
}

func (s *Server) HandleModifySlot(w http.ResponseWriter, r *http.Request) {
  if err := r.ParseForm(); err != nil {
    log.Printf("Error parsing form: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }

  slotIDStr := r.FormValue("slotID")
  staffIDStr := r.FormValue("staffID")
  slotID, err := uuid.Parse(slotIDStr)
  if err != nil {
    log.Printf("Invalid SlotID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  slot := s.GetSlotByID(slotID)
  if slot == nil {
    log.Printf("Invalid slotID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }

  staffID, err := uuid.Parse(staffIDStr)
  if err != nil {
    slot.AssignedStaff = nil
    slot.StaffString = nil
  } else {
    log.Printf("Modify %v slot id: %v, staffid: %v", slotID, slotID, staffID)
    member := s.GetStaffByID(staffID)
    if member != nil {
      slot.AssignedStaff = &member.ID
      if member.NickName != "" {
        slot.StaffString = &member.NickName
      } else {
        slot.StaffString = &member.FirstName
      }
    }
  }

  SaveState(s)
  err = s.Templates.ExecuteTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
  if err != nil {
    log.Fatalf("Error executing template: %v", err)
  }
}

type ModifyRows struct {
  Action string `json:"action"`
  DayID  string `json:"dayID"`
}

type ModifyProfileBody struct {
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
  staff := s.GetSessionUser(w, r)
  if (staff == nil) {
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
    StaffMember: *staff,
    RosterLive: s.IsLive,
    ShowUpdateSuccess: true,
  }
  SaveState(s)
  err := s.Templates.ExecuteTemplate(w, "profile", data)
  if err != nil {
    log.Printf("Error executing template: %v\n", err)
    http.Error(w, "Internal Server Error", http.StatusInternalServerError)
  }
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
  err := s.Templates.ExecuteTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
  if err != nil {
    log.Fatalf("Error executing template: %v", err)
  }
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
      err := s.Templates.ExecuteTemplate(w, "rosterDay", MakeDayStruct(*s.Days[i], *s, *thisStaff))
      if err != nil {
        log.Printf("Error executing template: %v", err)
        w.WriteHeader(http.StatusBadRequest)
        return
      }
      break
    }
  }
}

func (s *Server) handleGoogleLogout(w http.ResponseWriter, r *http.Request) {
  http.SetCookie(w, &http.Cookie{
    Name:     "session_token",
    Value:    "",
    Path:     "/",
    Expires:  time.Unix(0, 0),
    MaxAge:   -1,
    HttpOnly: true,
    Secure:   true,
    SameSite:  http.SameSiteLaxMode,
  })

  staff := s.GetSessionUser(w, r)
  if (staff != nil) {
    staff.Token = nil
  }
  w.Header().Set("HX-Redirect", "/landing")
  w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGoogleLogin(w http.ResponseWriter, r *http.Request) {
  if (DEV_MODE) {
    s.handleGoogleCallback(w, r)
  } else {
    url := googleOauthConfig().AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.SetAuthURLParam("prompt", "select_account"))
    http.Redirect(w, r, url, http.StatusTemporaryRedirect)
  }
}

type ToggleAdminBody struct {
  ID            string `json:"id"`
}

func (s *Server) handleToggleAdmin(w http.ResponseWriter, r *http.Request) {
  var reqBody ToggleAdminBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  accID, err := uuid.Parse(reqBody.ID)
  if err != nil {
    log.Printf("Invalid accID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  for _, staff := range *s.Staff {
    if staff.ID == accID {
      staff.IsAdmin = !staff.IsAdmin
      break
    }
  }
  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  err = s.Templates.ExecuteTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
  if err != nil {
    log.Fatalf("Error executing template: %v", err)
  }
}

type ToggleHiddenBody struct {
  ID            string `json:"id"`
}

func (s *Server) handleToggleHidden(w http.ResponseWriter, r *http.Request) {
  var reqBody ToggleHiddenBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  accID, err := uuid.Parse(reqBody.ID)
  if err != nil {
    log.Printf("Invalid accID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  for _, staff := range *s.Staff {
    if staff.ID == accID {
      staff.IsHidden = !staff.IsHidden
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
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  err = s.Templates.ExecuteTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
  if err != nil {
    log.Fatalf("Error executing template: %v", err)
  }
}

func (s *Server) handleToggleHideByIdeal(w http.ResponseWriter, r *http.Request) {
  s.HideByIdeal = !s.HideByIdeal
  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  err := s.Templates.ExecuteTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
  if err != nil {
    log.Printf("Error executing template: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
}

func (s *Server) handleToggleHideByPreferences(w http.ResponseWriter, r *http.Request) {
  s.HideByPrefs = !s.HideByPrefs
  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  err := s.Templates.ExecuteTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
  if err != nil {
    log.Printf("Error executing template: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
}

func (s *Server) handleToggleHideByLeave(w http.ResponseWriter, r *http.Request) {
  s.HideByLeave = !s.HideByLeave
  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  err := s.Templates.ExecuteTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
  if err != nil {
    log.Printf("Error executing template: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
}

func (s *Server) handleToggleLive(w http.ResponseWriter, r *http.Request) {
  s.IsLive = !s.IsLive
  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  err := s.Templates.ExecuteTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
  if err != nil {
    log.Printf("Error executing template: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
}

type ToggleAmeliaBody struct {
  DayID            string `json:"dayID"`
}

func (s *Server) handleToggleAmelia(w http.ResponseWriter, r *http.Request) {
  var reqBody ToggleAmeliaBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  dayID, err := uuid.Parse(reqBody.DayID)
  if err != nil {
    log.Printf("Invalid dayID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  day := s.GetDayByID(dayID)
  if day == nil {
    log.Printf("Invalid dayID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  day.AmeliaOpen = !day.AmeliaOpen
  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  err = s.Templates.ExecuteTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
  if err != nil {
    log.Fatalf("Error executing template: %v", err)
  }
}

type ToggleClosedBody struct {
  DayID            string `json:"dayID"`
}

func (s *Server) handleToggleClosed(w http.ResponseWriter, r *http.Request) {
  var reqBody ToggleClosedBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  dayID, err := uuid.Parse(reqBody.DayID)
  if err != nil {
    log.Printf("Invalid dayID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  day := s.GetDayByID(dayID)
  if day == nil {
    log.Printf("Invalid dayID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  day.IsClosed = !day.IsClosed
  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  err = s.Templates.ExecuteTemplate(w, "rosterDay", MakeDayStruct(*day, s.Staff, s.StartDate, s.IsLive, *thisStaff))
  if err != nil {
    log.Printf("Error executing template: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
}

type DeleteAccountBody struct {
  ID            string `json:"id"`
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
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
    s.handleGoogleLogout(w, r)
  } else {
    thisStaff := s.GetSessionUser(w, r)
    if (thisStaff == nil) {
      return
    }
    err = s.Templates.ExecuteTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
    if err != nil {
      log.Fatalf("Error executing template: %v", err)
    }
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

type AddTrialBody struct {
  Name            string `json:"name"`
}

func (s *Server) handleAddTrial(w http.ResponseWriter, r *http.Request) {
  var reqBody AddTrialBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  newStaff := (append(*s.Staff, &StaffMember{
    ID:    uuid.New(),
    GoogleID:    "Trial",
    IsTrial:    true,
    FirstName:  reqBody.Name,
    Availability: emptyAvailability,
  }))
  s.Staff = &newStaff

  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  err := s.Templates.ExecuteTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
  if err != nil {
    log.Fatalf("Error executing template: %v", err)
  }
}

func (s *Server) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
  var userInfo GoogleUserInfo
  if (!DEV_MODE) {
    ctx := r.Context()
    code := r.URL.Query().Get("code")
    token, err := googleOauthConfig().Exchange(ctx, code)
    if err != nil {
      http.Error(w, "Failed to exchange token: "+err.Error(), http.StatusInternalServerError)
      return
    }

    response, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token="+token.AccessToken)
    if err != nil {
      http.Error(w, "Failed to login: "+err.Error(), http.StatusInternalServerError)
      return
    }

    if err = json.NewDecoder(response.Body).Decode(&userInfo); err != nil {
      http.Error(w, "Error decoding user information: "+err.Error(), http.StatusInternalServerError)
      return
    }
  } else {
    userInfo = GoogleUserInfo{
      ID: "DEV",
    }
  }

  sessionIdentifier := uuid.New()
  http.SetCookie(w, &http.Cookie{
    Name:     "session_token",
    Value:    sessionIdentifier.String(),
    HttpOnly: true,
    Secure:   true,
    SameSite:  http.SameSiteLaxMode,
    Path:     "/",
  })

  found := false
  for i := range *s.Staff {
    if (*s.Staff)[i].GoogleID == userInfo.ID {
      found = true
      (*s.Staff)[i].Token = &sessionIdentifier
    }
  }

  if !found {
    isAdmin := len(*s.Staff) == 0
    new := (append(*s.Staff, &StaffMember{
      ID:    uuid.New(),
      GoogleID:    userInfo.ID,
      FirstName:  "",
      IsAdmin: isAdmin,
      Token: &sessionIdentifier,
      Availability: emptyAvailability,
    }))
    s.Staff = &new
  }

  SaveState(s)
  http.Redirect(w, r, "/", http.StatusSeeOther)
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
