package server

import (
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

	"roster/cmd/db"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

type ProfileIndexBody struct {
	ID string `json:"editStaffId"`
}

type ProfileIndexData struct {
	CacheBust   string
	RosterLive  bool
	AdminRights bool
	db.StaffMember
}

type ProfileData struct {
	db.StaffMember
	AdminRights       bool
	RosterLive        bool
	ShowUpdateSuccess bool
	ShowUpdateError   bool
	ShowLeaveSuccess  bool
	ShowLeaveError    bool
}

func MakeProfileStruct(rosterLive bool, staffMember db.StaffMember, adminRights bool) ProfileData {
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
	Time     time.Time
	Disabled bool
}

func MakePickerStruct(name string, label string, id uuid.UUID, time time.Time, disabled bool) PickerData {
	return PickerData{
		Name:     name,
		Label:    label,
		ID:       id,
		Time:     time,
		Disabled: disabled,
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
		CacheBust:   s.CacheBust,
		StaffMember: *editStaff,
		AdminRights: adminRights,
		RosterLive:  s.LoadRosterWeek(editStaff.Config.RosterStartDate).IsLive,
	}

	err := s.Templates.ExecuteTemplate(w, "profileIndex", data)
	if err != nil {
		log.Fatalf("Error executing template: %v", err)
		return
	}
}

func (s *Server) HandleProfile(w http.ResponseWriter, r *http.Request) {
	staff := s.GetSessionUser(w, r)
	if staff == nil {
		return
	}
	data := ProfileData{
		AdminRights: staff.IsAdmin,
		StaffMember: *staff,
		RosterLive:  s.LoadRosterWeek(staff.Config.RosterStartDate).IsLive,
	}
	s.renderTemplate(w, "profile", data)
}

func (s *Server) HandleSubmitLeave(w http.ResponseWriter, r *http.Request) {
	log.Println("Submit leave request")
	var reqBody db.LeaveRequest
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	reqBody.ID = uuid.New()
	now := time.Now()
	reqBody.CreationDate = db.CustomDate{Time: &now}
	staff := s.GetSessionUser(w, r)
	if staff == nil {
		return
	}
	data := ProfileData{
		AdminRights: staff.IsAdmin,
		RosterLive:  s.LoadRosterWeek(staff.Config.RosterStartDate).IsLive,
	}
	if reqBody.StartDate.After(*reqBody.EndDate.Time) {
		data.ShowLeaveError = true
	} else {
		data.ShowLeaveSuccess = true
		staff.LeaveRequests = append(staff.LeaveRequests, reqBody)
		s.SaveStaffMember(*staff)
	}
	data.StaffMember = *staff
	// TODO: Maybe won't need to reload the datepicker if I just swap the leave list
	// s.renderTemplate(w, "profile", data)
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

func (s *Server) ApplyModifyProfileBody(reqBody ModifyProfileBody, staffMember db.StaffMember) db.StaffMember {
	staffMember.NickName = reqBody.NickName
	staffMember.FirstName = reqBody.FirstName
	staffMember.LastName = reqBody.LastName
	staffMember.Email = reqBody.Email
	staffMember.Phone = reqBody.Phone
	staffMember.ContactName = reqBody.ContactName
	staffMember.ContactPhone = reqBody.ContactPhone
	// This can fail but not from me
	staffMember.IdealShifts, _ = strconv.Atoi(reqBody.IdealShifts)

	staffMember.Availability = []db.DayAvailability{
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
	log.Println("Modify profile")
	var reqBody ModifyProfileBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	staffID, err := uuid.Parse(reqBody.ID)
	if err != nil {
		log.Printf("Invalid staffID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	staff := s.GetStaffByID(staffID)
	if staff == nil {
		return
	}
	activeStaff := s.GetSessionUser(w, r)
	if activeStaff == nil {
		return
	}
	if activeStaff.ID != staff.ID && !activeStaff.IsAdmin {
		log.Printf("Insufficient privilledges: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	updatedStaff := s.ApplyModifyProfileBody(reqBody, *staff)
	data := ProfileData{
		AdminRights:       activeStaff.IsAdmin,
		StaffMember:       updatedStaff,
		RosterLive:        s.LoadRosterWeek(staff.Config.RosterStartDate).IsLive,
		ShowUpdateSuccess: true,
	}
	s.SaveStaffMember(updatedStaff)
	s.renderTemplate(w, "profile", data)
}

type DeleteLeaveBody struct {
	ID      string `json:"id"`
	StaffID string `json:"staffID"`
	Page    string `json:"page"`
}

func (s *Server) HandleDeleteLeaveReq(w http.ResponseWriter, r *http.Request) {
	log.Println("Delete leave request")
	var reqBody DeleteLeaveBody
	if err := ReadAndUnmarshal(w, r, &reqBody); err != nil {
		return
	}
	leaveID, err := uuid.Parse(reqBody.ID)
	if err != nil {
		log.Printf("Invalid leaveID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
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

	staffMember := s.GetStaffByID(staffID)
	if staffMember == nil {
		log.Printf("No staff found with id %v", staffID)
		week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
		s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
		return
	}

	s.DeleteLeaveReqByID(*staffMember, leaveID)
	if reqBody.Page == "root" {
		week := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)
		s.renderTemplate(w, "root", s.MakeRootStruct(*thisStaff, *week))
	} else {
		data := ProfileData{
			StaffMember: *thisStaff,
			AdminRights: thisStaff.IsAdmin,
			RosterLive:  s.LoadRosterWeek(thisStaff.Config.RosterStartDate).IsLive,
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
		log.Printf("Invalid accID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	collection := s.DB.Collection("staff")
	filter := bson.M{"id": accID}
	_, err = collection.DeleteOne(s.Context, filter)
	if err != nil {
		log.Fatalf("Failed to delete document: %v", err)
	}
	selfDelete := thisStaff.ID == accID

	rosterWeek := s.LoadRosterWeek(thisStaff.Config.RosterStartDate)

	for _, day := range rosterWeek.Days {
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

	s.SaveRosterWeek(*rosterWeek)
	if selfDelete {
		s.HandleGoogleLogout(w, r)
	} else {
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusOK)
	}
}

type LeaveReqData struct {
	db.LeaveRequest
	StaffID   uuid.UUID
	StaffName string
}

func GetSortedLeaveReqs(allStaff []*db.StaffMember) []LeaveReqData {
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
