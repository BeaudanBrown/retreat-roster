package migrate

import (
	"log"
	"roster/cmd/db"
	"roster/cmd/server"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type OldData struct {
	StartDate          time.Time          `json:"startDate"`
	TimesheetStartDate time.Time          `json:"timesheetStartDate"`
	Staff              *[]*db.StaffMember `json:"staff"`
	Days               []*db.RosterDay    `json:"days"`
	IsLive             bool               `json:"isLive"`
	HideByIdeal        bool               `json:"hideByIdeal"`
	HideByPrefs        bool               `json:"hideByPrefs"`
	HideByLeave        bool               `json:"hideByLeave"`
	ApprovalMode       bool               `json:"approvalMode"`
}

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

func MigrateV5(s *server.Server) error {
	version := LoadVersion(s)
	log.Printf("Version: %v", version.Version)
	if version == nil || (version.Version != 4) {
		log.Println("No v5 migration required")
		return nil
	}
	allWeeks := s.LoadAllRosterWeeks()
	for _, week := range allWeeks {
		// Migrate to using timezone
		week.StartDate = week.StartDate.Add(time.Hour * -10)
	}
	s.SaveAllRosterWeeks(allWeeks)
	version.Version = 5
	SaveVersion(s, *version)
	log.Println("V5 migration complete")
	return nil
}

func MigrateV4(s *server.Server) error {
	version := LoadVersion(s)
	log.Printf("Version: %v", version.Version)
	if version == nil || (version.Version != 3) {
		log.Println("No v4 migration required")
		return nil
	}
	allEntries := s.GetAllTimesheetEntries()
	if allEntries == nil {
		log.Println("No entries to migrate")
		return nil
	}
	for _, entry := range *allEntries {
		// Migrate to using timezone
		entry.ShiftStart = entry.ShiftStart.Add(time.Hour * -10)
		entry.ShiftEnd = entry.ShiftEnd.Add(time.Hour * -10)
		entry.BreakStart = entry.BreakStart.Add(time.Hour * -10)
		entry.BreakEnd = entry.BreakEnd.Add(time.Hour * -10)
	}
	s.SaveAllTimesheetEntries(*allEntries)
	version.Version = 4
	SaveVersion(s, *version)
	log.Println("V4 migration complete")
	return nil
}

func MigrateV3(s *server.Server) error {
	version := LoadVersion(s)
	log.Printf("Version: %v", version.Version)
	if version == nil || (version.Version != 2 && version.Version != 0) {
		log.Println("No v3 migration required")
		return nil
	}
	allEntries := s.GetAllTimesheetEntries()
	if allEntries == nil {
		log.Println("No entries to migrate")
		return nil
	}
	for _, entry := range *allEntries {
		log.Printf("Shift before: %v", entry.ShiftType)
		if entry.ShiftType != db.Bar {
			entry.ShiftType = entry.ShiftType - 1
			log.Printf("Shift after: %v", entry.ShiftType)
		}
	}
	s.SaveAllTimesheetEntries(*allEntries)
	version.Version = 3
	SaveVersion(s, *version)
	log.Println("V3 migration complete")
	return nil
}

func MigrateV2(s *server.Server) error {
	version := LoadVersion(s)
	if version == nil || version.Version != 1 {
		log.Println("No v2 migration required")
		return nil
	}
	allEntries := s.GetAllTimesheetEntries()
	if allEntries == nil {
		log.Println("No entries to migrate")
		return nil
	}
	for _, entry := range *allEntries {
		log.Printf("Shift before: %v", entry.ShiftType)
		if entry.ShiftType != db.Bar {
			entry.ShiftType = entry.ShiftType + 1
			log.Printf("Shift after: %v", entry.ShiftType)
		}
	}
	s.SaveAllTimesheetEntries(*allEntries)
	version.Version = version.Version + 1
	SaveVersion(s, *version)
	log.Println("V2 migration complete")
	return nil
}
