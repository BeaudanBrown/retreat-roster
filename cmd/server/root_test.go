package server

import (
	"testing"

	"github.com/google/uuid"
	"roster/cmd/models"
	"roster/cmd/utils"
)

// simple fake staff repository for testing
// fakeStaffRepo implements repository.StaffRepository for testing, only LoadAllStaff is functional.
type fakeStaffRepo struct{ staff []*models.StaffMember }

func (f *fakeStaffRepo) SaveStaffMember(staff models.StaffMember) error         { return nil }
func (f *fakeStaffRepo) SaveStaffMembers(staff []*models.StaffMember) error     { return nil }
func (f *fakeStaffRepo) LoadAllStaff() ([]*models.StaffMember, error)           { return f.staff, nil }
func (f *fakeStaffRepo) GetStaffByGoogleID(string) (*models.StaffMember, error) { return nil, nil }
func (f *fakeStaffRepo) GetStaffByID(uuid.UUID) (*models.StaffMember, error)    { return nil, nil }
func (f *fakeStaffRepo) GetStaffByToken(uuid.UUID) (*models.StaffMember, error) { return nil, nil }
func (f *fakeStaffRepo) RefreshStaffConfig(models.StaffMember) (models.StaffMember, error) {
	return models.StaffMember{}, nil
}
func (f *fakeStaffRepo) UpdateStaffToken(*models.StaffMember, uuid.UUID) error       { return nil }
func (f *fakeStaffRepo) CreateStaffMember(string, uuid.UUID) error                   { return nil }
func (f *fakeStaffRepo) DeleteLeaveReqByID(models.StaffMember, uuid.UUID) error      { return nil }
func (f *fakeStaffRepo) GetStaffByLeaveReqID(uuid.UUID) (*models.StaffMember, error) { return nil, nil }
func (f *fakeStaffRepo) CreateTrial(string) error                                    { return nil }
func (f *fakeStaffRepo) DeleteStaffByID(uuid.UUID) error                             { return nil }

// Test MemberIsAssigned returns true when IDs match, false otherwise.
func TestMemberIsAssigned(t *testing.T) {
	id := uuid.New()
	if MemberIsAssigned(id, nil) {
		t.Error("expected false when assignedID is nil")
	}
	// matching
	if !MemberIsAssigned(id, &id) {
		t.Error("expected true when IDs match")
	}
	other := uuid.New()
	if MemberIsAssigned(id, &other) {
		t.Error("expected false when IDs differ")
	}
}

// Test MakeRootStruct populates fields correctly using fake repo.
func TestMakeRootStruct(t *testing.T) {
	// prepare fake staff list
	s1 := &models.StaffMember{ID: uuid.New()}
	s2 := &models.StaffMember{ID: uuid.New()}
	fakeRepo := &fakeStaffRepo{staff: []*models.StaffMember{s1, s2}}
	srv := &Server{Repos: Repositories{Staff: fakeRepo}}
	// make dummy week
	wk := models.RosterWeek{ID: uuid.New(), WeekOffset: 2}
	// active staff
	active := models.StaffMember{ID: s1.ID}
	root := srv.MakeRootStruct(active, wk)
	// pointer to server
	if root.Server != srv {
		t.Error("MakeRootStruct: Server pointer not set correctly")
	}
	if root.ActiveStaff.ID != active.ID {
		t.Errorf("MakeRootStruct: ActiveStaff.ID = %v; want %v", root.ActiveStaff.ID, active.ID)
	}
	if root.RosterWeek.ID != wk.ID {
		t.Errorf("MakeRootStruct: RosterWeek.ID = %v; want %v", root.RosterWeek.ID, wk.ID)
	}
	if len(root.Staff) != 2 || root.Staff[0] != s1 || root.Staff[1] != s2 {
		t.Errorf("MakeRootStruct: Staff list does not match fake repo, got %v", root.Staff)
	}
}

// Test MakeDayStruct sets date via RosterDateOffset and returns staff list
func TestMakeDayStruct(t *testing.T) {
	// fake staff
	sA := &models.StaffMember{ID: uuid.New()}
	fakeRepo := &fakeStaffRepo{staff: []*models.StaffMember{sA}}
	srv := &Server{Repos: Repositories{Staff: fakeRepo}}
	// active staff with RosterDateOffset 1
	active := models.StaffMember{ID: uuid.New(), Config: models.StaffConfig{RosterDateOffset: 1}}
	// day with offset 3
	day := models.RosterDay{ID: uuid.New(), Offset: 3}
	ds := MakeDayStruct(true, day, srv, active)
	// date should be WeekStartFromOffset(1) + 3 days
	base := utils.WeekStartFromOffset(1)
	wantDate := base.AddDate(0, 0, 3)
	if !ds.Date.Equal(wantDate) {
		t.Errorf("MakeDayStruct: Date = %v; want %v", ds.Date, wantDate)
	}
	if !ds.IsLive {
		t.Error("MakeDayStruct: IsLive should be set true")
	}
	if ds.RosterDay.ID != day.ID {
		t.Errorf("MakeDayStruct: RosterDay.ID = %v; want %v", ds.RosterDay.ID, day.ID)
	}
	if len(ds.Staff) != 1 || ds.Staff[0] != sA {
		t.Errorf("MakeDayStruct: Staff list = %v; want fake repo list", ds.Staff)
	}
	if ds.ActiveStaff.ID != active.ID {
		t.Errorf("MakeDayStruct: ActiveStaff.ID = %v; want %v", ds.ActiveStaff.ID, active.ID)
	}
}
