package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type RootStruct struct {
  *Server
  ActiveStaff StaffMember
  RosterWeek
  StaffState
}

func (s *Server) MakeRootStruct(activeStaff StaffMember, week RosterWeek) RootStruct {
  return RootStruct{
    s,
    activeStaff,
    week,
    s.LoadStaffState(),
  }
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

type DayStruct struct {
  RosterDay
  Staff *[]*StaffMember
  Date time.Time
  IsLive bool
  ActiveStaff StaffMember
}

func MakeDayStruct(isLive bool, day RosterDay, s *Server, activeStaff StaffMember) DayStruct {
  date :=  activeStaff.Config.RosterStartDate.AddDate(0, 0, day.Offset)
  return DayStruct{
    day,
    s.LoadStaffState().Staff,
    date,
    isLive,
    activeStaff,
  }
}

func (s *Server) CheckFlags(staffState StaffState, week RosterWeek) (RosterWeek) {
  allStaff := staffState.Staff
  for _, staff := range *allStaff {
    staff.CurrentShifts = 0
    s.SaveStaffMember(*staff)
  }
  for i, day := range week.Days {
    // Create a new map for each day to track occurrences of staff IDs within that day
    staffIDOccurrences := make(map[uuid.UUID]int)

    for _, row := range day.Rows {
      if day.AmeliaOpen && row.Amelia.AssignedStaff != nil {
        staffIDOccurrences[*row.Amelia.AssignedStaff]++
        staff := s.GetStaffByID(*row.Amelia.AssignedStaff)
        if staff != nil {
          staff.CurrentShifts += 1
          s.SaveStaffMember(*staff)
        }
      }
      if row.Early.AssignedStaff != nil {
        staffIDOccurrences[*row.Early.AssignedStaff]++
        staff := s.GetStaffByID(*row.Early.AssignedStaff)
        if staff != nil {
          staff.CurrentShifts += 1
          s.SaveStaffMember(*staff)
        }
      }
      if row.Mid.AssignedStaff != nil {
        staffIDOccurrences[*row.Mid.AssignedStaff]++
        staff := s.GetStaffByID(*row.Mid.AssignedStaff)
        if staff != nil {
          staff.CurrentShifts += 1
          s.SaveStaffMember(*staff)
        }
      }
      if row.Late.AssignedStaff != nil {
        staffIDOccurrences[*row.Late.AssignedStaff]++
        staff := s.GetStaffByID(*row.Late.AssignedStaff)
        if staff != nil {
          staff.CurrentShifts += 1
          s.SaveStaffMember(*staff)
        }
      }
    }

    for _, row := range day.Rows {
      row.Amelia.Flag = None
      row.Early.Flag = None
      row.Mid.Flag = None
      row.Late.Flag = None
      date := week.StartDate.AddDate(0, 0, day.Offset)

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
              if !staff.Availability[i].Mid && !staff.Availability[i].Late {
                row.Early.Flag = PrefRefuse
              } else {
                row.Early.Flag = PrefConflict
              }
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
              if !staff.Availability[i].Early && !staff.Availability[i].Late {
                row.Mid.Flag = PrefRefuse
              } else {
                row.Mid.Flag = PrefConflict
              }
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
              if !staff.Availability[i].Early && !staff.Availability[i].Mid {
                row.Late.Flag = PrefRefuse
              } else {
                row.Late.Flag = PrefConflict
              }
            }
          }
        }
      }
    }
  }
  return week
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

func MemberIsAssigned(activeID uuid.UUID, assignedID *uuid.UUID) bool {
  if assignedID == nil {
    return false
  }
  return *assignedID == activeID
}

func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
  if !thisStaff.IsAdmin && !week.IsLive {
    http.Redirect(w, r, "/profile", http.StatusSeeOther)
    return
  }
  s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
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

func (s *Server) HandleGoogleLogin(w http.ResponseWriter, r *http.Request) {
  if (DEV_MODE) {
    s.HandleGoogleCallback(w, r)
  } else {
    url := googleOauthConfig().AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.SetAuthURLParam("prompt", "select_account"))
    http.Redirect(w, r, url, http.StatusTemporaryRedirect)
  }
}

func (s *Server) HandleGoogleLogout(w http.ResponseWriter, r *http.Request) {
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

type GoogleCallbackBody struct {
  ID            string `json:"id"`
}

func (s *Server) HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
  var userInfo GoogleCallbackBody
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
    userInfo = GoogleCallbackBody{
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

  staffMember := s.GetStaffByGoogleID(userInfo.ID)
  if staffMember != nil {
    staffMember.Token = &sessionIdentifier
    s.SaveStaffMember(*staffMember)
  } else {
    isAdmin := len(*s.LoadStaffState().Staff) == 0
    staffMember = &StaffMember{
      ID:    uuid.New(),
      GoogleID:    userInfo.ID,
      FirstName:  "",
      IsAdmin: isAdmin,
      Token: &sessionIdentifier,
      Availability: emptyAvailability,
      Config: StaffConfig{
        TimesheetStartDate: GetLastTuesday(),
        RosterStartDate: GetNextTuesday(),
      },
    }
    s.SaveStaffMember(*staffMember)
  }

  http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) HandleModifyDescriptionSlot(w http.ResponseWriter, r *http.Request) {
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
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
  week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
  slot := week.GetSlotByID(slotID)
  if slot == nil {
    log.Printf("Invalid slotID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }

  slot.Description = descVal
  s.SaveRosterWeek(*week)
}

func (s *Server) HandleModifyTimeSlot(w http.ResponseWriter, r *http.Request) {
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
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
  week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
  slot := week.GetSlotByID(slotID)
  if slot == nil {
    log.Printf("Invalid slotID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }

  slot.StartTime = timeVal
  s.SaveRosterWeek(*week)
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
  week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
  slot := week.GetSlotByID(slotID)
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

  s.SaveRosterWeek(*week)
  s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

type ToggleAdminBody struct {
  ID            string `json:"id"`
}

func (s *Server) HandleToggleAdmin(w http.ResponseWriter, r *http.Request) {
  var reqBody ToggleAdminBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  accID, err := uuid.Parse(reqBody.ID)
  if err != nil {
    log.Printf("Invalid accID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  staffMember := s.GetStaffByID(accID)
  if staffMember != nil {
    staffMember.IsAdmin = !staffMember.IsAdmin
    s.SaveStaffMember(*staffMember)
  }
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
  s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

type ToggleHiddenBody struct {
  ID            string `json:"id"`
}

func (s *Server) HandleToggleHidden(w http.ResponseWriter, r *http.Request) {
  var reqBody ToggleHiddenBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  accID, err := uuid.Parse(reqBody.ID)
  if err != nil {
    log.Printf("Invalid accID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  staffMember := s.GetStaffByID(accID)
  if staffMember != nil {
    staffMember.IsHidden = !staffMember.IsHidden
    s.SaveStaffMember(*staffMember)
  }
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
  for _, day := range week.Days {
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

  s.SaveRosterWeek(*week)
  s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

func (s *Server) HandleToggleHideByIdeal(w http.ResponseWriter, r *http.Request) {
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  thisStaff.Config.HideByIdeal = !thisStaff.Config.HideByIdeal
  s.SaveStaffMember(*thisStaff)
  week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
  s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

func (s *Server) HandleToggleHideByPreferences(w http.ResponseWriter, r *http.Request) {
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  thisStaff.Config.HideByPrefs = !thisStaff.Config.HideByPrefs
  s.SaveStaffMember(*thisStaff)
  week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
  s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

func (s *Server) HandleToggleHideByLeave(w http.ResponseWriter, r *http.Request) {
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  thisStaff.Config.HideByLeave = !thisStaff.Config.HideByLeave
  s.SaveStaffMember(*thisStaff)
  week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
  s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

func (s *Server) HandleToggleLive(w http.ResponseWriter, r *http.Request) {
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
  week.IsLive = !week.IsLive
  s.SaveRosterWeek(*week)
  s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

type ToggleAmeliaBody struct {
  DayID            string `json:"dayID"`
}

func (s *Server) HandleToggleAmelia(w http.ResponseWriter, r *http.Request) {
  var reqBody ToggleAmeliaBody
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
  week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
  day := week.GetDayByID(dayID)
  if day == nil {
    log.Printf("Invalid dayID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  day.AmeliaOpen = !day.AmeliaOpen
  s.SaveRosterWeek(*week)
  s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

type ToggleClosedBody struct {
  DayID            string `json:"dayID"`
}

func (s *Server) HandleToggleClosed(w http.ResponseWriter, r *http.Request) {
  var reqBody ToggleClosedBody
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
  week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
  if week == nil {
    log.Printf("Invalid dayID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  day := week.GetDayByID(dayID)
  if day == nil {
    log.Printf("Invalid dayID: %v", err)
    w.WriteHeader(http.StatusBadRequest)
    return
  }
  day.IsClosed = !day.IsClosed
  s.SaveRosterWeek(*week)
  s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

type AddTrialBody struct {
  Name            string `json:"name"`
}

func (s *Server) HandleAddTrial(w http.ResponseWriter, r *http.Request) {
  var reqBody AddTrialBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  newStaff := StaffMember{
    ID:    uuid.New(),
    GoogleID:    "Trial",
    IsTrial:    true,
    FirstName:  reqBody.Name,
    Availability: emptyAvailability,
  }
  s.SaveStaffMember(newStaff)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
  s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

type ShiftWindowBody struct {
  Action string `json:"action"`
}

func (s *Server) HandleShiftWindow(w http.ResponseWriter, r *http.Request) {
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  var reqBody ShiftWindowBody
  if err := ReadAndUnmarshal(w, r, &reqBody); err != nil { return }
  switch reqBody.Action {
  case "+":
    thisStaff.Config.RosterStartDate = thisStaff.Config.RosterStartDate.AddDate(0, 0, 7)
  case "-":
    thisStaff.Config.RosterStartDate = thisStaff.Config.RosterStartDate.AddDate(0, 0, -7)
  default:
    thisStaff.Config.RosterStartDate = GetNextTuesday()
  }
  s.SaveStaffMember(*thisStaff)
  week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
  s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

type ModifyRowsBody struct {
  Action string `json:"action"`
  DayID  string `json:"dayID"`
}

func (s *Server) HandleModifyRows(w http.ResponseWriter, r *http.Request) {
  var reqBody ModifyRowsBody
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

  week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
  for i := range week.Days {
    if week.Days[i].ID == dayID {
      if reqBody.Action == "+" {
        week.Days[i].Rows = append(week.Days[i].Rows, newRow())
      } else {
        if len(week.Days[i].Rows) > 4 {
          week.Days[i].Rows = week.Days[i].Rows[:len(week.Days[i].Rows)-1]
        }
      }
      s.SaveRosterWeek(*week)
      s.renderTemplate(w, "rosterDay", MakeDayStruct(week.IsLive, *week.Days[i], s, *thisStaff))
      break
    }
  }
}


func duplicateRosterWeek(startDate time.Time, src RosterWeek) RosterWeek {
  newWeek := RosterWeek{
    ID:        uuid.New(),
    StartDate: startDate,
    IsLive:    false,
    Days:      make([]*RosterDay, len(src.Days)),
  }

  for i, day := range src.Days {
    newDay := &RosterDay{
      ID:          uuid.New(),
      DayName:     day.DayName,
      Colour:      day.Colour,
      Offset:      day.Offset,
      IsClosed:    day.IsClosed,
      AmeliaOpen:  day.AmeliaOpen,
      Rows:        make([]*Row, len(day.Rows)),
    }
    for j, row := range day.Rows {
      newRow := &Row{
        ID:    uuid.New(),
        Amelia: duplicateSlot(row.Amelia),
        Early:  duplicateSlot(row.Early),
        Mid:    duplicateSlot(row.Mid),
        Late:   duplicateSlot(row.Late),
      }
      newDay.Rows[j] = newRow
    }
    newWeek.Days[i] = newDay
  }

  return newWeek
}

func duplicateSlot(src Slot) Slot {
  var newAssignedStaff *uuid.UUID
  if src.AssignedStaff != nil {
    newStaffID := *src.AssignedStaff // Copy the UUID value
    newAssignedStaff = &newStaffID
  }

  var newStaffString *string
  if src.StaffString != nil {
    newString := *src.StaffString // Copy the string value
    newStaffString = &newString
  }

  return Slot{
    ID:            uuid.New(),
    StartTime:     src.StartTime,
    AssignedStaff: newAssignedStaff,
    StaffString:   newStaffString,
    Flag:          src.Flag,
    Description:   src.Description,
  }
}

func (s *Server) HandleImportRosterWeek(w http.ResponseWriter, r *http.Request) {
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  log.Println("Importing")
  lastWeekDate := thisStaff.Config.RosterStartDate.AddDate(0, 0, -7)
  lastWeek := s.LoadRosterWeek(lastWeekDate)
  thisWeek := duplicateRosterWeek(thisStaff.Config.RosterStartDate, *lastWeek)
  s.SaveRosterWeek(thisWeek)
  s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, thisWeek))
}
