package migrate

import (
	"encoding/json"
	"log"
	"os"
	"roster/cmd/server"
	"time"

	"github.com/google/uuid"
)

type OldData struct {
  StartDate time.Time `json:"startDate"`
  TimesheetStartDate time.Time `json:"timesheetStartDate"`
  Staff *[]*server.StaffMember `json:"staff"`
  Days  []*server.RosterDay   `json:"days"`
  IsLive bool `json:"isLive"`
  HideByIdeal bool `json:"hideByIdeal"`
  HideByPrefs bool `json:"hideByPrefs"`
  HideByLeave bool `json:"hideByLeave"`
  ApprovalMode bool `json:"approvalMode"`
}

const STATE_FILE = "./data/state.json"

func MigrateToMongo(s *server.Server) (error) {
  var state OldData
  if _, err := os.Stat(STATE_FILE); err != nil {
    log.Println("No state to migrate")
    return nil
  }
  data, err := os.ReadFile(STATE_FILE)
  if err != nil {
    log.Printf("Couldn't read state file: %v", err)
    return nil
  }
  if err = json.Unmarshal(data, &state); err != nil {
    log.Printf("Couldn't unmarshal state file: %v", err)
    return nil
  }
  for _, staffMember := range *state.Staff {
    staffMember.Config = server.StaffConfig{
      TimesheetStartDate: server.GetLastTuesday(),
      RosterStartDate: server.GetNextTuesday(),
    }
    err = s.SaveStaffMember(*staffMember)
    if err != nil {
      log.Printf("Error while migrating staff: %v", err)
      return err
    }
  }
  year, month, day := state.StartDate.Date()
  startDate := time.Date(year, month, day, 0, 0, 0, 0, time.Now().Location())
  rosterWeek := server.RosterWeek{
    ID: uuid.New(),
    StartDate: startDate,
    Days: state.Days,
    IsLive: state.IsLive,
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
