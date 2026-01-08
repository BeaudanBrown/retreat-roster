package server

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"roster/cmd/models"
	"roster/cmd/utils"
)

func TestMakeProfileStruct(t *testing.T) {
	staff := models.StaffMember{ID: uuid.New(), FirstName: "Test", Role: models.AdminRole}
	data := MakeProfileStruct(true, staff, false)
	if data.StaffMember.ID != staff.ID {
		t.Errorf("MakeProfileStruct: StaffMember.ID = %v; want %v", data.StaffMember.ID, staff.ID)
	}
	if !data.RosterLive {
		t.Error("MakeProfileStruct: RosterLive should be true")
	}
	if data.AdminRights {
		t.Error("MakeProfileStruct: AdminRights should be false")
	}
}

func TestMakePickerStruct(t *testing.T) {
	name := "pick"
	label := "Pick Label"
	id := uuid.New()
	weekOffset := 2
	dayOffset := 4
	// use a fixed time
	tm := time.Date(2023, 7, 1, 15, 30, 0, 0, time.Local)
	pd := MakePickerStruct(name, label, id, weekOffset, dayOffset, tm, true)
	if pd.Name != name || pd.Label != label || pd.ID != id {
		t.Error("MakePickerStruct: name/label/ID fields not set correctly")
	}
	// Date should be TueEpoch + weekOffset*7 days, local, then + dayOffset
	wantDate := utils.WeekStartFromOffset(weekOffset).AddDate(0, 0, dayOffset)
	if !pd.Date.Equal(wantDate) {
		t.Errorf("MakePickerStruct: Date = %v; want %v", pd.Date, wantDate)
	}
	if !pd.Time.Equal(tm) {
		t.Errorf("MakePickerStruct: Time = %v; want %v", pd.Time, tm)
	}
	if !pd.Disabled {
		t.Error("MakePickerStruct: Disabled should be true")
	}
}

func TestMakeLeaveReqStruct(t *testing.T) {
	staff := models.StaffMember{ID: uuid.New(), FirstName: "A"}
	success := true
	failure := false
	lr := MakeLeaveReqStruct(staff, success, failure)
	if lr.StaffMember.ID != staff.ID {
		t.Error("MakeLeaveReqStruct: StaffMember not set")
	}
	if !lr.ShowLeaveSuccess || lr.ShowLeaveError {
		t.Error("MakeLeaveReqStruct: flags not set correctly")
	}
}

func TestGetSortedLeaveReqs(t *testing.T) {
	// Build staff with leave requests in unsorted order
	baseDay := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	plusOne := baseDay.AddDate(0, 0, 1)
	subOne := baseDay.AddDate(0, 0, -1)
	// three staff with one leave each
	s1 := &models.StaffMember{ID: uuid.New(), FirstName: "S1", LeaveRequests: []models.LeaveRequest{{
		ID:           uuid.New(),
		StartDate:    models.CustomDate{Time: &baseDay},
		CreationDate: models.CustomDate{Time: &baseDay},
		EndDate:      models.CustomDate{Time: &plusOne},
	}}}
	s2 := &models.StaffMember{ID: uuid.New(), FirstName: "S2", LeaveRequests: []models.LeaveRequest{{
		ID:           uuid.New(),
		StartDate:    models.CustomDate{Time: &subOne},
		CreationDate: models.CustomDate{Time: &subOne},
		EndDate:      models.CustomDate{Time: &baseDay},
	}}}
	all := []*models.StaffMember{s1, s2}
	list := GetSortedLeaveReqs(all)
	if len(list) != 2 {
		t.Fatalf("GetSortedLeaveReqs: expected 2, got %d", len(list))
	}
	// s2 has earlier start
	if list[0].StaffID != s2.ID || list[1].StaffID != s1.ID {
		t.Errorf("GetSortedLeaveReqs: expected order [s2, s1], got [%v, %v]", list[0].StaffID, list[1].StaffID)
	}
}
