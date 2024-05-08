package server

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

type RootStruct struct {
  Server
  ActiveStaff StaffMember
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
  s.renderTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
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

func (s *Server) HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
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
  s.renderTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
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
  s.renderTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
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
  s.renderTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
}

func (s *Server) HandleToggleHideByIdeal(w http.ResponseWriter, r *http.Request) {
  s.HideByIdeal = !s.HideByIdeal
  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  s.renderTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
}

func (s *Server) HandleToggleHideByPreferences(w http.ResponseWriter, r *http.Request) {
  s.HideByPrefs = !s.HideByPrefs
  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  s.renderTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
}

func (s *Server) HandleToggleHideByLeave(w http.ResponseWriter, r *http.Request) {
  s.HideByLeave = !s.HideByLeave
  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  s.renderTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
}

func (s *Server) HandleToggleLive(w http.ResponseWriter, r *http.Request) {
  s.IsLive = !s.IsLive
  SaveState(s)
  thisStaff := s.GetSessionUser(w, r)
  if (thisStaff == nil) {
    return
  }
  s.renderTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
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
  s.renderTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
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
  s.renderTemplate(w, "rosterDat", MakeDayStruct(*day, *s, *thisStaff))
}

type AddTrialBody struct {
  Name            string `json:"name"`
}

func (s *Server) HandleAddTrial(w http.ResponseWriter, r *http.Request) {
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
  s.renderTemplate(w, "root", MakeRootStruct(*s, *thisStaff))
}
