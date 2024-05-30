package server

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
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
	newDays := []*db.RosterDay{}
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
		newDays = append(newDays, &newDay)
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

func (s *Server) GetWorkFromEntry(windowStart time.Time, windowEnd time.Time, entry db.TimesheetEntry) float64 {
	if entry.ShiftStart == nil || entry.ShiftEnd == nil {
		return 0.0
	}
	shiftStart := maxTime(windowStart, *entry.ShiftStart)
	shiftEnd := minTime(windowEnd, *entry.ShiftEnd)
	if shiftStart.After(shiftEnd) {
		return 0.0
	}
	overlappedShiftDuration := shiftEnd.Sub(shiftStart).Hours()
	if entry.BreakStart != nil && entry.ShiftEnd != nil {
		breakWindowStart := maxTime(shiftStart, *entry.BreakStart)
		breakWindowEnd := minTime(shiftEnd, *entry.BreakEnd)
		if breakWindowStart.After(breakWindowEnd) {
			breakWindowStart = breakWindowEnd
		}
		overlappedBreakDuration := breakWindowEnd.Sub(breakWindowStart).Hours()
		return overlappedShiftDuration - overlappedBreakDuration
	}
	return overlappedShiftDuration
}

type ShiftHours struct {
	Staff   float64
	Manager float64
	Salary  float64
}

func (s *Server) HandleExportWageReport(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		log.Println("Couldn't find session user")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	entries := s.GetTimesheetWeek(thisStaff.Config.TimesheetStartDate)
	if entries == nil {
		log.Println("No timesheet entries to export")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)

	for i := 0; i <= 6; i++ {
		report := map[time.Time]ShiftHours{}
		thisDate := thisStaff.Config.TimesheetStartDate.AddDate(0, 0, i)
		for i := 8; i <= 27; i++ {
			windowStart := thisDate.Add(time.Duration(i) * time.Hour)
			windowEnd := thisDate.Add(time.Duration(i+1) * time.Hour)
			for _, entry := range *entries {
				if entry.Status != db.Approved {
					continue
				}
				window := report[windowStart]
				if entry.ShiftType == db.Bar {
					window.Staff += s.GetWorkFromEntry(windowStart, windowEnd, entry)
				} else if entry.ShiftType == db.DayManager || entry.ShiftType == db.NightManager {
					window.Manager += s.GetWorkFromEntry(windowStart, windowEnd, entry)
				} else {
					window.Salary += s.GetWorkFromEntry(windowStart, windowEnd, entry)
				}
				report[windowStart] = window
			}
		}

		fileWriter, err := zipWriter.Create(thisDate.Format("02-01-06") + ".csv")
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
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment;filename=data.zip")
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
	header := []string{"Time", "Staff Hours", "Manager Hours", "Salary Hours"}
	if err := writer.Write(header); err != nil {
		return nil, err
	}

	// Write sorted data to CSV
	for _, t := range times {
		record := []string{
			t.Format("1504"),
			fmt.Sprintf("%.2f", data[t].Staff),
			fmt.Sprintf("%.2f", data[t].Manager),
			fmt.Sprintf("%.2f", data[t].Salary),
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
