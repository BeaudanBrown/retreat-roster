package repository

import (
	"context"
	"sort"
	"time"

	"roster/cmd/models"
	"roster/cmd/utils"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TimesheetRepository defines persistence operations for staff members.
type TimesheetRepository interface {
	SaveTimesheetEntry(e TimesheetEntry) error
	GetTimesheetEntryByID(entryID uuid.UUID) (*models.TimesheetEntry, error)
	GetAllTimesheetEntries() *[]*TimesheetEntry
	SaveAllTimesheetEntries(entries []*TimesheetEntry) error
	GetStaffTimesheetWeek(staffID uuid.UUID, startDate time.Time) (*[]*TimesheetEntry, error)
	GetTimesheetWeek(startDate time.Time) (*[]*TimesheetEntry, error)
	DeleteTimesheetEntry(entryID uuid.UUID) error
}

// MongoTimesheetRepository implements TimesheetRepository using MongoDB.
type MongoTimesheetRepository struct {
	collection *mongo.Collection
	ctx        context.Context
}

// NewMongoTimesheetRepository creates a new instance of MongoTimesheetRepository.
func NewMongoTimesheetRepository(ctx context.Context, db *mongo.Database) *MongoTimesheetRepository {
	return &MongoTimesheetRepository{
		collection: db.Collection("timesheets"),
		ctx:        ctx,
	}
}

type TimesheetEntry = models.TimesheetEntry

func (repo *MongoTimesheetRepository) SaveTimesheetEntry(e TimesheetEntry) error {
	filter := bson.M{"id": e.ID}
	update := bson.M{"$set": e}
	opts := options.Update().SetUpsert(true)
	_, err := repo.collection.UpdateOne(repo.ctx, filter, update, opts)
	if err != nil {
		utils.PrintError(err, "Failed to save timesheet entry")
		return err
	}
	utils.PrintLog("Saved timesheet entry")
	return nil
}

func (repo *MongoTimesheetRepository) GetTimesheetEntryByID(entryID uuid.UUID) (*models.TimesheetEntry, error) {
	filter := bson.M{"id": entryID}

	var timesheetEntry models.TimesheetEntry
	err := repo.collection.FindOne(repo.ctx, filter).Decode(&timesheetEntry)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.PrintError(err, "No timesheet entry with id")
			return nil, err
		}
		utils.PrintError(err, "Error getting timesheet entry")
		return nil, err
	}
	return &timesheetEntry, nil
}

func SortTimesheetEntries(entries []*TimesheetEntry) []*TimesheetEntry {
	copiedEntries := make([]*TimesheetEntry, len(entries))
	copy(copiedEntries, entries)
	sort.Slice(copiedEntries, func(i, j int) bool {
		entry1 := copiedEntries[i]
		entry2 := copiedEntries[j]
		if entry1.ShiftStart.Equal(entry2.ShiftStart) {
			return entry1.ShiftEnd.Before(entry2.ShiftEnd)
		}
		return entry1.ShiftStart.Before(entry2.ShiftStart)
	})
	return copiedEntries
}

func (repo *MongoTimesheetRepository) GetAllTimesheetEntries() *[]*TimesheetEntry {
	cursor, err := repo.collection.Find(repo.ctx, bson.M{})
	if err != nil {
		utils.PrintError(err, "Error executing query")
		return nil
	}
	defer cursor.Close(repo.ctx)
	var entries []*TimesheetEntry
	if err = cursor.All(repo.ctx, &entries); err != nil {
		utils.PrintError(err, "Error decoding timesheet entries")
		return nil
	}
	entries = SortTimesheetEntries(entries)
	return &entries
}

func (repo *MongoTimesheetRepository) SaveAllTimesheetEntries(entries []*TimesheetEntry) error {
	bulkWriteModels := make([]mongo.WriteModel, len(entries))
	for i, entry := range entries {
		filter := bson.M{"id": entry.ID}
		update := bson.M{"$set": *entry}
		bulkWriteModels[i] = mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update).SetUpsert(true)
	}

	opts := options.BulkWrite().SetOrdered(false)
	results, err := repo.collection.BulkWrite(repo.ctx, bulkWriteModels, opts)
	if err != nil {
		utils.PrintError(err, "Failed to save timesheet entries")
		return err
	}
	utils.PrintLog("Saved %v timesheet entries, Upserted %v timesheet entries", results.ModifiedCount, results.UpsertedCount)
	return nil
}

func (repo *MongoTimesheetRepository) GetStaffTimesheetWeek(staffID uuid.UUID, startDate time.Time) (*[]*TimesheetEntry, error) {
	year, month, day := startDate.Date()
	weekStart := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	weekEnd := weekStart.AddDate(0, 0, 7)
	filter := bson.M{
		"startDate": bson.M{
			"$gte": weekStart.UTC(),
			"$lt":  weekEnd.UTC(),
		},
		// TODO: Fucked up the bson name
		"days": staffID,
	}

	cursor, err := repo.collection.Find(repo.ctx, filter)
	if err != nil {
		utils.PrintError(err, "Error finding timesheet week")
		return nil, err
	}
	defer cursor.Close(repo.ctx)
	var entries []*TimesheetEntry
	if err = cursor.All(repo.ctx, &entries); err != nil {
		utils.PrintError(err, "Error decoding timesheet weeks")
		return nil, err
	}
	return &entries, nil
}

func (repo *MongoTimesheetRepository) GetTimesheetWeek(startDate time.Time) (*[]*TimesheetEntry, error) {
	year, month, day := startDate.Date()
	weekStart := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	weekEnd := weekStart.AddDate(0, 0, 7)
	filter := bson.M{
		"startDate": bson.M{
			"$gte": weekStart.UTC(),
			"$lt":  weekEnd.UTC(),
		},
	}

	cursor, err := repo.collection.Find(repo.ctx, filter)
	if err != nil {
		utils.PrintError(err, "Error finding timesheet week")
		return nil, err
	}
	defer cursor.Close(repo.ctx)
	var entries []*TimesheetEntry
	if err = cursor.All(repo.ctx, &entries); err != nil {
		utils.PrintError(err, "Error decoding timesheet weeks")
		return nil, err
	}
	entries = SortTimesheetEntries(entries)
	return &entries, nil
}

func (repo *MongoTimesheetRepository) DeleteTimesheetEntry(entryID uuid.UUID) error {
	filter := bson.M{"id": entryID}
	_, err := repo.collection.DeleteOne(repo.ctx, filter)
	if err != nil {
		return err
	}
	return nil
}
