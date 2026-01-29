package server

import (
	"context"
	"html/template"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"roster/cmd/models"
	"roster/cmd/utils"
)

// simple fake staff repository for testing
// fakeStaffRepo implements repository.StaffRepository for testing
type fakeStaffRepo struct {
	staff       []*models.StaffMember
	tokenToUser map[uuid.UUID]*models.StaffMember
}

func (f *fakeStaffRepo) SaveStaffMember(staff models.StaffMember) error         { return nil }
func (f *fakeStaffRepo) SaveStaffMembers(staff []*models.StaffMember) error     { return nil }
func (f *fakeStaffRepo) LoadAllStaff() ([]*models.StaffMember, error)           { return f.staff, nil }
func (f *fakeStaffRepo) GetStaffByGoogleID(string) (*models.StaffMember, error) { return nil, nil }
func (f *fakeStaffRepo) GetStaffByID(uuid.UUID) (*models.StaffMember, error)    { return nil, nil }
func (f *fakeStaffRepo) GetStaffByToken(token uuid.UUID) (*models.StaffMember, error) {
	if s, ok := f.tokenToUser[token]; ok {
		return s, nil
	}
	return nil, nil
}
func (f *fakeStaffRepo) RefreshStaffConfig(s models.StaffMember) (models.StaffMember, error) {
	return s, nil
}
func (f *fakeStaffRepo) UpdateStaffToken(*models.StaffMember, uuid.UUID) error       { return nil }
func (f *fakeStaffRepo) CreateStaffMember(string, uuid.UUID) error                   { return nil }
func (f *fakeStaffRepo) DeleteLeaveReqByID(models.StaffMember, uuid.UUID) error      { return nil }
func (f *fakeStaffRepo) GetStaffByLeaveReqID(uuid.UUID) (*models.StaffMember, error) { return nil, nil }
func (f *fakeStaffRepo) CreateTrial(string) error                                    { return nil }
func (f *fakeStaffRepo) DeleteStaffByID(uuid.UUID) error                             { return nil }

// simple fake roster week repository for testing
type fakeRosterWeekRepo struct {
	weeks map[int]*models.RosterWeek
	saved []*models.RosterWeek
}

func (f *fakeRosterWeekRepo) SaveRosterWeek(week *models.RosterWeek) error {
	f.saved = append(f.saved, week)
	f.weeks[week.WeekOffset] = week
	return nil
}
func (f *fakeRosterWeekRepo) SaveAllRosterWeeks(weeks []*models.RosterWeek) error { return nil }
func (f *fakeRosterWeekRepo) LoadAllRosterWeeks() ([]*models.RosterWeek, error)   { return nil, nil }
func (f *fakeRosterWeekRepo) LoadRosterWeek(weekOffset int) (*models.RosterWeek, error) {
	if week, ok := f.weeks[weekOffset]; ok {
		return week, nil
	}
	// Create a new week if it doesn't exist, like the real repository
	newWeek := &models.RosterWeek{
		ID:         uuid.New(),
		WeekOffset: weekOffset,
		Days:       []*models.RosterDay{},
	}
	f.weeks[weekOffset] = newWeek
	return newWeek, nil
}
func (f *fakeRosterWeekRepo) ChangeDayRowCount(weekOffset int, dayID uuid.UUID, action string) (*models.RosterDay, bool, error) {
	return nil, false, nil
}

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

// Test HandleImportRosterWeek imports the immediately previous week (offset - 1), not 7 weeks ago.
func TestHandleImportRosterWeek(t *testing.T) {
	tests := []struct {
		name            string
		currentOffset   int
		expectedPrevOff int
	}{
		{"import to offset 5", 5, 4},
		{"import to offset 100", 100, 99},
		{"import to offset 0", 0, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create roster weeks for previous week and current week
			prevWeekID := uuid.New()
			currWeekID := uuid.New()

			prevWeek := &models.RosterWeek{
				ID:         prevWeekID,
				WeekOffset: tt.expectedPrevOff,
				Days: []*models.RosterDay{
					{ID: uuid.New(), DayName: "Tues", Offset: 0, Rows: []*models.Row{}},
				},
			}
			currWeek := &models.RosterWeek{
				ID:         currWeekID,
				WeekOffset: tt.currentOffset,
				Days:       []*models.RosterDay{},
			}

			// Create active staff and session
			sessionToken := uuid.New()
			activeStaff := &models.StaffMember{
				ID: uuid.New(),
				Config: models.StaffConfig{
					RosterDateOffset: tt.currentOffset,
				},
			}

			// Setup fake repositories
			fakeRosterRepo := &fakeRosterWeekRepo{
				weeks: map[int]*models.RosterWeek{
					tt.expectedPrevOff: prevWeek,
					tt.currentOffset:   currWeek,
				},
				saved: []*models.RosterWeek{},
			}
			fakeStaffRepo := &fakeStaffRepo{
				staff: []*models.StaffMember{activeStaff},
				tokenToUser: map[uuid.UUID]*models.StaffMember{
					sessionToken: activeStaff,
				},
			}

			// Setup templates
			tmpl := template.New("rosterMainContainer")
			// Add dummy functions required by templates if necessary, or just parse an empty string
			// The handler calls s.MakeRootStruct which calls MakeHeaderStruct etc,
			// but s.renderTemplate just executes the template.
			// The actual template needs valid data, but for this test we can use a dummy template
			// that doesn't use the data.
			tmpl.Parse("")

			srv := &Server{
				Repos: Repositories{
					RosterWeek: fakeRosterRepo,
					Staff:      fakeStaffRepo,
				},
				Templates: tmpl,
			}

			// Create request
			req := httptest.NewRequest("POST", "/importRosterWeek", nil)
			// Add session to context
			ctx := context.WithValue(req.Context(), "sessionToken", sessionToken)
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()

			// Call the handler
			srv.HandleImportRosterWeek(w, req)

			// Verify that a week was saved (should be one call for the duplicated week)
			if len(fakeRosterRepo.saved) != 1 {
				t.Errorf("Expected 1 saved week, got %d", len(fakeRosterRepo.saved))
				return
			}

			// Verify the saved week has the correct offset (current week)
			if fakeRosterRepo.saved[0].WeekOffset != tt.currentOffset {
				t.Errorf("Saved week offset = %d; want %d", fakeRosterRepo.saved[0].WeekOffset, tt.currentOffset)
			}

			// Verify that the previous week loaded was from offset - 1, not offset - 7
			// We can verify this by checking if prevWeek's data (the Tues day) made it into the saved week
			if len(fakeRosterRepo.saved[0].Days) == 0 {
				t.Error("Expected days from previous week to be duplicated")
			}
			if len(fakeRosterRepo.saved[0].Days) > 0 && fakeRosterRepo.saved[0].Days[0].DayName != "Tues" {
				t.Errorf("Expected first day to be 'Tues' from previous week, got %q", fakeRosterRepo.saved[0].Days[0].DayName)
			}
		})
	}
}
