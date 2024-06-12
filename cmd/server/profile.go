package server

import (
	"log"
	"net/http"
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

func (s *Server) HandleProfileIndex(w http.ResponseWriter, r *http.Request) {
	editStaff := s.GetSessionUser(w, r)
	if editStaff == nil {
		w.WriteHeader(http.StatusUnauthorized)
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
	s.renderTemplate(w, "profile", data)
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
	TuesEarly    string `json:"Tues-early-avail"`
	TuesMid      string `json:"Tues-mid-avail"`
	TuesLate     string `json:"Tues-late-avail"`
	WedEarly     string `json:"Wed-early-avail"`
	WedMid       string `json:"Wed-mid-avail"`
	WedLate      string `json:"Wed-late-avail"`
	ThursEarly   string `json:"Thurs-early-avail"`
	ThursMid     string `json:"Thurs-mid-avail"`
	ThursLate    string `json:"Thurs-late-avail"`
	FriEarly     string `json:"Fri-early-avail"`
	FriMid       string `json:"Fri-mid-avail"`
	FriLate      string `json:"Fri-late-avail"`
	SatEarly     string `json:"Sat-early-avail"`
	SatMid       string `json:"Sat-mid-avail"`
	SatLate      string `json:"Sat-late-avail"`
	SunEarly     string `json:"Sun-early-avail"`
	SunMid       string `json:"Sun-mid-avail"`
	SunLate      string `json:"Sun-late-avail"`
	MonEarly     string `json:"Mon-early-avail"`
	MonMid       string `json:"Mon-mid-avail"`
	MonLate      string `json:"Mon-late-avail"`
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
	staff.NickName = reqBody.NickName
	staff.FirstName = reqBody.FirstName
	staff.LastName = reqBody.LastName
	staff.Email = reqBody.Email
	staff.Phone = reqBody.Phone
	staff.ContactName = reqBody.ContactName
	staff.ContactPhone = reqBody.ContactPhone
	// This can fail but not from me
	staff.IdealShifts, _ = strconv.Atoi(reqBody.IdealShifts)

	staff.Availability = []db.DayAvailability{
		{
			Name:  "Tues",
			Early: reqBody.TuesEarly == "on",
			Mid:   reqBody.TuesMid == "on",
			Late:  reqBody.TuesLate == "on",
		},
		{
			Name:  "Wed",
			Early: reqBody.WedEarly == "on",
			Mid:   reqBody.WedMid == "on",
			Late:  reqBody.WedLate == "on",
		},
		{
			Name:  "Thurs",
			Early: reqBody.ThursEarly == "on",
			Mid:   reqBody.ThursMid == "on",
			Late:  reqBody.ThursLate == "on",
		},
		{
			Name:  "Fri",
			Early: reqBody.FriEarly == "on",
			Mid:   reqBody.FriMid == "on",
			Late:  reqBody.FriLate == "on",
		},
		{
			Name:  "Sat",
			Early: reqBody.SatEarly == "on",
			Mid:   reqBody.SatMid == "on",
			Late:  reqBody.SatLate == "on",
		},
		{
			Name:  "Sun",
			Early: reqBody.SunEarly == "on",
			Mid:   reqBody.SunMid == "on",
			Late:  reqBody.SunLate == "on",
		},
		{
			Name:  "Mon",
			Early: reqBody.MonEarly == "on",
			Mid:   reqBody.MonMid == "on",
			Late:  reqBody.MonLate == "on",
		},
	}
	data := ProfileData{
		AdminRights:       activeStaff.IsAdmin,
		StaffMember:       *staff,
		RosterLive:        s.LoadRosterWeek(staff.Config.RosterStartDate).IsLive,
		ShowUpdateSuccess: true,
	}
	s.SaveStaffMember(*staff)
	s.renderTemplate(w, "profile", data)
}

type DeleteLeaveBody struct {
	ID      string `json:"id"`
	StaffID string `json:"staffID"`
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
	adminDelete := thisStaff.ID != staffMember.ID
	if adminDelete {
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
