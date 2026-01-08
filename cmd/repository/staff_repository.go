package repository

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"roster/cmd/models"
	"roster/cmd/utils"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// StaffRepository defines persistence operations for staff members.
type StaffRepository interface {
	SaveStaffMember(staff models.StaffMember) error
	SaveStaffMembers(staff []*models.StaffMember) error
	LoadAllStaff() ([]*models.StaffMember, error)
	GetStaffByGoogleID(googleID string) (*models.StaffMember, error)
	GetStaffByID(id uuid.UUID) (*models.StaffMember, error)
	GetStaffByToken(token uuid.UUID) (*models.StaffMember, error)
	RefreshStaffConfig(staff models.StaffMember) (models.StaffMember, error)
	UpdateStaffToken(staff *models.StaffMember, token uuid.UUID) error
	CreateStaffMember(googleID string, token uuid.UUID) error
	DeleteLeaveReqByID(staff models.StaffMember, leaveReqID uuid.UUID) error
	GetStaffByLeaveReqID(leaveReqID uuid.UUID) (*models.StaffMember, error)
	CreateTrial(trialName string) error
	DeleteStaffByID(id uuid.UUID) error
}

const ConfigRefreshTime = time.Hour

// MongoStaffRepository implements StaffRepository using MongoDB.
type MongoStaffRepository struct {
	collection *mongo.Collection
	ctx        context.Context
}

// NewMongoStaffRepository creates a new instance of MongoStaffRepository.
func NewMongoStaffRepository(ctx context.Context, db *mongo.Database) *MongoStaffRepository {
	return &MongoStaffRepository{
		collection: db.Collection("staff"),
		ctx:        ctx,
	}
}

func (repo *MongoStaffRepository) SaveStaffMember(staff models.StaffMember) error {
	filter := bson.M{"id": staff.ID}
	update := bson.M{"$set": staff}
	opts := options.Update().SetUpsert(true)
	_, err := repo.collection.UpdateOne(repo.ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("SaveStaffMember: %w", err)
	}
	return nil
}

func (repo *MongoStaffRepository) SaveStaffMembers(staffMembers []*models.StaffMember) error {
	modelsList := make([]mongo.WriteModel, len(staffMembers))
	for i, s := range staffMembers {
		filter := bson.M{"id": s.ID}
		update := bson.M{"$set": s}
		modelsList[i] = mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update).SetUpsert(true)
	}
	opts := options.BulkWrite().SetOrdered(false)
	results, err := repo.collection.BulkWrite(repo.ctx, modelsList, opts)
	if err != nil {
		return fmt.Errorf("SaveStaffMembers: %w", err)
	}
	utils.PrintLog("Bulk saved staff members: %d modified, %d upserted", results.ModifiedCount, results.UpsertedCount)
	return nil
}

func (repo *MongoStaffRepository) LoadAllStaff() ([]*models.StaffMember, error) {
	cursor, err := repo.collection.Find(repo.ctx, bson.M{"isdeleted": bson.M{"$ne": true}})
	if err != nil {
		return nil, fmt.Errorf("LoadAllStaff: %w", err)
	}
	defer cursor.Close(repo.ctx)

	var allStaff []*models.StaffMember
	for cursor.Next(repo.ctx) {
		var s models.StaffMember
		if err := cursor.Decode(&s); err != nil {
			utils.PrintError(err, "Error decoding staff member")
			continue
		}
		// Only include valid records.
		if s.FirstName != "" {
			allStaff = append(allStaff, &s)
		}
	}
	// Sort by effective name (prefer NickName if set).
	sort.Slice(allStaff, func(i, j int) bool {
		name1 := allStaff[i].FirstName
		if allStaff[i].NickName != "" {
			name1 = allStaff[i].NickName
		}
		name2 := allStaff[j].FirstName
		if allStaff[j].NickName != "" {
			name2 = allStaff[j].NickName
		}
		return name1 < name2
	})
	return allStaff, nil
}

func (repo *MongoStaffRepository) GetStaffByGoogleID(googleID string) (*models.StaffMember, error) {
	filter := bson.M{"googleid": googleID, "isdeleted": bson.M{"$ne": true}}
	var s models.StaffMember
	if err := repo.collection.FindOne(repo.ctx, filter).Decode(&s); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetStaffByGoogleID: %w", err)
	}
	return &s, nil
}

func (repo *MongoStaffRepository) GetStaffByID(id uuid.UUID) (*models.StaffMember, error) {
	filter := bson.M{"id": id, "isdeleted": bson.M{"$ne": true}}
	var s models.StaffMember
	if err := repo.collection.FindOne(repo.ctx, filter).Decode(&s); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetStaffByID: %w", err)
	}
	return &s, nil
}

func (repo *MongoStaffRepository) GetStaffByToken(token uuid.UUID) (*models.StaffMember, error) {
	filter := bson.M{"tokens": bson.M{"$elemMatch": bson.M{"$eq": token}}, "isdeleted": bson.M{"$ne": true}}
	var s models.StaffMember
	if err := repo.collection.FindOne(repo.ctx, filter).Decode(&s); err != nil {
		return nil, fmt.Errorf("GetStaffByToken: %w", err)
	}
	return &s, nil
}

func (repo *MongoStaffRepository) RefreshStaffConfig(staff models.StaffMember) (models.StaffMember, error) {
	if time.Since(staff.Config.LastVisit) > ConfigRefreshTime {
		// Use our central time helper to set start dates.
		staff.Config.RosterDateOffset = utils.WeekOffsetFromDate(utils.GetLastTuesday())
		staff.Config.TimesheetDateOffset = utils.WeekOffsetFromDate(utils.GetLastTuesday())
	}
	staff.Config.LastVisit = time.Now()
	if err := repo.SaveStaffMember(staff); err != nil {
		return staff, fmt.Errorf("RefreshStaffConfig: %w", err)
	}
	return staff, nil
}

func (repo *MongoStaffRepository) UpdateStaffToken(staff *models.StaffMember, token uuid.UUID) error {
	if !slices.Contains(staff.Tokens, token) {
		staff.Tokens = append(staff.Tokens, token)
		return repo.SaveStaffMember(*staff)
	}
	return nil
}

func (repo *MongoStaffRepository) CreateStaffMember(googleID string, token uuid.UUID) error {
	existing, err := repo.GetStaffByGoogleID(googleID)
	if err != nil {
		return fmt.Errorf("CreateStaffMember: %w", err)
	}
	if existing != nil {
		return errors.New("staff exists")
	}
	allStaff, err := repo.LoadAllStaff()
	if err != nil {
		return fmt.Errorf("CreateStaffMember: %w", err)
	}
	isFirstUser := len(allStaff) == 0
	role := models.Staff
	if isFirstUser {
		role = models.AdminRole
	}
	newStaff := models.StaffMember{
		ID:        uuid.New(),
		GoogleID:  googleID,
		FirstName: "",
		// Keep legacy IsAdmin for backwards-compat until handlers/templates are updated.
		IsAdmin:      isFirstUser,
		Role:         role,
		Tokens:       []uuid.UUID{token},
		Availability: emptyAvailability(),
		Config: models.StaffConfig{
			LastVisit:           time.Now(),
			TimesheetDateOffset: utils.WeekOffsetFromDate(utils.GetLastTuesday()),
			RosterDateOffset:    utils.WeekOffsetFromDate(utils.GetLastTuesday()),
		},
	}
	return repo.SaveStaffMember(newStaff)
}

func (repo *MongoStaffRepository) DeleteLeaveReqByID(staff models.StaffMember, leaveReqID uuid.UUID) error {
	var updated []models.LeaveRequest
	for _, lr := range staff.LeaveRequests {
		if lr.ID == leaveReqID {
			continue
		}
		updated = append(updated, lr)
	}
	staff.LeaveRequests = updated
	return repo.SaveStaffMember(staff)
}

func (repo *MongoStaffRepository) GetStaffByLeaveReqID(leaveReqID uuid.UUID) (*models.StaffMember, error) {
	// BSON uses lowercased field names by default, so `LeaveRequests` is stored as `leaverequests`.
	filter := bson.M{
		"leaverequests": bson.M{
			"$elemMatch": bson.M{"id": leaveReqID},
		},
		"isdeleted": bson.M{"$ne": true},
	}
	var s models.StaffMember
	if err := repo.collection.FindOne(repo.ctx, filter).Decode(&s); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetStaffByLeaveReqID: %w", err)
	}
	return &s, nil
}

func (repo *MongoStaffRepository) CreateTrial(trialName string) error {
	newStaff := models.StaffMember{
		ID:           uuid.New(),
		GoogleID:     "Trial",
		IsTrial:      true,
		FirstName:    trialName,
		Availability: emptyAvailability(),
		IdealShifts:  7,
	}
	return repo.SaveStaffMember(newStaff)
}

func (repo *MongoStaffRepository) DeleteStaffByID(id uuid.UUID) error {
	filter := bson.M{"id": id}
	update := bson.M{"$set": bson.M{"isdeleted": true}}
	res, err := repo.collection.UpdateOne(repo.ctx, filter, update)
	if err != nil {
		return fmt.Errorf("DeleteStaffByID: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("DeleteStaffByID: no document found")
	}
	return nil
}

// emptyAvailability returns a default DayAvailability slice.
func emptyAvailability() []models.DayAvailability {
	return []models.DayAvailability{
		{
			Name:  "Tues",
			Early: true,
			Mid:   true,
			Late:  true,
		},
		{
			Name:  "Wed",
			Early: true,
			Mid:   true,
			Late:  true,
		},
		{
			Name:  "Thurs",
			Early: true,
			Mid:   true,
			Late:  true,
		},
		{
			Name:  "Fri",
			Early: true,
			Mid:   true,
			Late:  true,
		},
		{
			Name:  "Sat",
			Early: true,
			Mid:   true,
			Late:  true,
		},
		{
			Name:  "Sun",
			Early: true,
			Mid:   true,
			Late:  true,
		},
		{
			Name:  "Mon",
			Early: true,
			Mid:   true,
			Late:  true,
		},
	}
}
