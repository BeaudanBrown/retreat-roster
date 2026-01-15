package repository

import (
	"testing"

	"github.com/google/uuid"
	"roster/cmd/models"
)

// Test helper constructors in rosterweek_repository.go
func TestNewRosterWeek(t *testing.T) {
	offset := 5
	week := newRosterWeek(offset)
	// Should have 7 days
	if len(week.Days) != 7 {
		t.Fatalf("newRosterWeek: expected 7 days, got %d", len(week.Days))
	}
	// Offsets should match index and colours alternate
	for i, day := range week.Days {
		if day.Offset != i {
			t.Errorf("newRosterWeek: day %d offset = %d, want %d", i, day.Offset, i)
		}
		wantColour := "#ffffff"
		if i%2 == 0 {
			wantColour = "#b7b7b7"
		}
		if day.Colour != wantColour {
			t.Errorf("newRosterWeek: day %d colour = %s, want %s", i, day.Colour, wantColour)
		}
		// Each day should have 4 rows
		if len(day.Rows) != 4 {
			t.Errorf("newRosterWeek: day %d rows = %d, want 4", i, len(day.Rows))
		}
		// IDs should be set
		if day.ID == uuid.Nil {
			t.Errorf("newRosterWeek: day %d has nil ID", i)
		}
	}
	// WeekOffset and IsLive
	if week.WeekOffset != offset {
		t.Errorf("newRosterWeek: WeekOffset = %d, want %d", week.WeekOffset, offset)
	}
	if week.IsLive {
		t.Errorf("newRosterWeek: IsLive = true, want false")
	}
}

func TestNewRowAndNewSlot(t *testing.T) {
	row := newRow()
	// Row should have 3 slots
	slots := []models.Slot{row.Early, row.Mid, row.Late}
	for _, slot := range slots {
		if slot.ID == uuid.Nil {
			t.Errorf("newRow: slot has nil ID")
		}
		// StartTime defaults to empty
		if slot.StartTime != "" {
			t.Errorf("newRow: expected empty StartTime, got %s", slot.StartTime)
		}
	}
}
