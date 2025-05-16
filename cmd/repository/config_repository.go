package repository

import (
	"context"

	"roster/cmd/models"
	"roster/cmd/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ConfigRepository defines persistence operations for server config.
type ConfigRepository interface {
	SaveVersion(v models.Version) error
	LoadVersion() (*models.Version, error)
}

// MongoConfigRepository implements ConfigRepository using MongoDB.
type MongoConfigRepository struct {
	collection *mongo.Collection
	ctx        context.Context
}

// NewMongoTimesheetRepository creates a new instance of MongoTimesheetRepository.
func NewMongoConfigRepository(ctx context.Context, db *mongo.Database) *MongoConfigRepository {
	return &MongoConfigRepository{
		collection: db.Collection("server"),
		ctx:        ctx,
	}
}

func (r *MongoConfigRepository) SaveVersion(v models.Version) error {
	filter := bson.M{"id": v.ID}
	update := bson.M{"$set": v}
	opts := options.Update().SetUpsert(true)
	_, err := r.collection.UpdateOne(r.ctx, filter, update, opts)
	if err != nil {
		utils.PrintError(err, "Failed to save version")
		return err
	}
	utils.PrintLog("Saved version")
	return nil
}

func (r *MongoConfigRepository) LoadVersion() (*models.Version, error) {
	var version models.Version
	err := r.collection.FindOne(r.ctx, bson.M{"id": "version"}).Decode(&version)

	if err != nil {
		if err != mongo.ErrNoDocuments {
			utils.PrintError(err, "Error reading version")
			return nil, err
		}
		utils.PrintLog("Creating new version")
		version = models.Version{
			ID:      "version",
			Version: 1,
		}
	}
	if version.ID != "version" {
		// Fix up borked databases
		_, err := r.collection.DeleteMany(r.ctx, bson.M{})
		if err != nil {
			utils.PrintError(err, "Error deleting versions")
			return nil, err
		}
		utils.PrintLog("Fixing old version")
		version = models.Version{
			ID:      "version",
			Version: 2,
		}
		_, err = r.collection.InsertOne(r.ctx, version)
		if err != nil {
			utils.PrintError(err, "Error inserting new version")
			return nil, err
		}
	}
	return &version, nil
}
