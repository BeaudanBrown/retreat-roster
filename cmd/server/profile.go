package server

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"roster/cmd/models"
	"roster/cmd/utils"

	"github.com/google/uuid"
)

type ProfileIndexBody struct {
	ID string `json:"editStaffId"`
}

type ProfileIndexData struct {
	CacheBust   string
	RosterLive  bool
	AdminRights bool
	models.StaffMember
}

type ProfileData struct {
	models.StaffMember
	AdminRights       bool
	RosterLive        bool
	ShowUpdateSuccess bool
	ShowUpdateError   bool
	ShowLeaveSuccess  bool
	ShowLeaveError    bool
}

func MakeProfileStruct(rosterLive bool, staffMember models.StaffMember, adminRights bool) ProfileData {
	return ProfileData{
		StaffMember: staffMember,
		AdminRights: adminRights,
		RosterLive:  rosterLive,
	}
}

type PickerData struct {
	Name     string
	Label    string
	ID       uuid.UUID
	Date     time.Time
	Time     time.Time
	Disabled bool
}

func MakePickerStruct(name string, label string, id uuid.UUID, weekOffset int, dayOffset int, time time.Time, disabled bool) PickerData {
	startDate := utils.WeekStartFromOffset(weekOffset).AddDate(0, 0, dayOffset)
	return PickerData{
		Name:     name,
		Label:    label,
		ID:       id,
		Date:     startDate,
		Time:     time,
		Disabled: disabled,
	}
}

type LeaveReqListData struct {
	StaffMember      models.StaffMember
	ShowLeaveSuccess bool
	ShowLeaveError   bool
}

func MakeLeaveReqStruct(staffMember models.StaffMember, showLeaveSuccess bool, showLeaveError bool) LeaveReqListData {
	return LeaveReqListData{
		StaffMember:      staffMember,
		ShowLeaveSuccess: showLeaveSuccess,
		ShowLeaveError:   showLeaveError,
	}
}

func (s *Server) HandleProfileIndex(w http.ResponseWriter, r *http.Request) {
	editStaff := s.GetSessionUser(w, r)
	if editStaff == nil {
		http.Redirect(w, r, "/landing", http.StatusSeeOther)
		return
	}
	adminRights := editStaff.IsAdmin

	if r.Method == http.MethodGet {
		editStaffIdParam := r.URL.Query().Get("editStaffId")
		if editStaffIdParam != "" {
			editStaffId, err := uuid.Parse(editStaffIdParam)
			if err != nil {
				utils.PrintError(err, "Invalid UUID")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			staff, err := s.Repos.Staff.GetStaffByID(editStaffId)
			if err != nil {
				utils.PrintError(err, "Failed to get staff by ID")
				w.WriteHeader(http.StatusNotFound)
				return
			} else {
				editStaff = staff
			}
		}
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rosterWeek, err := s.Repos.RosterWeek.LoadRosterWeek(editStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	data := ProfileIndexData{
		CacheBust:   s.CacheBust,
		StaffMember: *editStaff,
		AdminRights: adminRights,
		RosterLive:  rosterWeek.IsLive,
	}

	err = s.Templates.ExecuteTemplate(w, "profileIndex", data)
	if err != nil {
		utils.PrintError(err, "Error executing template")
		return
	}
}

func (s *Server) HandleProfile(w http.ResponseWriter, r *http.Request) {
	staff := s.GetSessionUser(w, r)
	if staff == nil {
		return
	}
	rosterWeek, err := s.Repos.RosterWeek.LoadRosterWeek(staff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		return
	}

	data := ProfileData{
		AdminRights: staff.IsAdmin,
		StaffMember: *staff,
		RosterLive:  rosterWeek.IsLive,
	}
	s.renderTemplate(w, "profile", data)
}

func (s *Server) HandleSubmitLeave(w http.ResponseWriter, r *http.Request) {
	var reqBody models.LeaveRequest
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	reqBody.ID = uuid.New()
	now := time.Now()
	reqBody.CreationDate = models.CustomDate{Time: &now}
	staff := s.GetSessionUser(w, r)
	if staff == nil {
		return
	}
	showLeaveError := false
	showLeaveSuccess := false
	if !reqBody.EndDate.After(*reqBody.StartDate.Time) {
		showLeaveError = true
	} else {
		showLeaveSuccess = true
		staff.LeaveRequests = append(staff.LeaveRequests, reqBody)
		s.Repos.Staff.SaveStaffMember(*staff)
	}
	data := MakeLeaveReqStruct(*staff, showLeaveSuccess, showLeaveError)
	data.StaffMember = *staff
	s.renderTemplate(w, "leaveReqForm", data)
}

type ModifyProfileBody struct {
	ID           string `json:"id"`
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	NickName     string `json:"nickName"`
	IdealShifts  string `json:"ideal-shifts"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	ContactName  string `json:"contactName"`
	ContactPhone string `json:"contactPhone"`
	TuesEarly    string `json:"Tuesday-early-avail"`
	TuesMid      string `json:"Tuesday-mid-avail"`
	TuesLate     string `json:"Tuesday-late-avail"`
	WedEarly     string `json:"Wednesday-early-avail"`
	WedMid       string `json:"Wednesday-mid-avail"`
	WedLate      string `json:"Wednesday-late-avail"`
	ThursEarly   string `json:"Thursday-early-avail"`
	ThursMid     string `json:"Thursday-mid-avail"`
	ThursLate    string `json:"Thursday-late-avail"`
	FriEarly     string `json:"Friday-early-avail"`
	FriMid       string `json:"Friday-mid-avail"`
	FriLate      string `json:"Friday-late-avail"`
	SatEarly     string `json:"Saturday-early-avail"`
	SatMid       string `json:"Saturday-mid-avail"`
	SatLate      string `json:"Saturday-late-avail"`
	SunEarly     string `json:"Sunday-early-avail"`
	SunMid       string `json:"Sunday-mid-avail"`
	SunLate      string `json:"Sunday-late-avail"`
	MonEarly     string `json:"Monday-early-avail"`
	MonMid       string `json:"Monday-mid-avail"`
	MonLate      string `json:"Monday-late-avail"`
}

func (s *Server) ApplyModifyProfileBody(reqBody ModifyProfileBody, staffMember models.StaffMember) models.StaffMember {
	staffMember.NickName = reqBody.NickName
	staffMember.FirstName = reqBody.FirstName
	staffMember.LastName = reqBody.LastName
	staffMember.Email = reqBody.Email
	staffMember.Phone = reqBody.Phone
	staffMember.ContactName = reqBody.ContactName
	staffMember.ContactPhone = reqBody.ContactPhone
	// This can fail but not from me
	staffMember.IdealShifts, _ = strconv.Atoi(reqBody.IdealShifts)

	staffMember.Availability = []models.DayAvailability{
		{
			Name:  "Tuesday",
			Early: reqBody.TuesEarly == "on",
			Mid:   reqBody.TuesMid == "on",
			Late:  reqBody.TuesLate == "on",
		},
		{
			Name:  "Wednesday",
			Early: reqBody.WedEarly == "on",
			Mid:   reqBody.WedMid == "on",
			Late:  reqBody.WedLate == "on",
		},
		{
			Name:  "Thursday",
			Early: reqBody.ThursEarly == "on",
			Mid:   reqBody.ThursMid == "on",
			Late:  reqBody.ThursLate == "on",
		},
		{
			Name:  "Friday",
			Early: reqBody.FriEarly == "on",
			Mid:   reqBody.FriMid == "on",
			Late:  reqBody.FriLate == "on",
		},
		{
			Name:  "Saturday",
			Early: reqBody.SatEarly == "on",
			Mid:   reqBody.SatMid == "on",
			Late:  reqBody.SatLate == "on",
		},
		{
			Name:  "Sunday",
			Early: reqBody.SunEarly == "on",
			Mid:   reqBody.SunMid == "on",
			Late:  reqBody.SunLate == "on",
		},
		{
			Name:  "Monday",
			Early: reqBody.MonEarly == "on",
			Mid:   reqBody.MonMid == "on",
			Late:  reqBody.MonLate == "on",
		},
	}
	return staffMember
}

func (s *Server) HandleModifyProfile(w http.ResponseWriter, r *http.Request) {
	var reqBody ModifyProfileBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	staffID, err := uuid.Parse(reqBody.ID)
	if err != nil {
		utils.PrintError(err, "Invalid staffID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	staff, err := s.Repos.Staff.GetStaffByID(staffID)
	if err != nil {
		return
	}
	activeStaff := s.GetSessionUser(w, r)
	if activeStaff == nil {
		return
	}
	if activeStaff.ID != staff.ID && !activeStaff.IsAdmin {
		utils.PrintError(err, "Insufficient privilledges")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	updatedStaff := s.ApplyModifyProfileBody(reqBody, *staff)
	rosterWeek, err := s.Repos.RosterWeek.LoadRosterWeek(staff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		return
	}
	data := ProfileData{
		AdminRights:       activeStaff.IsAdmin,
		StaffMember:       updatedStaff,
		RosterLive:        rosterWeek.IsLive,
		ShowUpdateSuccess: true,
	}
	s.Repos.Staff.SaveStaffMember(updatedStaff)
	s.renderTemplate(w, "profile", data)
}

type DeleteLeaveBody struct {
	ID      string `json:"id"`
	StaffID string `json:"staffID"`
	Page    string `json:"page"`
}

func (s *Server) HandleDeleteLeaveReq(w http.ResponseWriter, r *http.Request) {
	var reqBody DeleteLeaveBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	leaveID, err := uuid.Parse(reqBody.ID)
	if err != nil {
		utils.PrintError(err, "Invalid leaveID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	staffID, err := uuid.Parse(reqBody.StaffID)
	if err != nil {
		utils.PrintError(err, "Invalid staffID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	thisStaff := s.GetSessionUser(w, r)
	if thisStaff == nil {
		return
	}

	rosterWeek, err := s.Repos.RosterWeek.LoadRosterWeek(thisStaff.Config.RosterDateOffset)
	if err != nil {
		utils.PrintError(err, "Failed to load roster week")
		return
	}

	staffMember, err := s.Repos.Staff.GetStaffByID(staffID)
	if err != nil {
		utils.PrintError(err, "Failed to load staff member")
		s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *rosterWeek))
		return
	}

	err = s.Repos.Staff.DeleteLeaveReqByID(*staffMember, leaveID)
	if err != nil {
		utils.PrintError(err, "Failed delete leave request")
		return
	}
	if reqBody.Page == "root" {
		s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *rosterWeek))
	} else {
		data := ProfileData{
			StaffMember: *thisStaff,
			AdminRights: thisStaff.IsAdmin,
			RosterLive:  rosterWeek.IsLive,
		}
		s.renderTemplate(w, "profile", data)
	}
}

type DeleteAccountBody struct {
	ID string `json:"id"`
}

func (s *Server) HandleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	var reqBody DeleteAccountBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	accID, err := uuid.Parse(reqBody.ID)
	thisStaff := s.GetSessionUser(w, r)
	if err != nil || thisStaff == nil {
		utils.PrintError(err, "Invalid accID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := s.Repos.Staff.DeleteStaffByID(accID); err != nil {
		utils.PrintError(err, "Failed to delete staff")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	selfDelete := thisStaff.ID == accID

	if selfDelete {
		s.HandleGoogleLogout(w, r)
	} else {
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusOK)
	}
}

type LeaveReqData struct {
	models.LeaveRequest
	StaffID   uuid.UUID
	StaffName string
}

func GetSortedLeaveReqs(allStaff []*models.StaffMember) []LeaveReqData {
	reqs := []LeaveReqData{}
	for _, staffMember := range allStaff {
		for _, req := range staffMember.LeaveRequests {
			name := staffMember.FirstName
			if staffMember.NickName != "" {
				name = staffMember.NickName
			}
			reqs = append(reqs, LeaveReqData{
				req,
				staffMember.ID,
				name,
			})
		}
	}
	sort.Slice(reqs, func(i, j int) bool {
		return reqs[i].StartDate.Before(*reqs[j].StartDate.Time)
	})
	return reqs
}
