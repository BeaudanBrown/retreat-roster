package migrate

import (
	"log"
	"roster/cmd/server"

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
			log.Printf("Error reading version: %v", err)
			return nil
		}
		log.Printf("Creating new version")
		version = Version{
			ID:      "version",
			Version: 1,
		}
	}
	if version.ID != "version" {
		// Fix up borked databases
		_, err := versionCollection.DeleteMany(s.Context, bson.M{})
		if err != nil {
			log.Printf("Error deleting versions: %v", err)
			return nil
		}
		log.Printf("Fixing old version")
		version = Version{
			ID:      "version",
			Version: 2,
		}
		_, err = versionCollection.InsertOne(s.Context, version)
		if err != nil {
			log.Printf("Error inserting new version: %v", err)
			return nil
		}
	}
	return &version
}

func SaveVersion(s *server.Server, v Version) error {
	log.Printf("saving version: %v", v.ID)
	collection := s.DB.Collection("version")
	filter := bson.M{"id": v.ID}
	update := bson.M{"$set": v}
	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(s.Context, filter, update, opts)
	if err != nil {
		log.Println("Failed to save version")
		return err
	}
	log.Println("Saved version")
	return nil
}
