package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"roster/cmd/db"
	"roster/cmd/utils"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type RootStruct struct {
	*Server
	ActiveStaff db.StaffMember
	db.RosterWeek
	Staff []*db.StaffMember
}

func (s *Server) MakeRootStruct(activeStaff db.StaffMember, week db.RosterWeek) RootStruct {
	return RootStruct{
		s,
		activeStaff,
		week,
		s.LoadAllStaff(),
	}
}

type DayStruct struct {
	db.RosterDay
	Staff       []*db.StaffMember
	Date        time.Time
	IsLive      bool
	ActiveStaff db.StaffMember
}

func MakeDayStruct(isLive bool, day db.RosterDay, s *Server, activeStaff db.StaffMember) DayStruct {
	date := activeStaff.Config.RosterStartDate.AddDate(0, 0, day.Offset)
	return DayStruct{
		day,
		s.LoadAllStaff(),
		date,
		isLive,
		activeStaff,
	}
}

func MemberIsAssigned(activeID uuid.UUID, assignedID *uuid.UUID) bool {
	if assignedID == nil {
		return false
	}
	return *assignedID == activeID
}

func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	log.Printf("roster Start Date: %v", thisStaff.Config.RosterStartDate)
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
	if DEV_MODE {
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
		SameSite: http.SameSiteLaxMode,
	})

	staff := s.GetSessionUser(w, r)
	if staff != nil {
		staff.Tokens = []uuid.UUID{}
	}
	w.Header().Set("HX-Redirect", "/landing")
	w.WriteHeader(http.StatusOK)
}

type GoogleCallbackBody struct {
	ID string `json:"id"`
}

func (s *Server) HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	var userInfo GoogleCallbackBody
	if !DEV_MODE {
		ctx := r.Context()
		code := r.URL.Query().Get("code")
		token, err := googleOauthConfig().Exchange(ctx, code)
		if err != nil {
			http.Error(w, "Failed to exchange token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		response, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)
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
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	err := s.CreateOrUpdateStaffGoogleID(userInfo.ID, sessionIdentifier)
	if err != nil {
		log.Printf("Error logging in with google: %v", err)
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) HandleModifyDescriptionSlot(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
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
	if thisStaff == nil {
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
	if thisStaff == nil {
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
	ID string `json:"id"`
}

func (s *Server) HandleToggleAdmin(w http.ResponseWriter, r *http.Request) {
	var reqBody ToggleAdminBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
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
	if thisStaff == nil {
		return
	}
	week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
	s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

type ToggleHiddenBody struct {
	ID string `json:"id"`
}

func (s *Server) HandleToggleHidden(w http.ResponseWriter, r *http.Request) {
	var reqBody ToggleHiddenBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
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
	if thisStaff == nil {
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
	if thisStaff == nil {
		return
	}
	thisStaff.Config.HideByIdeal = !thisStaff.Config.HideByIdeal
	s.SaveStaffMember(*thisStaff)
	week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
	s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

func (s *Server) HandleToggleHideByPreferences(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	thisStaff.Config.HideByPrefs = !thisStaff.Config.HideByPrefs
	s.SaveStaffMember(*thisStaff)
	week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
	s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

func (s *Server) HandleToggleHideByLeave(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	thisStaff.Config.HideByLeave = !thisStaff.Config.HideByLeave
	s.SaveStaffMember(*thisStaff)
	week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
	s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

func (s *Server) HandleToggleLive(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
	week.IsLive = !week.IsLive
	s.SaveRosterWeek(*week)
	s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
}

type ToggleAmeliaBody struct {
	DayID string `json:"dayID"`
}

func (s *Server) HandleToggleAmelia(w http.ResponseWriter, r *http.Request) {
	var reqBody ToggleAmeliaBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	dayID, err := uuid.Parse(reqBody.DayID)
	if err != nil {
		log.Printf("Invalid dayID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
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
	DayID string `json:"dayID"`
}

func (s *Server) HandleToggleClosed(w http.ResponseWriter, r *http.Request) {
	var reqBody ToggleClosedBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	dayID, err := uuid.Parse(reqBody.DayID)
	if err != nil {
		log.Printf("Invalid dayID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
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
	Name string `json:"name"`
}

func (s *Server) HandleAddTrial(w http.ResponseWriter, r *http.Request) {
	var reqBody AddTrialBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	s.CreateTrial(reqBody.Name)
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		// TODO: Handle error, also loading the roster week below
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
	if thisStaff == nil {
		return
	}
	var reqBody ShiftWindowBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	switch reqBody.Action {
	case "+":
		thisStaff.Config.RosterStartDate = thisStaff.Config.RosterStartDate.AddDate(0, 0, 7)
	case "-":
		thisStaff.Config.RosterStartDate = thisStaff.Config.RosterStartDate.AddDate(0, 0, -7)
	default:
		thisStaff.Config.RosterStartDate = utils.GetNextTuesday()
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
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}

	dayID, err := uuid.Parse(reqBody.DayID)
	if err != nil {
		log.Printf("Invalid dayID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}

	newDay, isLive := s.ChangeDayRowCount(thisStaff.Config.RosterStartDate, dayID, reqBody.Action)
	s.renderTemplate(w, "rosterDay", MakeDayStruct(isLive, *newDay, s, *thisStaff))
}

func duplicateRosterWeek(src db.RosterWeek, newWeek db.RosterWeek) db.RosterWeek {
	newDays := []db.RosterDay{}
	for _, day := range src.Days {
		newDay := db.RosterDay{
			ID:         uuid.New(),
			DayName:    day.DayName,
			Colour:     day.Colour,
			Offset:     day.Offset,
			IsClosed:   day.IsClosed,
			AmeliaOpen: day.AmeliaOpen,
			Rows:       []*db.Row{},
		}
		for _, row := range day.Rows {
			newRow := &db.Row{
				ID:     uuid.New(),
				Amelia: duplicateSlot(row.Amelia),
				Early:  duplicateSlot(row.Early),
				Mid:    duplicateSlot(row.Mid),
				Late:   duplicateSlot(row.Late),
			}
			newDay.Rows = append(newDay.Rows, newRow)
		}
		newDays = append(newDays, newDay)
	}
	newWeek.Days = newDays

	return newWeek
}

func duplicateSlot(src db.Slot) db.Slot {
	var newAssignedStaff *uuid.UUID
	if src.AssignedStaff != nil {
		newStaffID := *src.AssignedStaff
		newAssignedStaff = &newStaffID
	}

	var newStaffString *string
	if src.StaffString != nil {
		newString := *src.StaffString
		newStaffString = &newString
	}

	return db.Slot{
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
	if thisStaff == nil {
		return
	}
	log.Println("Importing")
	lastWeekDate := thisStaff.Config.RosterStartDate.AddDate(0, 0, -7)
	lastWeek := s.LoadRosterWeek(lastWeekDate)
	if lastWeek == nil {
		log.Println("No last week to duplicate")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	thisWeek := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
	if thisWeek == nil {
		thisWeek = &db.RosterWeek{
			ID:        uuid.New(),
			StartDate: thisStaff.Config.RosterStartDate,
		}
	}
	newWeek := duplicateRosterWeek(*lastWeek, *thisWeek)
	thisWeek = &newWeek
	s.SaveRosterWeek(*thisWeek)
	s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *thisWeek))
}
