package migrate

import (
	"encoding/json"
	"log"
	"os"
	"roster/cmd/db"
	"roster/cmd/server"
	"roster/cmd/utils"
	"time"

	"github.com/google/uuid"
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

func MigrateToMongo(s *server.Server) error {
	var state OldData
	if _, err := os.Stat(STATE_FILE); err != nil {
		log.Println("No state to migrate")
		return err
	}
	data, err := os.ReadFile(STATE_FILE)
	if err != nil {
		log.Printf("Couldn't read state file: %v", err)
		return err
	}
	if err = json.Unmarshal(data, &state); err != nil {
		log.Printf("Couldn't unmarshal state file: %v", err)
		return err
	}
	for _, staffMember := range *state.Staff {
		staffMember.Config = db.StaffConfig{
			TimesheetStartDate: utils.GetLastTuesday(),
			RosterStartDate:    utils.GetNextTuesday(),
		}
		err = s.SaveStaffMember(*staffMember)
		if err != nil {
			log.Printf("Error while migrating staff: %v", err)
			return err
		}
	}
	year, month, day := state.StartDate.Date()
	startDate := time.Date(year, month, day, 0, 0, 0, 0, time.Now().Location())
	rosterWeek := db.RosterWeek{
		ID:        uuid.New(),
		StartDate: startDate,
		Days:      state.Days,
		IsLive:    state.IsLive,
	}
	err = s.SaveRosterWeek(rosterWeek)
	if err != nil {
		log.Printf("Error while migrating roster week: %v", err)
		return err
	}
	MoveOldState()
	return nil
}

func MoveOldState() {
	if _, err := os.Stat(STATE_FILE); err == nil {
		filename := "./data/state-" + time.Now().Format("01-02--15-04") + ".json"
		err := os.Rename(STATE_FILE, filename)
		if err != nil {
			log.Fatalf("Failed to move old staffState: %v", err)
		}
		log.Println("Successfully migrated staff state")
	}
}
