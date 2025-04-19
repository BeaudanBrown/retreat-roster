package repository

import (
	"context"
	"fmt"
	"time"

	"roster/cmd/models"
	"roster/cmd/utils"

	"github.com/google/uuid"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// RosterWeekRepository defines the interface for RosterWeek persistence.
type RosterWeekRepository interface {
	SaveRosterWeek(week *models.RosterWeek) error
	SaveAllRosterWeeks(weeks []*models.RosterWeek) error
	LoadAllRosterWeeks() ([]*models.RosterWeek, error)
	LoadRosterWeek(startDate time.Time) (*models.RosterWeek, error)
	ChangeDayRowCount(startDate time.Time, dayID uuid.UUID, action string) (*models.RosterDay, bool, error)
}

// MongoRosterWeekRepository is the MongoDB implementation of RosterWeekRepository.
type MongoRosterWeekRepository struct {
	collection *mongo.Collection
	ctx        context.Context
}

// NewMongoRosterWeekRepository creates a new instance of MongoRosterWeekRepository.
// Typically this is called during server/repository initialization.
func NewMongoRosterWeekRepository(ctx context.Context, db *mongo.Database) RosterWeekRepository {
	return &MongoRosterWeekRepository{
		collection: db.Collection("rosters"),
		ctx:        ctx,
	}
}

// SaveRosterWeek saves a single roster week to MongoDB.
func (r *MongoRosterWeekRepository) SaveRosterWeek(week *models.RosterWeek) error {
	filter := bson.M{"id": week.ID}
	update := bson.M{"$set": week}
	opts := options.Update().SetUpsert(true)
	_, err := r.collection.UpdateOne(r.ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to save roster week (id: %v): %w", week.ID, err)
	}
	utils.PrintLog("Saved roster week (id: %v)", week.ID)
	return nil
}

// SaveAllRosterWeeks performs a bulk upsert of roster weeks.
func (r *MongoRosterWeekRepository) SaveAllRosterWeeks(weeks []*models.RosterWeek) error {
	bulkModels := make([]mongo.WriteModel, len(weeks))
	for i, week := range weeks {
		filter := bson.M{"id": week.ID}
		update := bson.M{"$set": week}
		bulkModels[i] = mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update).SetUpsert(true)
	}
	opts := options.BulkWrite().SetOrdered(false)
	results, err := r.collection.BulkWrite(r.ctx, bulkModels, opts)
	if err != nil {
		return fmt.Errorf("failed to bulk save roster weeks: %w", err)
	}
	utils.PrintLog("Bulk saved roster weeks: %d modified, %d upserted", results.ModifiedCount, results.UpsertedCount)
	return nil
}

// LoadAllRosterWeeks returns all roster weeks from the database.
func (r *MongoRosterWeekRepository) LoadAllRosterWeeks() ([]*models.RosterWeek, error) {
	cursor, err := r.collection.Find(r.ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("error executing query for all roster weeks: %w", err)
	}
	defer cursor.Close(r.ctx)

	var weeks []*models.RosterWeek
	for cursor.Next(r.ctx) {
		var week models.RosterWeek
		if err := cursor.Decode(&week); err != nil {
			utils.PrintError(err, "Error decoding roster week")
			continue
		}
		weeks = append(weeks, &week)
	}
	return weeks, nil
}

// LoadRosterWeek returns a roster week for the given startDate. If no document is found,
// a new RosterWeek is created and saved.
func (r *MongoRosterWeekRepository) LoadRosterWeek(startDate time.Time) (*models.RosterWeek, error) {
	// Ensure the time is converted to UTC for comparing with stored ISO dates.
	filter := bson.M{"startDate": startDate.UTC()}
	var rosterWeek models.RosterWeek
	err := r.collection.FindOne(r.ctx, filter).Decode(&rosterWeek)
	if err == mongo.ErrNoDocuments {
		utils.PrintError(err, "Creating new roster week")
		newWeek := newRosterWeek(startDate)
		if saveErr := r.SaveRosterWeek(&newWeek); saveErr != nil {
			return nil, fmt.Errorf("failed to save new roster week: %w", saveErr)
		}
		return &newWeek, nil
	} else if err != nil {
		return nil, fmt.Errorf("error loading roster week: %w", err)
	}
	return &rosterWeek, nil
}

// ChangeDayRowCount modifies the number of rows in a specific RosterDay.
// Returns the affected day, the roster week's live status and an error if applicable.
func (r *MongoRosterWeekRepository) ChangeDayRowCount(startDate time.Time, dayID uuid.UUID, action string) (*models.RosterDay, bool, error) {
	week, err := r.LoadRosterWeek(startDate.UTC())
	if err != nil {
		return nil, false, fmt.Errorf("failed to load roster week for modifying row count: %w", err)
	}

	var affectedDay *models.RosterDay
	for i, day := range week.Days {
		if day.ID == dayID {
			if action == "+" {
				week.Days[i].Rows = append(week.Days[i].Rows, newRow())
			} else {
				if len(week.Days[i].Rows) > 4 {
					week.Days[i].Rows = week.Days[i].Rows[:len(week.Days[i].Rows)-1]
				}
			}
			affectedDay = week.Days[i]
			break
		}
	}

	if affectedDay == nil {
		return nil, week.IsLive, fmt.Errorf("no roster day found with id: %v", dayID)
	}

	if err := r.SaveRosterWeek(week); err != nil {
		return nil, week.IsLive, fmt.Errorf("failed to save roster week after changing day row count: %w", err)
	}

	return affectedDay, week.IsLive, nil
}

// -- Helper functions for repository internal use --

// newRosterWeek is a helper for creating a new RosterWeek from a startDate.
func newRosterWeek(startDate time.Time) models.RosterWeek {
	// Convert the start date to local time then use it to generate days.
	localDate := startDate.In(time.Local)
	dayNames := []string{"Tues", "Wed", "Thurs", "Fri", "Sat", "Sun", "Mon"}
	var days []*models.RosterDay

	for i, name := range dayNames {
		colour := "#ffffff"
		if i%2 == 0 {
			colour = "#b7b7b7"
		}
		day := &models.RosterDay{
			ID:      uuid.New(),
			DayName: name,
			Rows:    []*models.Row{newRow(), newRow(), newRow(), newRow()},
			Colour:  colour,
			Offset:  i,
		}
		days = append(days, day)
	}

	return models.RosterWeek{
		ID:        uuid.New(),
		StartDate: localDate,
		Days:      days,
		IsLive:    false,
	}
}

func newRow() *models.Row {
	return &models.Row{
		ID:     uuid.New(),
		Amelia: newSlot(),
		Early:  newSlot(),
		Mid:    newSlot(),
		Late:   newSlot(),
	}
}

func newSlot() models.Slot {
	return models.Slot{
		ID:        uuid.New(),
		StartTime: "",
		// AssignedStaff remains nil until set.
	}
}
