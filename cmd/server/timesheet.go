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

const LEVEL_2_PAY = 24.98
const LEVEL_3_PAY = 25.8
const LEVEL_4_PAY = 27.17
const LEVEL_5_PAY = 28.87

const EVENING_PENALTY = 2.72
const AFTER_12_PENALTY = 4.08

const WEEK_PAY_MULT = 1.25
const SAT_PAY_MULT = 1.5
const SUN_PAY_MULT = 1.75

type TimesheetData struct {
	Entries         []*db.TimesheetEntry
	StaffMember     db.StaffMember
	DayNames        []string
	AllStaff        []*db.StaffMember
	StaffPaySummary StaffPaySummary
	RosterLive      bool
	CacheBust       string
}

func (s *Server) MakeTimesheetStruct(activeStaff db.StaffMember) TimesheetData {
	entries := s.GetTimesheetWeek(activeStaff.Config.TimesheetStartDate)
	if entries == nil {
		log.Printf("Failed to load timesheet week when making struct")
	}

	//TODO: this can be optimised
	staffPayData := s.GetPayWeekForStaff(activeStaff.ID, activeStaff.Config.TimesheetStartDate)
	paySummary := s.GetPaySummary(staffPayData)

	return TimesheetData{
		Entries:         *entries,
		StaffMember:     activeStaff,
		DayNames:        []string{"Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday", "Monday"},
		AllStaff:        s.LoadAllStaff(),
		StaffPaySummary: paySummary,
		CacheBust:       s.CacheBust,
	}
}

func (s *Server) HandleTimesheet(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	data := s.MakeTimesheetStruct(*thisStaff)
	s.renderTemplate(w, "timesheet", data)
}

func (s *Server) RenderTimesheetTemplate(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}

	data := s.MakeTimesheetStruct(*thisStaff)
	s.renderTemplate(w, "timesheet", data)
}

type ShiftTimesheetWindowBody struct {
	Action    string `json:"action"`
	AdminView bool   `json:"adminView"`
}

func (s *Server) HandleShiftTimesheetWindow(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	var reqBody ShiftTimesheetWindowBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	switch reqBody.Action {
	case "+":
		thisStaff.Config.TimesheetStartDate = thisStaff.Config.TimesheetStartDate.AddDate(0, 0, 7)
	case "-":
		thisStaff.Config.TimesheetStartDate = thisStaff.Config.TimesheetStartDate.AddDate(0, 0, -7)
	default:
		thisStaff.Config.TimesheetStartDate = utils.GetLastTuesday()
	}
	s.SaveStaffMember(*thisStaff)
	s.RenderTimesheetTemplate(w, r)
}

type DeleteTimesheetEntryBody struct {
	StaffID   string `json:"staffID"`
	EntryID   string `json:"entryID"`
	AdminView bool   `json:"adminView"`
}

func (s *Server) HandleDeleteTimesheetEntry(w http.ResponseWriter, r *http.Request) {
	log.Println("Delete timesheet entry")
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	var reqBody DeleteTimesheetEntryBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	entryID, err := uuid.Parse(reqBody.EntryID)
	if err != nil {
		log.Printf("Invalid entryID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err = s.DeleteTimesheetEntry(entryID)
	if err != nil {
		log.Printf("Error deleting timesheet entry: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	s.RenderTimesheetTemplate(w, r)
}

type AddTimesheetEntryBody struct {
	StaffID   string        `json:"staffID"`
	DayIdx    int           `json:"dayIdx"`
	StartDate db.CustomDate `json:"startDate"`
	AdminView bool          `json:"adminView"`
}

func (s *Server) HandleAddTimesheetEntry(w http.ResponseWriter, r *http.Request) {
	var reqBody AddTimesheetEntryBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		log.Printf("Error parsing body: %v", err)
		return
	}
	staffID, err := uuid.Parse(reqBody.StaffID)
	if err != nil {
		log.Printf("Invalid staffID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	newEntry := MakeEmptyTimesheetEntry(*reqBody.StartDate.Time, staffID)
	data := MakeTimesheetEditModalStruct(newEntry, thisStaff.ID, s.LoadAllStaff(), thisStaff.IsAdmin)
	s.renderTemplate(w, "timesheetEditModal", data)
}

type ModifyTimesheetEntryBody struct {
	StaffID    string        `json:"staffID"`
	EntryID    string        `json:"entryID"`
	StartDate  db.CustomDate `json:"startDate"`
	ShiftStart db.CustomDate `json:"shiftStart"`
	ShiftEnd   db.CustomDate `json:"shiftEnd"`
	BreakStart db.CustomDate `json:"breakStart"`
	BreakEnd   db.CustomDate `json:"breakEnd"`
	Approved   bool          `json:"approved"`
	ShiftType  string        `json:"shiftType"`
	AdminView  bool          `json:"adminView"`
	HasBreak   string        `json:"hasBreak"`
}

func getAdjustedTime(t db.CustomDate, dayDate time.Time) time.Time {
	year, month, day := dayDate.Date()
	if t.Time != nil {
		hour, min, _ := t.Clock()
		adjustedTime := time.Date(year, month, day, hour, min, 0, 0, time.Now().Location())
		return adjustedTime
	} else {
		adjustedTime := time.Date(year, month, day, 12, 0, 0, 0, time.Now().Location())
		return adjustedTime
	}
}

func (s *Server) HandleModifyTimesheetEntry(w http.ResponseWriter, r *http.Request) {
	var reqBody ModifyTimesheetEntryBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		log.Printf("Error parsing body: %v", err)
		return
	}
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
	entry := s.GetTimesheetEntryByID(entryID)
	if entry == nil {
		newEntry := db.TimesheetEntry{
			ID:        entryID,
			StartDate: *reqBody.StartDate.Time,
		}
		entry = &newEntry
	}
	entry.StaffID = staffID
	entry.Approved = reqBody.Approved
	entry.ShiftType = db.StringToShiftType(reqBody.ShiftType)
	if reqBody.BreakStart.Time != nil {
		bs := getAdjustedTime(reqBody.BreakStart, entry.StartDate)
		entry.BreakStart = bs
	}
	if reqBody.BreakEnd.Time != nil {
		be := getAdjustedTime(reqBody.BreakEnd, entry.StartDate)
		entry.BreakEnd = be
	}

	if reqBody.HasBreak == "on" {
		entry.HasBreak = true
		log.Println("Had break")
		if entry.BreakStart.After(entry.BreakEnd) {
			newBreakEnd := entry.BreakEnd.AddDate(0, 0, 1)
			entry.BreakEnd = newBreakEnd
		}
		entry.BreakLength = math.Round(entry.BreakEnd.Sub(entry.BreakStart).Hours()*100) / 100
	} else {
		entry.HasBreak = false
		entry.BreakLength = 0
	}
	entry.ShiftStart = getAdjustedTime(reqBody.ShiftStart, entry.StartDate)
	entry.ShiftEnd = getAdjustedTime(reqBody.ShiftEnd, entry.StartDate)

	if entry.ShiftStart.After(entry.ShiftEnd) {
		entry.ShiftEnd = entry.ShiftEnd.AddDate(0, 0, 1)
	}
	entry.ShiftLength = math.Round((entry.ShiftEnd.Sub(entry.ShiftStart).Hours()-entry.BreakLength)*100) / 100
	s.SaveTimesheetEntry(*entry)

	s.RenderTimesheetTemplate(w, r)
}

func (s *Server) HandleToggleShowAll(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	thisStaff.Config.ShowAll = !thisStaff.Config.ShowAll
	s.SaveStaffMember(*thisStaff)
	s.RenderTimesheetTemplate(w, r)
}

func (s *Server) HandleToggleHideApproved(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	thisStaff.Config.HideApproved = !thisStaff.Config.HideApproved
	s.SaveStaffMember(*thisStaff)
	s.RenderTimesheetTemplate(w, r)
}

type ToggleApprovedBody struct {
	EntryID string
}

func (s *Server) HandleToggleApproved(w http.ResponseWriter, r *http.Request) {
	var reqBody ToggleApprovedBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	entryID, err := uuid.Parse(reqBody.EntryID)
	if err != nil {
		log.Printf("Invalid entryID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	entry := s.GetTimesheetEntryByID(entryID)
	if entry == nil {
		log.Println("Couldn't find entry to modify")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	entry.Approved = !entry.Approved
	s.SaveTimesheetEntry(*entry)

	s.RenderTimesheetTemplate(w, r)
}

type TimesheetEntryData struct {
	db.TimesheetEntry
	ActiveStaff db.StaffMember
	EntryStaff  db.StaffMember
	ShowAll     bool
}

func MakeTimesheetEntryStruct(entry db.TimesheetEntry, activeStaff db.StaffMember, allStaff []*db.StaffMember, showAll bool) TimesheetEntryData {
	entryStaff := db.GetStaffFromList(entry.StaffID, allStaff)
	if entryStaff == nil {
		// TODO: This is not ideal
		entryStaff = &db.StaffMember{}
	}
	return TimesheetEntryData{
		TimesheetEntry: entry,
		ActiveStaff:    activeStaff,
		EntryStaff:     *entryStaff,
		ShowAll:        showAll,
	}
}

type TimesheetEditModalData struct {
	db.TimesheetEntry
	ThisStaffID uuid.UUID
	AllStaff    []*db.StaffMember
	IsAdmin     bool
}

func MakeEmptyTimesheetEntry(startDate time.Time, staffID uuid.UUID) db.TimesheetEntry {
	year, month, day := startDate.Date()
	dateOnly := time.Date(year, month, day, 0, 0, 0, 0, time.Now().Location())
	start := db.LastWholeHour()
	end := db.NextWholeHour()
	newEntry := db.TimesheetEntry{
		ID:          uuid.New(),
		StaffID:     staffID,
		StartDate:   dateOnly,
		ShiftStart:  start,
		ShiftEnd:    end,
		ShiftLength: end.Sub(start).Hours(),
	}
	return newEntry
}

func MakeTimesheetEditModalStruct(entry db.TimesheetEntry, thisStaffID uuid.UUID, allStaff []*db.StaffMember, isAdmin bool) TimesheetEditModalData {
	return TimesheetEditModalData{
		TimesheetEntry: entry,
		ThisStaffID:    thisStaffID,
		AllStaff:       allStaff,
		IsAdmin:        isAdmin,
	}
}

type GetTimesheetEditModalBody struct {
	EntryID string
}

func (s *Server) HandleGetTimesheetEditModal(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	var reqBody GetTimesheetEditModalBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	entryID, err := uuid.Parse(reqBody.EntryID)
	if err != nil {
		log.Printf("Invalid entryID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	entry := s.GetTimesheetEntryByID(entryID)
	if entry == nil {
		log.Println("Couldn't find entry to modify")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data := MakeTimesheetEditModalStruct(*entry, thisStaff.ID, s.LoadAllStaff(), thisStaff.IsAdmin)
	s.renderTemplate(w, "timesheetEditModal", data)
}
