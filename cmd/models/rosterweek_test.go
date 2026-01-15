package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGetSlotByIDAndGetDayByID(t *testing.T) {
	// Build a roster week with known IDs
	dayID := uuid.New()
	slotID := uuid.New()
	slot := &Slot{ID: slotID}
	row := &Row{Early: *slot}
	day := &RosterDay{ID: dayID, Rows: []*Row{row}, Offset: 0}
	week := &RosterWeek{Days: []*RosterDay{day}}
	// Test GetDayByID
	if got := week.GetDayByID(dayID); got != day {
		t.Errorf("GetDayByID: expected %+v, got %+v", day, got)
	}
	// Test GetSlotByID
	if got := week.GetSlotByID(slotID); got == nil || got.ID != slotID {
		t.Errorf("GetSlotByID: expected slot ID %v, got %v", slotID, got)
	}
	// Missing IDs
	if week.GetDayByID(uuid.New()) != nil {
		t.Errorf("GetDayByID: expected nil for unknown ID")
	}
	if week.GetSlotByID(uuid.New()) != nil {
		t.Errorf("GetSlotByID: expected nil for unknown slot ID")
	}
}

func TestHasThisStaff(t *testing.T) {
	staffID := uuid.New()
	s := staffID
	slot := Slot{AssignedStaff: &s}
	if !slot.HasThisStaff(staffID) {
		t.Errorf("HasThisStaff: expected true when assignedMatches")
	}
	other := uuid.New()
	if slot.HasThisStaff(other) {
		t.Errorf("HasThisStaff: expected false for non-assigned staff")
	}
}

func TestGetHighlightCol(t *testing.T) {
	defaultCol := "#000000"
	cases := []struct {
		flag Highlight
		want string
	}{
		{Duplicate, "#FFA07A"},
		{PrefConflict, "#FF9999"},
		{LateToEarly, "#117593"},
		{LeaveConflict, "#CC3333"},
		{PrefRefuse, "#CC3333"},
		{IdealMet, "#B2E1B0"},
		{IdealExceeded, "#D7A9A9"},
	}
	for _, c := range cases {
		got := GetHighlightCol(defaultCol, c.flag)
		if got != c.want {
			t.Errorf("GetHighlightCol(%v): expected %s, got %s", c.flag, c.want, got)
		}
	}
	// default
	if got := GetHighlightCol(defaultCol, None); got != defaultCol {
		t.Errorf("GetHighlightCol default: expected %s, got %s", defaultCol, got)
	}
}

func TestSumArray(t *testing.T) {
	arr := []int{1, 2, 3, 4}
	if got := SumArray(arr); got != 10 {
		t.Errorf("SumArray: expected 10, got %d", got)
	}
}

func TestCountShifts(t *testing.T) {
	// Two days, one staff assigned twice
	staffID := uuid.New()
	s := staffID
	// day with one row, Early and Late assigned to the same staff
	day := &RosterDay{Offset: 2}
	row1 := &Row{Early: Slot{AssignedStaff: &s}, Mid: Slot{}, Late: Slot{AssignedStaff: &s}}
	day.Rows = []*Row{row1}
	counts := map[uuid.UUID][]int{staffID: make([]int, 7)}
	day.CountShifts(counts)
	if counts[staffID][2] != 2 {
		t.Errorf("CountShifts: expected 2 shifts at offset 2, got %d", counts[staffID][2])
	}
}

func TestMarshalUnmarshalBSON_RosterWeek_Times(t *testing.T) {
	// Test that Marshal/Unmarshal preserves StartDate in local timezone
	local := time.Date(2022, 3, 14, 0, 0, 0, 0, time.Local)
	week := RosterWeek{ID: uuid.New(), StartDate: local}
	data, err := week.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON failed: %v", err)
	}
	var got RosterWeek
	if err := got.UnmarshalBSON(data); err != nil {
		t.Fatalf("UnmarshalBSON failed: %v", err)
	}
	// Check year-month-day equality
	y1, m1, d1 := week.StartDate.Date()
	y2, m2, d2 := got.StartDate.Date()
	if y1 != y2 || m1 != m2 || d1 != d2 {
		t.Errorf("StartDate mismatch after BSON roundtrip: expected %v, got %v", week.StartDate, got.StartDate)
	}
}
