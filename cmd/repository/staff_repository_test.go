package repository

import (
	"testing"
	"time"

	"roster/cmd/models"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

func TestStaffMemberBSON_LeaveRequestsFieldName(t *testing.T) {
	now := time.Now()
	end := now.AddDate(0, 0, 1)

	staff := models.StaffMember{
		ID: uuid.New(),
		LeaveRequests: []models.LeaveRequest{
			{
				ID:           uuid.New(),
				CreationDate: models.CustomDate{Time: &now},
				StartDate:    models.CustomDate{Time: &now},
				EndDate:      models.CustomDate{Time: &end},
			},
		},
	}

	data, err := bson.Marshal(staff)
	if err != nil {
		t.Fatalf("bson.Marshal: %v", err)
	}

	var doc bson.M
	if err := bson.Unmarshal(data, &doc); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	if _, ok := doc["leaverequests"]; !ok {
		t.Fatalf("expected BSON to contain key 'leaverequests', got keys: %#v", doc)
	}
	if _, ok := doc["leaveRequests"]; ok {
		t.Fatalf("unexpected BSON key 'leaveRequests' (camelCase) present")
	}

	leavesAny := doc["leaverequests"]
	leaves, ok := leavesAny.(bson.A)
	if !ok {
		t.Fatalf("expected 'leaverequests' to be bson.A, got %T", leavesAny)
	}
	if len(leaves) != 1 {
		t.Fatalf("expected 1 leave request, got %d", len(leaves))
	}

	firstAny := leaves[0]
	first, ok := firstAny.(bson.M)
	if !ok {
		t.Fatalf("expected leave request element to be bson.M, got %T", firstAny)
	}
	if _, ok := first["id"]; !ok {
		t.Fatalf("expected leave request element to contain key 'id', got %#v", first)
	}
}
