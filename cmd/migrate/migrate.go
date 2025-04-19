package migrate

import (
	"roster/cmd/server"
	"roster/cmd/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const STATE_FILE = "./data/state.json"

type Version struct {
	ID      string
	Version int
}

func LoadVersion(s *server.Server) *Version {
	versionCollection := s.DB.Collection("version")
	var version Version
	err := versionCollection.FindOne(s.Context, bson.M{"id": "version"}).Decode(&version)
	if err != nil {
		if err != mongo.ErrNoDocuments {
			utils.PrintError(err, "Error reading version")
			return nil
		}
		utils.PrintLog("Creating new version")
		version = Version{
			ID:      "version",
			Version: 1,
		}
	}
	if version.ID != "version" {
		// Fix up borked databases
		_, err := versionCollection.DeleteMany(s.Context, bson.M{})
		if err != nil {
			utils.PrintError(err, "Error deleting versions")
			return nil
		}
		utils.PrintLog("Fixing old version")
		version = Version{
			ID:      "version",
			Version: 2,
		}
		_, err = versionCollection.InsertOne(s.Context, version)
		if err != nil {
			utils.PrintError(err, "Error inserting new version")
			return nil
		}
	}
	return &version
}

func SaveVersion(s *server.Server, v Version) error {
	utils.PrintLog("Saving version: %v", v.ID)
	collection := s.DB.Collection("version")
	filter := bson.M{"id": v.ID}
	update := bson.M{"$set": v}
	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(s.Context, filter, update, opts)
	if err != nil {
		utils.PrintError(err, "Failed to save version")
		return err
	}
	utils.PrintLog("Saved version")
	return nil
}
