package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

func TestGetStaffFromList(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	s1 := &StaffMember{ID: id1}
	s2 := &StaffMember{ID: id2}
	list := []*StaffMember{s1, s2}
	if got := GetStaffFromList(id2, list); got != s2 {
		t.Errorf("expected staff %v, got %v", s2, got)
	}
	// non-existent
	if got := GetStaffFromList(uuid.New(), list); got != nil {
		t.Errorf("expected nil for missing id, got %v", got)
	}
}

func TestGetConflict(t *testing.T) {
	base := StaffMember{
		Availability: []DayAvailability{
			{Name: "Day0", Early: false, Mid: false, Late: false},
			{Name: "Day1", Early: true, Mid: false, Late: true},
		},
	}
	// fully unavailable
	if got := base.GetConflict("Early", 0); got != PrefRefuse {
		t.Errorf("expected PrefRefuse, got %v", got)
	}
	// conflict slot
	if got := base.GetConflict("Mid", 1); got != PrefConflict {
		t.Errorf("expected PrefConflict, got %v", got)
	}
	// no conflict
	if got := base.GetConflict("Late", 1); got != None {
		t.Errorf("expected None, got %v", got)
	}
}

func TestCustomDateUnmarshalJSON(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{`"02/01/2006"`, true},
		{`"2006-01-02"`, true},
		{`"15:04"`, true},
		{`"invalid-date"`, false},
	}
	for _, tc := range tests {
		var cd CustomDate
		err := cd.UnmarshalJSON([]byte(tc.input))
		if err != nil {
			t.Errorf("UnmarshalJSON error %v for input %s", err, tc.input)
		}
		if tc.valid {
			if cd.Time == nil {
				t.Errorf("expected non-nil Time for input %s", tc.input)
			}
		} else {
			if cd.Time != nil {
				t.Errorf("expected nil Time for invalid input %s, got %v", tc.input, cd.Time)
			}
		}
	}
}

func TestBSONMarshalUnmarshal_RoundTrip(t *testing.T) {
	now := time.Date(2021, 12, 25, 0, 0, 0, 0, time.Local)
	lr := LeaveRequest{
		ID:           uuid.New(),
		CreationDate: CustomDate{&now},
		StartDate:    CustomDate{&now},
		EndDate:      CustomDate{&now},
		Reason:       "Test",
	}
	staff := StaffMember{
		ID:            uuid.New(),
		GoogleID:      "g123",
		LeaveRequests: []LeaveRequest{lr},
	}
	data, err := bson.Marshal(staff)
	if err != nil {
		t.Fatalf("Marshal BSON failed: %v", err)
	}
	var got StaffMember
	if err := bson.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal BSON failed: %v", err)
	}
	if got.ID != staff.ID {
		t.Errorf("expected ID %v, got %v", staff.ID, got.ID)
	}
	if len(got.LeaveRequests) != 1 {
		t.Fatalf("expected 1 LeaveRequest, got %d", len(got.LeaveRequests))
	}
	orig := staff.LeaveRequests[0]
	rc := got.LeaveRequests[0]
	if !rc.CreationDate.Equal(*orig.CreationDate.Time) && !rc.CreationDate.Time.Equal(*orig.CreationDate.Time) {
		t.Errorf("expected CreationDate %v, got %v", orig.CreationDate.Time, rc.CreationDate.Time)
	}
}
