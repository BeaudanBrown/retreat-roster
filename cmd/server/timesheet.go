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

type TimesheetData struct {
	Entries     []db.TimesheetEntry
	StaffMember db.StaffMember
	DayNames    []string
	StaffMap    map[uuid.UUID]db.StaffMember
	RosterLive  bool
	CacheBust   string
}

func (s *Server) MakeTimesheetStruct(activeStaff db.StaffMember) TimesheetData {
	entries := s.GetTimesheetWeek(activeStaff.Config.TimesheetStartDate)
	if entries == nil {
		log.Printf("Failed to load timesheet week when making struct")
	}

	return TimesheetData{
		Entries:     *entries,
		StaffMember: activeStaff,
		DayNames:    []string{"Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday", "Monday"},
		StaffMap:    s.GetStaffMap(),
		CacheBust:   s.CacheBust,
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

func (s *Server) RenderTimesheetTemplate(w http.ResponseWriter, r *http.Request, adminView bool) {
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
	s.RenderTimesheetTemplate(w, r, reqBody.AdminView)
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
	s.RenderTimesheetTemplate(w, r, reqBody.AdminView)
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
	err = s.CreateTimesheetEntry(*reqBody.StartDate.Time, staffID)
	if err != nil {
		log.Printf("Couldn't create new timesheet entry: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	s.RenderTimesheetTemplate(w, r, reqBody.AdminView)
}

type ModifyTimesheetEntryBody struct {
	StaffID    string        `json:"staffID"`
	EntryID    string        `json:"entryID"`
	ShiftStart db.CustomDate `json:"shiftStart"`
	ShiftEnd   db.CustomDate `json:"shiftEnd"`
	BreakStart db.CustomDate `json:"breakStart"`
	BreakEnd   db.CustomDate `json:"breakEnd"`
	Approved   bool          `json:"approved"`
	ShiftType  string        `json:"shiftType"`
	AdminView  bool          `json:"adminView"`
}

func getAdjustedTime(t db.CustomDate, dayDate time.Time) time.Time {
	year, month, day := dayDate.Date()
	if t.Time != nil {
		hour, min, _ := t.Clock()
		newBreakStart := time.Date(year, month, day, hour, min, 0, 0, time.Now().Location())
		return newBreakStart
	} else {
		return time.Now()
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
		log.Println("Couldn't find entry to modify")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	entry.StaffID = staffID
	entry.Approved = reqBody.Approved
	entry.ShiftType = db.StringToShiftType(reqBody.ShiftType)
	if reqBody.BreakStart.Time != nil {
		bs := getAdjustedTime(reqBody.BreakStart, entry.StartDate)
		entry.BreakStart = &bs
	}
	if reqBody.BreakEnd.Time != nil {
		be := getAdjustedTime(reqBody.BreakEnd, entry.StartDate)
		entry.BreakEnd = &be
	}

	if entry.BreakStart != nil && entry.BreakEnd != nil {
		if entry.BreakStart.After(*entry.BreakEnd) {
			newBreakEnd := entry.BreakEnd.AddDate(0, 0, 1)
			entry.BreakEnd = &newBreakEnd
		}
		entry.BreakLength = math.Round(entry.BreakEnd.Sub(*entry.BreakStart).Hours()*100) / 100
	} else {
		entry.BreakLength = 0
	}

	entry.ShiftStart = getAdjustedTime(reqBody.ShiftStart, entry.StartDate)
	entry.ShiftEnd = getAdjustedTime(reqBody.ShiftEnd, entry.StartDate)

	if entry.ShiftStart.After(entry.ShiftEnd) {
		entry.ShiftEnd = entry.ShiftEnd.AddDate(0, 0, 1)
	}
	entry.ShiftLength = math.Round((entry.ShiftEnd.Sub(entry.ShiftStart).Hours()-entry.BreakLength)*100) / 100
	s.SaveTimesheetEntry(*entry)

	s.RenderTimesheetTemplate(w, r, reqBody.AdminView)
}

func (s *Server) HandleToggleShowAll(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	thisStaff.Config.ShowAll = !thisStaff.Config.ShowAll
	s.SaveStaffMember(*thisStaff)
	s.RenderTimesheetTemplate(w, r, thisStaff.IsAdmin)
}

func (s *Server) HandleToggleHideApproved(w http.ResponseWriter, r *http.Request) {
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}
	thisStaff.Config.HideApproved = !thisStaff.Config.HideApproved
	s.SaveStaffMember(*thisStaff)
	s.RenderTimesheetTemplate(w, r, thisStaff.IsAdmin)
}

type TimesheetEntryData struct {
	db.TimesheetEntry
	ThisStaffID uuid.UUID
	StaffMap    map[uuid.UUID]db.StaffMember
	ShowAll     bool
	IsAdmin     bool
}

func MakeTimesheetEntryStruct(entry db.TimesheetEntry, thisStaffID uuid.UUID, staffMap map[uuid.UUID]db.StaffMember, showAll bool, isAdmin bool) TimesheetEntryData {
	return TimesheetEntryData{
		TimesheetEntry: entry,
		ThisStaffID:    thisStaffID,
		StaffMap:       staffMap,
		ShowAll:        showAll,
		IsAdmin:        isAdmin,
	}
}
