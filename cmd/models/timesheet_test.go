package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGetAllShiftTypes(t *testing.T) {
	types := GetAllShiftTypes()
	// Expect values from Bar (0) to Admin inclusive
	wantLen := int(Admin) - int(Bar) + 1
	if len(types) != wantLen {
		t.Errorf("GetAllShiftTypes: expected length %d, got %d", wantLen, len(types))
	}
	for i, st := range types {
		if int(st) != i {
			t.Errorf("GetAllShiftTypes: index %d, expected %d, got %d", i, i, st)
		}
	}
}

func TestShiftType_StringAndInt(t *testing.T) {
	names := []string{
		"Bar",
		"Deliveries",
		"Day Manager",
		"Amelia Supervisor",
		"Night Manager",
		"General Management",
		"Kitchen",
		"Admin",
	}
	for i, want := range names {
		st := ShiftType(i)
		if st.String() != want {
			t.Errorf("ShiftType.String: for %d expected %q, got %q", i, want, st.String())
		}
		if st.Int() != i {
			t.Errorf("ShiftType.Int: for %v expected %d, got %d", st, i, st.Int())
		}
	}
}

func TestStringToShiftType(t *testing.T) {
	// valid
	if st := StringToShiftType("3"); st != AmeliaSupervisor {
		t.Errorf("StringToShiftType: expected AmeliaSupervisor, got %v", st)
	}
	// out of range
	if st := StringToShiftType("99"); st != Bar {
		t.Errorf("StringToShiftType out-of-range: expected Bar, got %v", st)
	}
	// invalid
	if st := StringToShiftType("foo"); st != Bar {
		t.Errorf("StringToShiftType invalid: expected Bar, got %v", st)
	}
}

func TestDisableTimesheet_OutOfRange(t *testing.T) {
	oldDate := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
	// non-admin should be disabled for old dates
	if got := DisableTimesheet(oldDate, false); !got {
		t.Errorf("DisableTimesheet: expected true for oldDate, non-admin, got %v", got)
	}
	// admin should be allowed (disabled == false)
	if got := DisableTimesheet(oldDate, true); got {
		t.Errorf("DisableTimesheet: expected false for oldDate, admin, got %v", got)
	}
}

func TestBSONMarshalUnmarshal_TimesheetEntry(t *testing.T) {
	// Prepare times in local zone
	baseDate := time.Date(2022, 5, 10, 0, 0, 0, 0, time.Local)
	start := baseDate.Add(9 * time.Hour)
	end := baseDate.Add(17 * time.Hour)
	breakStart := baseDate.Add(12 * time.Hour)
	breakEnd := baseDate.Add(12*time.Hour + 30*time.Minute)
	entry := TimesheetEntry{
		ID:          uuid.New(),
		StaffID:     uuid.New(),
		WeekOffset:  0,
		DayOffset:   2,
		StartDate:   baseDate,
		ShiftStart:  start,
		ShiftEnd:    end,
		HasBreak:    true,
		BreakStart:  breakStart,
		BreakEnd:    breakEnd,
		BreakLength: 0.5,
		ShiftLength: 8.0,
		Approved:    true,
		ShiftType:   NightManager,
	}
	data, err := entry.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON failed: %v", err)
	}
	var got TimesheetEntry
	if err := got.UnmarshalBSON(data); err != nil {
		t.Fatalf("UnmarshalBSON failed: %v", err)
	}
	compares := []struct {
		name       string
		want, have time.Time
	}{
		{"StartDate", entry.StartDate, got.StartDate},
		{"ShiftStart", entry.ShiftStart, got.ShiftStart},
		{"ShiftEnd", entry.ShiftEnd, got.ShiftEnd},
		{"BreakStart", entry.BreakStart, got.BreakStart},
		{"BreakEnd", entry.BreakEnd, got.BreakEnd},
	}
	for _, c := range compares {
		if !c.have.Equal(c.want) {
			t.Errorf("%s mismatch: want %v, got %v", c.name, c.want, c.have)
		}
	}
	if got.HasBreak != entry.HasBreak || got.BreakLength != entry.BreakLength ||
		got.ShiftLength != entry.ShiftLength || got.Approved != entry.Approved ||
		got.ShiftType != entry.ShiftType {
		t.Error("Non-time fields did not match after BSON roundtrip")
	}
	if got.ID != entry.ID || got.StaffID != entry.StaffID || got.WeekOffset != entry.WeekOffset ||
		got.DayOffset != entry.DayOffset {
		t.Error("Basic metadata did not match after BSON roundtrip")
	}
}
