package server

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"roster/cmd/models"
	"roster/cmd/repository"
	"roster/cmd/utils"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type RootStruct struct {
	*Server
	ActiveStaff models.StaffMember
	models.RosterWeek
	Staff           []*models.StaffMember
	StaffShiftCount map[uuid.UUID]int
}

func (s *Server) MakeRootStruct(activeStaff models.StaffMember, week models.RosterWeek) RootStruct {
	allStaff, err := s.Repos.Staff.LoadAllStaff()
	if err != nil {
		utils.PrintError(err, "Failed to load all staff")
		allStaff = []*models.StaffMember{}
	}

	// Calculate shift counts for each staff member (once per week)
	staffShiftCount := make(map[uuid.UUID]int)
	for _, staff := range allStaff {
		staffShiftCount[staff.ID] = week.CountShiftsForStaff(staff.ID)
	}

	return RootStruct{
		s,
		activeStaff,
		week,
		allStaff,
		staffShiftCount,
	}
}

type DayStruct struct {
	models.RosterDay
	Staff       []*models.StaffMember
	Date        time.Time
	IsLive      bool
	ActiveStaff models.StaffMember
}

func MakeDayStruct(isLive bool, day models.RosterDay, s *Server, activeStaff models.StaffMember) DayStruct {
	date := utils.WeekStartFromOffset(activeStaff.Config.RosterDateOffset).AddDate(0, 0, day.Offset)
	allStaff, err := s.Repos.Staff.LoadAllStaff()
	if err != nil {
		utils.PrintError(err, "Failed to load all staff")
		allStaff = []*models.StaffMember{}
	}
	return DayStruct{
		day,
		allStaff,
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
		s.HandleGoogleLogout(w, r)
		utils.PrintLog("Couldn't find staff member")
		return
	}
	week, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Error creating staff member")
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
	http.Redirect(w, r, "/landing", http.StatusSeeOther)
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

	sessionToken := uuid.New()
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    sessionToken.String(),
		Expires:  time.Now().AddDate(10, 0, 0),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	staffMember, err := s.Repos.Staff.GetStaffByGoogleID(userInfo.ID)
	if err != nil {
		utils.PrintError(err, "Failed to get staff by google id")
		http.Redirect(w, r, "/landing", http.StatusSeeOther)
		return
	} else if staffMember == nil {
		utils.PrintLog("Creating new staff member")
		err := s.Repos.Staff.CreateStaffMember(userInfo.ID, sessionToken)
		if err != nil {
			utils.PrintError(err, "Error creating staff member")
			http.Redirect(w, r, "/landing", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/newAccount", http.StatusSeeOther)
	} else {
		utils.PrintLog("Updating staff token ID")
		err := s.Repos.Staff.UpdateStaffToken(staffMember, sessionToken)
		if err != nil {
			utils.PrintError(err, "Error logging in with google")
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (s *Server) HandleCreateAccount(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		utils.PrintLog("No database entry for new account")
		http.Redirect(w, r, "/landing", http.StatusSeeOther)
		return
	}
	var reqBody ModifyProfileBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		utils.PrintError(err, "Error parsing create account body")
		http.Redirect(w, r, "/landing", http.StatusSeeOther)
		return
	}
	updatedStaff := s.ApplyModifyProfileBody(reqBody, *thisStaff, false)
	if err := s.Repos.Staff.SaveStaffMember(updatedStaff); err != nil {
		utils.PrintError(err, "Error creating staff member")
		http.Redirect(w, r, "/landing", http.StatusSeeOther)
		return
	}
	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) HandleNewAccount(w http.ResponseWriter, r *http.Request) {
	sessionToken := GetTokenFromCookies(r)
	if sessionToken == nil {
		utils.PrintLog("No token for new account")
		http.Redirect(w, r, "/landing", http.StatusSeeOther)
		return
	}
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		utils.PrintLog("No database entry for new account")
		http.Redirect(w, r, "/landing", http.StatusSeeOther)
		return
	}
	if thisStaff.FirstName != "" {
		utils.PrintLog("Account is already initialised")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	data := ProfileIndexData{
		CacheBust:   s.CacheBust,
		StaffMember: *thisStaff,
		AdminRights: thisStaff.IsManagerRole(),
		RosterLive:  false,
	}
	s.renderTemplate(w, "newAccount", data)
}

func (s *Server) HandleModifyDescriptionSlot(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		utils.PrintError(err, "Error parsing form")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	slotIDStr := r.FormValue("slotID")
	descVal := r.FormValue("descVal")
	slotID, err := uuid.Parse(slotIDStr)
	if err != nil {
		utils.PrintError(err, "Invalid SlotID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	week, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to get roster week")
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
	slot := week.GetSlotByID(slotID)
	if slot == nil {
		utils.PrintError(err, "Invalid slotID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	slot.Description = descVal
	s.Repos.RosterWeek.SaveRosterWeek(week)
}

func (s *Server) HandleModifyTimeSlot(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		utils.PrintError(err, "Error parsing form")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	slotIDStr := r.FormValue("slotID")
	timeVal := r.FormValue("timeVal")
	slotID, err := uuid.Parse(slotIDStr)
	if err != nil {
		utils.PrintError(err, "Invalid SlotID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	utils.PrintLog("Modify %v timeslot id: %v", slotID, timeVal)
	week, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		return
	}

	slot := week.GetSlotByID(slotID)
	if slot == nil {
		utils.PrintError(err, "Invalid slotID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	slot.StartTime = timeVal
	s.Repos.RosterWeek.SaveRosterWeek(week)
}

func (s *Server) HandleModifySlot(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		utils.PrintError(err, "Error parsing form")
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
		utils.PrintError(err, "Invalid SlotID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	week, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		return
	}
	slot := week.GetSlotByID(slotID)
	if slot == nil {
		utils.PrintError(err, "Invalid slotID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	staffID, err := uuid.Parse(staffIDStr)
	if err != nil {
		slot.AssignedStaff = nil
		slot.StaffString = nil
	} else {
		utils.PrintLog("Modify %v slot id: %v, staffid: %v", slotID, slotID, staffID)
		member, err := s.Repos.Staff.GetStaffByID(staffID)
		if err != nil {
			utils.PrintError(err, "failed to get staff by ID")
		} else {
			slot.AssignedStaff = &member.ID
			if member.NickName != "" {
				slot.StaffString = &member.NickName
			} else {
				slot.StaffString = &member.FirstName
			}
		}
	}

	// TODO: Make the flags check here
	allStaff, err := s.Repos.Staff.LoadAllStaff()
	if err != nil {
		utils.PrintError(err, "Failed to load all staff")
		return
	}

	checkedWeek := week.CheckFlags(allStaff)
	week = &checkedWeek

	s.Repos.RosterWeek.SaveRosterWeek(week)
	s.renderTemplate(w, "rosterMainContainer", s.MakeRootStruct(*thisStaff, *week))
}

// TODO: Make these toggles consolidated
type ToggleKitchenBody struct {
	ID string `json:"id"`
}

func (s *Server) HandleToggleKitchen(w http.ResponseWriter, r *http.Request) {
	var reqBody ToggleKitchenBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		utils.PrintError(err, "Failed to unmarshal body")
		return
	}
	accID, err := uuid.Parse(reqBody.ID)
	if err != nil {
		utils.PrintError(err, "Invalid accID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	staffMember, err := s.Repos.Staff.GetStaffByID(accID)
	if err != nil {
		utils.PrintError(err, "failed to get staff by ID")
	} else {
		staffMember.IsKitchen = !staffMember.IsKitchen
		s.Repos.Staff.SaveStaffMember(*staffMember)
	}
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		utils.PrintLog("Couldn't find staff")
		return
	}
	week, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		return
	}
	s.renderTemplate(w, "rosterMainContainer", s.MakeRootStruct(*thisStaff, *week))
}

type SetRoleBody struct {
	ID   string `json:"id"`
	Role int    `json:"role"`
}

func (s *Server) HandleSetRole(w http.ResponseWriter, r *http.Request) {
	var reqBody SetRoleBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	accID, err := uuid.Parse(reqBody.ID)
	if err != nil {
		utils.PrintError(err, "Invalid accID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if reqBody.Role < int(models.Staff) || reqBody.Role > int(models.AdminRole) {
		utils.PrintError(fmt.Errorf("invalid role"), "Invalid role value")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	staffMember, err := s.Repos.Staff.GetStaffByID(accID)
	if err != nil || staffMember == nil {
		utils.PrintError(err, "failed to get staff by ID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	staffMember.Role = models.StaffRole(reqBody.Role)
	// Keep legacy flag aligned until templates are updated
	staffMember.IsAdmin = staffMember.Role >= models.Manager
	s.Repos.Staff.SaveStaffMember(*staffMember)

	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	week, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		return
	}
	s.renderTemplate(w, "rosterMainContainer", s.MakeRootStruct(*thisStaff, *week))
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
		utils.PrintError(err, "Invalid accID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	staffMember, err := s.Repos.Staff.GetStaffByID(accID)
	if err != nil {
		utils.PrintError(err, "failed to get staff by ID")
	} else {
		staffMember.IsHidden = !staffMember.IsHidden
		s.Repos.Staff.SaveStaffMember(*staffMember)
	}
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	week, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		return
	}
	for _, day := range week.Days {
		for _, row := range day.Rows {
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

	s.Repos.RosterWeek.SaveRosterWeek(week)
	s.renderTemplate(w, "rosterMainContainer", s.MakeRootStruct(*thisStaff, *week))
}

func (s *Server) HandleToggleHideByIdeal(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	thisStaff.Config.HideByIdeal = !thisStaff.Config.HideByIdeal
	s.Repos.Staff.SaveStaffMember(*thisStaff)
	week, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		return
	}
	s.renderTemplate(w, "rosterMainContainer", s.MakeRootStruct(*thisStaff, *week))
}

func (s *Server) HandleToggleHideByPreferences(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	thisStaff.Config.HideByPrefs = !thisStaff.Config.HideByPrefs
	s.Repos.Staff.SaveStaffMember(*thisStaff)
	week, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		return
	}
	s.renderTemplate(w, "rosterMainContainer", s.MakeRootStruct(*thisStaff, *week))
}

func (s *Server) HandleToggleHideByLeave(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	thisStaff.Config.HideByLeave = !thisStaff.Config.HideByLeave
	s.Repos.Staff.SaveStaffMember(*thisStaff)
	week, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		return
	}
	s.renderTemplate(w, "rosterMainContainer", s.MakeRootStruct(*thisStaff, *week))
}

func (s *Server) HandleToggleHideStaffList(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	thisStaff.Config.HideStaffList = !thisStaff.Config.HideStaffList
	s.Repos.Staff.SaveStaffMember(*thisStaff)
	week, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		return
	}
	s.renderTemplate(w, "rosterMainContainer", s.MakeRootStruct(*thisStaff, *week))
}

func (s *Server) HandleToggleLive(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	week, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		return
	}
	week.IsLive = !week.IsLive
	s.Repos.RosterWeek.SaveRosterWeek(week)
	s.renderTemplate(w, "rosterMainContainer", s.MakeRootStruct(*thisStaff, *week))
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
		utils.PrintError(err, "Invalid dayID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	week, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	day := week.GetDayByID(dayID)
	if day == nil {
		utils.PrintError(err, "Invalid dayID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	day.IsClosed = !day.IsClosed
	s.Repos.RosterWeek.SaveRosterWeek(week)
	s.renderTemplate(w, "rosterMainContainer", s.MakeRootStruct(*thisStaff, *week))
}

type AddTrialBody struct {
	Name string `json:"name"`
}

func (s *Server) HandleAddTrial(w http.ResponseWriter, r *http.Request) {
	var reqBody AddTrialBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	s.Repos.Staff.CreateTrial(reqBody.Name)
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		// TODO: Handle error, also loading the roster week below
		return
	}
	week, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	s.renderTemplate(w, "rosterMainContainer", s.MakeRootStruct(*thisStaff, *week))
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
		thisStaff.Config.RosterDateOffset = thisStaff.Config.RosterDateOffset + 1
	case "-":
		thisStaff.Config.RosterDateOffset = thisStaff.Config.RosterDateOffset - 1
	default:
		thisStaff.Config.RosterDateOffset = utils.WeekOffsetFromDate(utils.GetLastTuesday())
	}
	s.Repos.Staff.SaveStaffMember(*thisStaff)
	week, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	s.renderTemplate(w, "rosterMainContainer", s.MakeRootStruct(*thisStaff, *week))
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
		utils.PrintError(err, "Invalid dayID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}

	newDay, isLive, err := s.Repos.RosterWeek.ChangeDayRowCount(thisStaff.Config.RosterDateOffset, dayID, reqBody.Action)
	if err != nil {
		utils.PrintError(err, "Failed to change day row count")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	s.renderTemplate(w, "rosterDay", MakeDayStruct(isLive, *newDay, s, *thisStaff))
}

func duplicateRosterWeek(src models.RosterWeek, newWeek models.RosterWeek) models.RosterWeek {
	newDays := []*models.RosterDay{}
	for _, day := range src.Days {
		newDay := models.RosterDay{
			ID:       uuid.New(),
			DayName:  day.DayName,
			Colour:   day.Colour,
			Offset:   day.Offset,
			IsClosed: day.IsClosed,
			Rows:     []*models.Row{},
		}
		for _, row := range day.Rows {
			newRow := &models.Row{
				ID:    uuid.New(),
				Early: duplicateSlot(row.Early),
				Mid:   duplicateSlot(row.Mid),
				Late:  duplicateSlot(row.Late),
			}
			newDay.Rows = append(newDay.Rows, newRow)
		}
		newDays = append(newDays, &newDay)
	}
	newWeek.Days = newDays

	return newWeek
}

func duplicateSlot(src models.Slot) models.Slot {
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

	return models.Slot{
		ID:            uuid.New(),
		StartTime:     src.StartTime,
		AssignedStaff: newAssignedStaff,
		StaffString:   newStaffString,
		Flag:          src.Flag,
		Description:   src.Description,
	}
}

func minTime(t1, t2 time.Time) time.Time {
	if t1.Before(t2) {
		return t1
	}
	return t2
}

func maxTime(t1, t2 time.Time) time.Time {
	if t1.After(t2) {
		return t1
	}
	return t2
}

func GetWorkFromEntry(windowStart time.Time, windowEnd time.Time, entry models.TimesheetEntry) float64 {
	shiftStart := maxTime(windowStart, entry.ShiftStart)
	shiftEnd := minTime(windowEnd, entry.ShiftEnd)
	if shiftStart.After(shiftEnd) {
		return 0.0
	}
	overlappedShiftDuration := shiftEnd.Sub(shiftStart).Hours()
	if entry.HasBreak {
		breakWindowStart := maxTime(shiftStart, entry.BreakStart)
		breakWindowEnd := minTime(shiftEnd, entry.BreakEnd)
		if breakWindowStart.After(breakWindowEnd) {
			breakWindowStart = breakWindowEnd
		}
		overlappedBreakDuration := breakWindowEnd.Sub(breakWindowStart).Hours()
		return overlappedShiftDuration - overlappedBreakDuration
	}
	return overlappedShiftDuration
}

type KitchenShiftHours struct {
	Chef float64
}

type ShiftHours struct {
	Manager    float64
	Staff      float64
	Amelia     float64
	Salary     float64
	Deliveries float64
}

type DayIdx int

const (
	Tuesday DayIdx = iota
	Wednesday
	Thursday
	Friday
	Saturday
	Sunday
	Monday
)

type PayLevel int

const (
	Level2 PayLevel = iota
	Level3
	Level4
	Level5
)

type StaffPayData struct {
	Level2Hrs [7]DayBreakdown
	Level3Hrs [7]DayBreakdown
	Level4Hrs [7]DayBreakdown
	Level5Hrs [7]DayBreakdown
	General   [7]DayBreakdown
	Kitchen   [7]DayBreakdown
}

type DayBreakdown struct {
	OrdinaryHrs float64
	EveningHrs  float64
	After12Hrs  float64
}

func ApplyEntryToLevel(dayBreakdown DayBreakdown, thisDate time.Time, entry models.TimesheetEntry) DayBreakdown {
	ordinaryWindowStart := thisDate.Add(time.Duration(7) * time.Hour)
	ordinaryWindowEnd := thisDate.Add(time.Duration(19) * time.Hour)
	eveningWindowStart := ordinaryWindowEnd
	eveningWindowEnd := thisDate.Add(time.Duration(24) * time.Hour)
	after12WindowStart := eveningWindowEnd
	after12WindowEnd := thisDate.Add(time.Duration(31) * time.Hour)

	dayBreakdown.OrdinaryHrs += GetWorkFromEntry(ordinaryWindowStart, ordinaryWindowEnd, entry)
	dayBreakdown.EveningHrs += GetWorkFromEntry(eveningWindowStart, eveningWindowEnd, entry)
	dayBreakdown.After12Hrs += GetWorkFromEntry(after12WindowStart, after12WindowEnd, entry)
	return dayBreakdown
}

func AddEntryToPaydata(entry models.TimesheetEntry, thisDate time.Time, day DayIdx, payData StaffPayData) StaffPayData {
	if entry.ShiftType == models.Bar || entry.ShiftType == models.Deliveries || entry.ShiftType == models.Admin {
		payData.Level2Hrs[day] = ApplyEntryToLevel(payData.Level2Hrs[day], thisDate, entry)
	} else if entry.ShiftType == models.DayManager {
		if day != Friday && day != Saturday && day != Sunday {
			payData.Level3Hrs[day] = ApplyEntryToLevel(payData.Level3Hrs[day], thisDate, entry)
		} else {
			// day == Friday, Saturday or Sunday
			payData.Level4Hrs[day] = ApplyEntryToLevel(payData.Level4Hrs[day], thisDate, entry)
		}
	} else if entry.ShiftType == models.NightManager {
		payData.Level5Hrs[day] = ApplyEntryToLevel(payData.Level5Hrs[day], thisDate, entry)
	} else if entry.ShiftType == models.AmeliaSupervisor {
		payData.Level4Hrs[day] = ApplyEntryToLevel(payData.Level4Hrs[day], thisDate, entry)
	} else if entry.ShiftType == models.GeneralManagement {
		payData.General[day] = ApplyEntryToLevel(payData.General[day], thisDate, entry)
	} else if entry.ShiftType == models.Kitchen {
		payData.Kitchen[day] = ApplyEntryToLevel(payData.Kitchen[day], thisDate, entry)
	}
	return payData
}

func (s *Server) getSessionUserAndEntries(w http.ResponseWriter, r *http.Request) (*models.StaffMember, *[]*models.TimesheetEntry, bool) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		utils.PrintLog("Couldn't find session user")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return nil, nil, false
	}
	entries, err := s.Repos.Timesheet.GetTimesheetWeek(thisStaff.Config.TimesheetDateOffset)
	if err != nil {
		utils.PrintError(err, "No timesheet entries to export")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return nil, nil, false
	}
	return thisStaff, entries, true
}

func writeRecordsToCSV(staffData map[uuid.UUID]StaffPayData, allStaff []*models.StaffMember, writer *csv.Writer, reportType string) {
	header := []string{
		"Employee",
		"Tues Ord", "Tues 7-12", "Tues 12+",
		"Wed Ord", "Wed 7-12", "Wed 12+",
		"Thurs Ord", "Thurs 7-12", "Thurs 12+",
		"Fri Ord", "Fri 7-12", "Fri 12+",
		"Sat Ord", "Sat 12+",
		"Sun Ord",
		"Mon Ord", "Mon 7-12", "Mon 12+",
	}
	if err := writer.Write(header); err != nil {
		utils.PrintError(err, "Error writing kitchen report header")
	}
	reportRows := [][]string{}
	for staffID, payData := range staffData {
		staffMember := models.GetStaffFromList(staffID, allStaff)
		if staffMember == nil {
			utils.PrintLog("Missing staffID")
			continue
		}
		fullName := strings.TrimSpace(staffMember.LastName) + ", " + strings.TrimSpace(staffMember.FirstName)

		if reportType == "kitchen" {
			if hasHours(payData.Kitchen) {
				reportRows = append(reportRows, BuildReportRecord(payData.Kitchen, fullName+" Kitchen"))
			}
		} else if reportType == "evan" {
			if hasHours(payData.Level2Hrs) {
				reportRows = append(reportRows, BuildReportRecord(payData.Level2Hrs, fullName))
			}
			if hasHours(payData.Level3Hrs) {
				reportRows = append(reportRows, BuildReportRecord(payData.Level3Hrs, fullName+" LVL 3"))
			}
			if hasHours(payData.Level4Hrs) {
				reportRows = append(reportRows, BuildReportRecord(payData.Level4Hrs, fullName+" LVL 4"))
			}
			if hasHours(payData.Level5Hrs) {
				reportRows = append(reportRows, BuildReportRecord(payData.Level5Hrs, fullName+" LVL 5"))
			}
			if hasHours(payData.General) {
				reportRows = append(reportRows, BuildReportRecord(payData.General, fullName+" Salary"))
			}
		}
	}
	sort.Slice(reportRows, func(i, j int) bool {
		return reportRows[i][0] < reportRows[j][0]
	})
	for _, row := range reportRows {
		if err := writer.Write(row); err != nil {
			utils.PrintError(err, "Error writing record")
		}
	}
	writer.Flush()
}

func processEntries(thisStaff models.StaffMember, entries []*models.TimesheetEntry, allStaff []*models.StaffMember) map[uuid.UUID]StaffPayData {
	staffData := map[uuid.UUID]StaffPayData{}

	startOfWeekUTC := utils.WeekStartFromOffset(thisStaff.Config.TimesheetDateOffset)

	for day := Tuesday; day <= 6; day++ {
		currentDayUTC := startOfWeekUTC.AddDate(0, 0, int(day))
		// Convert to Local Time (00:00 Local) to ensure windows align with shifts which are stored in Local Time
		thisDate := time.Date(
			currentDayUTC.Year(),
			currentDayUTC.Month(),
			currentDayUTC.Day(),
			0, 0, 0, 0,
			time.Local,
		)

		for _, entry := range entries {
			if !entry.Approved {
				continue
			}
			staffMember := models.GetStaffFromList(entry.StaffID, allStaff)
			if staffMember == nil || staffMember.IsTrial {
				utils.PrintLog("Missing staffmember")
				continue
			}

			payData, exists := staffData[entry.StaffID]
			if !exists {
				payData = StaffPayData{}
				staffData[entry.StaffID] = payData
			}
			staffData[entry.StaffID] = AddEntryToPaydata(*entry, thisDate, day, payData)
		}
	}
	return staffData
}

func (s *Server) HandleExportKitchenReport(w http.ResponseWriter, r *http.Request) {
	thisStaff, entries, ok := s.getSessionUserAndEntries(w, r)
	if !ok {
		return
	}

	allStaff, err := s.Repos.Staff.LoadAllStaff()
	if err != nil {
		utils.PrintError(err, "Failed to load all staff")
		allStaff = []*models.StaffMember{}
	}
	staffData := processEntries(*thisStaff, *entries, allStaff)

	var fileBuffer bytes.Buffer
	writer := csv.NewWriter(&fileBuffer)

	writeRecordsToCSV(staffData, allStaff, writer, "kitchen")
	if err := writer.Error(); err != nil {
		utils.PrintError(err, "Error creating kitchen report")
	}

	// Set the appropriate headers
	w.Header().Set("Content-Type", "text/csv")
	formattedDate := utils.WeekStartFromOffset(thisStaff.Config.TimesheetDateOffset).Format("2006-01-02")
	reportFilename := "kitchen_staff_hrs_starting-" + formattedDate + ".csv"
	w.Header().Set("Content-Disposition", "attachment;filename="+reportFilename)
	w.Header().Set("Content-Length", strconv.Itoa(len(fileBuffer.Bytes())))

	// Write the zip file to the response
	if _, err := w.Write(fileBuffer.Bytes()); err != nil {
		http.Error(w, "Failed to write file to response", http.StatusInternalServerError)
		return
	}
}

func (s *Server) HandleExportEvanReport(w http.ResponseWriter, r *http.Request) {
	thisStaff, entries, ok := s.getSessionUserAndEntries(w, r)
	if !ok {
		return
	}

	allStaff, err := s.Repos.Staff.LoadAllStaff()
	if err != nil {
		utils.PrintError(err, "Failed to load all staff")
		allStaff = []*models.StaffMember{}
	}
	staffData := processEntries(*thisStaff, *entries, allStaff)

	var fileBuffer bytes.Buffer
	writer := csv.NewWriter(&fileBuffer)

	writeRecordsToCSV(staffData, allStaff, writer, "evan")
	if err := writer.Error(); err != nil {
		utils.PrintError(err, "Error creating evan report")
	}

	// Set the appropriate headers
	w.Header().Set("Content-Type", "text/csv")
	formattedDate := utils.WeekStartFromOffset(thisStaff.Config.TimesheetDateOffset).Format("2006-01-02")
	reportFilename := "staff_hrs_starting-" + formattedDate + ".csv"
	w.Header().Set("Content-Disposition", "attachment;filename="+reportFilename)
	w.Header().Set("Content-Length", strconv.Itoa(len(fileBuffer.Bytes())))

	// Write the zip file to the response
	if _, err := w.Write(fileBuffer.Bytes()); err != nil {
		http.Error(w, "Failed to write file to response", http.StatusInternalServerError)
		return
	}
}

func hasHours(hours [7]DayBreakdown) bool {
	for i := 0; i < 7; i++ {
		if hours[i].OrdinaryHrs+hours[i].EveningHrs+hours[i].After12Hrs > 0 {
			return true
		}
	}
	return false
}

func BuildReportRecord(hours [7]DayBreakdown, name string) []string {
	return []string{
		name,
		fmt.Sprintf("%.2f", hours[Tuesday].OrdinaryHrs),
		fmt.Sprintf("%.2f", hours[Tuesday].EveningHrs),
		fmt.Sprintf("%.2f", hours[Tuesday].After12Hrs),

		fmt.Sprintf("%.2f", hours[Wednesday].OrdinaryHrs),
		fmt.Sprintf("%.2f", hours[Wednesday].EveningHrs),
		fmt.Sprintf("%.2f", hours[Wednesday].After12Hrs),

		fmt.Sprintf("%.2f", hours[Thursday].OrdinaryHrs),
		fmt.Sprintf("%.2f", hours[Thursday].EveningHrs),
		fmt.Sprintf("%.2f", hours[Thursday].After12Hrs),

		fmt.Sprintf("%.2f", hours[Friday].OrdinaryHrs),
		fmt.Sprintf("%.2f", hours[Friday].EveningHrs),
		fmt.Sprintf("%.2f", hours[Friday].After12Hrs),

		fmt.Sprintf("%.2f", hours[Saturday].OrdinaryHrs+
			hours[Saturday].EveningHrs),
		fmt.Sprintf("%.2f", hours[Saturday].After12Hrs),

		fmt.Sprintf("%.2f", hours[Sunday].OrdinaryHrs+
			hours[Sunday].EveningHrs+
			hours[Sunday].After12Hrs),

		fmt.Sprintf("%.2f", hours[Monday].OrdinaryHrs),
		fmt.Sprintf("%.2f", hours[Monday].EveningHrs),
		fmt.Sprintf("%.2f", hours[Monday].After12Hrs),
	}
}

func (s *Server) HandleExportWageReport(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		utils.PrintLog("Couldn't find session user")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	entries, err := s.Repos.Timesheet.GetTimesheetWeek(thisStaff.Config.TimesheetDateOffset)
	if err != nil {
		utils.PrintError(err, "No timesheet entries to export")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)

	startOfWeekUTC := utils.WeekStartFromOffset(thisStaff.Config.TimesheetDateOffset)

	for i := 0; i <= 6; i++ {
		report := map[time.Time]ShiftHours{}
		currentDayUTC := startOfWeekUTC.AddDate(0, 0, i)
		thisDate := time.Date(
			currentDayUTC.Year(),
			currentDayUTC.Month(),
			currentDayUTC.Day(),
			0, 0, 0, 0,
			time.Local,
		)

		for h := 8; h <= 27; h++ {
			windowStart := thisDate.Add(time.Duration(h) * time.Hour)
			windowEnd := thisDate.Add(time.Duration(h+1) * time.Hour)
			for _, entry := range *entries {
				if !entry.Approved {
					continue
				}
				window := report[windowStart]
				if entry.ShiftType == models.Bar {
					window.Staff += GetWorkFromEntry(windowStart, windowEnd, *entry)
				} else if entry.ShiftType == models.DayManager || entry.ShiftType == models.NightManager {
					window.Manager += GetWorkFromEntry(windowStart, windowEnd, *entry)
				} else if entry.ShiftType == models.AmeliaSupervisor {
					window.Amelia += GetWorkFromEntry(windowStart, windowEnd, *entry)
				} else if entry.ShiftType == models.GeneralManagement {
					window.Salary += GetWorkFromEntry(windowStart, windowEnd, *entry)
				} else if entry.ShiftType == models.Deliveries {
					window.Deliveries += GetWorkFromEntry(windowStart, windowEnd, *entry)
				}
				report[windowStart] = window
			}
		}

		fileWriter, err := zipWriter.Create(thisDate.Format("2006-01-02") + ".csv")
		if err != nil {
			http.Error(w, "Failed to create file in ZIP", http.StatusInternalServerError)
			return
		}
		csvContent, err := createCSVContent(report)
		if err != nil {
			http.Error(w, "Failed to create CSV content", http.StatusInternalServerError)
			return
		}
		if _, err := fileWriter.Write(csvContent); err != nil {
			http.Error(w, "Failed to write CSV content to ZIP", http.StatusInternalServerError)
			return
		}
	}
	if err := zipWriter.Close(); err != nil {
		http.Error(w, "Failed to close ZIP writer", http.StatusInternalServerError)
		return
	}

	// Set the appropriate headers
	formattedDate := utils.WeekStartFromOffset(thisStaff.Config.TimesheetDateOffset).Format("2006-01-02")
	reportFilename := "report-" + formattedDate + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment;filename="+reportFilename)
	w.Header().Set("Content-Length", strconv.Itoa(len(zipBuffer.Bytes())))

	// Write the zip file to the response
	if _, err := w.Write(zipBuffer.Bytes()); err != nil {
		http.Error(w, "Failed to write ZIP file to response", http.StatusInternalServerError)
		return
	}
}

func createCSVContent(data map[time.Time]ShiftHours) ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)

	// Convert map keys to a slice
	var times []time.Time
	for t := range data {
		times = append(times, t)
	}

	// Sort the slice by date
	sort.Slice(times, func(i, j int) bool {
		return times[i].Before(times[j])
	})

	// Write CSV header
	header := []string{
		"Time",
		"Manager",
		"Staff",
		"Amelia Supervisor",
		"Salary",
		"Deliveries",
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}

	// Write sorted data to CSV
	for _, t := range times {
		record := []string{
			t.Format("1504"),
			fmt.Sprintf("%.2f", data[t].Manager),
			fmt.Sprintf("%.2f", data[t].Staff),
			fmt.Sprintf("%.2f", data[t].Amelia),
			fmt.Sprintf("%.2f", data[t].Salary),
			fmt.Sprintf("%.2f", data[t].Deliveries),
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func (s *Server) HandleImportRosterWeek(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	lastWeek, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset - 1)
	if err != nil {
		utils.PrintError(err, "Couldn't load last week")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	thisWeek, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Couldn't load this week")
		thisWeek = &models.RosterWeek{
			ID:         uuid.New(),
			WeekOffset: thisStaff.Config.RosterDateOffset,
		}
	}
	newWeek := duplicateRosterWeek(*lastWeek, *thisWeek)
	thisWeek = &newWeek
	s.Repos.RosterWeek.SaveRosterWeek(thisWeek)
	s.renderTemplate(w, "rosterMainContainer", s.MakeRootStruct(*thisStaff, *thisWeek))
}

func (s *Server) GetPayWeekForStaff(staffID uuid.UUID, weekOffset int) StaffPayData {
	payData := StaffPayData{}
	entries, err := s.Repos.Timesheet.GetStaffTimesheetWeek(staffID, weekOffset)
	startDate := utils.WeekStartFromOffset(weekOffset)
	if err != nil {
		utils.PrintError(err, "Error getting timesheet entries")
		emptyEntries := []*repository.TimesheetEntry{}
		entries = &emptyEntries
	}
	for day := Tuesday; day <= 6; day++ {
		currentDayUTC := startDate.AddDate(0, 0, int(day))
		thisDate := time.Date(
			currentDayUTC.Year(),
			currentDayUTC.Month(),
			currentDayUTC.Day(),
			0, 0, 0, 0,
			time.Local,
		)
		for _, entry := range *entries {
			if entry.Approved {
				payData = AddEntryToPaydata(*entry, thisDate, day, payData)
			}
		}
	}
	return payData
}

type StaffPaySummary struct {
	TotalHrs    float64
	PayEstimate float64
}

func AddLevelSummary(paySummary StaffPaySummary, breakdown [7]DayBreakdown, levelPay float64) StaffPaySummary {
	for i, day := range breakdown {
		paySummary.TotalHrs += day.OrdinaryHrs
		paySummary.TotalHrs += day.EveningHrs
		paySummary.TotalHrs += day.After12Hrs

		dayIdx := DayIdx(i)
		if dayIdx == Saturday {
			paySummary.PayEstimate += (day.OrdinaryHrs + day.EveningHrs + day.After12Hrs) * levelPay * SAT_PAY_MULT
		} else if dayIdx == Sunday {
			paySummary.PayEstimate += (day.OrdinaryHrs + day.EveningHrs + day.After12Hrs) * levelPay * SUN_PAY_MULT
		} else {
			paySummary.PayEstimate += day.OrdinaryHrs * levelPay * WEEK_PAY_MULT
			paySummary.PayEstimate += day.EveningHrs * (levelPay*WEEK_PAY_MULT + EVENING_PENALTY)
			paySummary.PayEstimate += day.After12Hrs * (levelPay*WEEK_PAY_MULT + AFTER_12_PENALTY)
		}
	}
	return paySummary
}

func (s *Server) GetPaySummary(payData StaffPayData) StaffPaySummary {
	paySummary := StaffPaySummary{}
	paySummary = AddLevelSummary(paySummary, payData.Level2Hrs, LEVEL_2_PAY)
	paySummary = AddLevelSummary(paySummary, payData.Level3Hrs, LEVEL_3_PAY)
	paySummary = AddLevelSummary(paySummary, payData.Level4Hrs, LEVEL_4_PAY)
	paySummary = AddLevelSummary(paySummary, payData.Level5Hrs, LEVEL_5_PAY)
	return paySummary
}
