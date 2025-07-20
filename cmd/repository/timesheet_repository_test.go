package repository

import (
	"testing"
	"time"

	"roster/cmd/models"
)

// helper to build entries with given start/end times
func makeEntry(start, end time.Time) *models.TimesheetEntry {
	return &models.TimesheetEntry{ShiftStart: start, ShiftEnd: end}
}

func TestSortTimesheetEntries_OrderAndOriginalUnchanged(t *testing.T) {
	base := time.Date(2022, 1, 1, 8, 0, 0, 0, time.UTC)
	a := makeEntry(base.Add(2*time.Hour), base.Add(3*time.Hour))                // 10-11
	b := makeEntry(base.Add(1*time.Hour), base.Add(4*time.Hour))                // 9-12
	c := makeEntry(base.Add(2*time.Hour), base.Add(2*time.Hour+30*time.Minute)) //10-10:30
	input := []*models.TimesheetEntry{a, b, c}
	// copy original
	orig := make([]*models.TimesheetEntry, len(input))
	copy(orig, input)
	sorted := SortTimesheetEntries(input)
	// Expect order: b (9-12), c (10-10:30), a (10-11)
	if sorted[0] != b || sorted[1] != c || sorted[2] != a {
		t.Errorf("SortTimesheetEntries: wrong order, got %+v", sorted)
	}
	// input should remain unchanged
	for i := range input {
		if input[i] != orig[i] {
			t.Errorf("SortTimesheetEntries: mutated input at index %d", i)
		}
	}
}
