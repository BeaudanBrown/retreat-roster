package migrate

import (
  "encoding/json"
  "log"
  "os"
  "path/filepath"
  "roster/cmd/server"
)

const STAFF_STATE_FILE = "./data/staff.json"
const ROSTER_DIR = "./data/rosters/"
const MIGRATE_DIR = "./data-old/"

func MigrateToMongo(s *server.Server) (error) {
  err := MigrateAllRosterWeeks(s)
  if err != nil {
    return err
  }
  err = MigrateStaffState(s)
  if err != nil {
    return err
  }
  MoveOldState()
  return nil
}

func MigrateAllRosterWeeks(s *server.Server) (error) {
  if _, err := os.Stat(ROSTER_DIR); err != nil {
    log.Println("No roster weeks to migrate")
    return nil
  }
  err := filepath.Walk(ROSTER_DIR, func(filename string, info os.FileInfo, err error) error {
    if err != nil {
      return err
    }
    if info.IsDir() {
      return nil
    }
    rosterWeek := LoadRosterWeek(filename)
    if rosterWeek == nil {
      return nil
    }
    err = s.SaveRosterWeek(*rosterWeek)
    return err
  })
  return err
}

func LoadRosterWeek(filename string) *server.RosterWeek {
  var rosterWeek server.RosterWeek
  if _, err := os.Stat(filename); err != nil {
    log.Println("No file")
    return nil
  }
  data, err := os.ReadFile(filename)
  if err != nil {
    log.Println("No read file")
    return nil
  }
  if err := json.Unmarshal(data, &rosterWeek); err != nil {
    log.Println("No json file")
    return nil
  }
  log.Println("Loaded rosterWeek")
  return &rosterWeek
}

func MigrateStaffState(s *server.Server) (error) {
  staffState := LoadStaffState()
  if staffState == nil {
    return nil
  }
  for _, staffMember := range *staffState.Staff {
    err := s.SaveStaffMember(*staffMember)
    if err != nil {
      return err
    }
  }
  return nil
}

func LoadStaffState() *server.StaffState {
  var staffState server.StaffState

  if _, err := os.Stat(STAFF_STATE_FILE); err != nil {
    log.Println("No staff to migrate")
    return nil
  }

  // Read the file
  data, err := os.ReadFile(STAFF_STATE_FILE)
  if err != nil {
    log.Println(err)
    return nil
  }

  // Unmarshal the JSON data
  if err := json.Unmarshal(data, &staffState); err != nil {
    log.Println(err)
    return nil
  }

  return &staffState
}

func MakeMigrateDir() {
  err := os.Mkdir(MIGRATE_DIR, 0755)
  if err != nil {
    if os.IsExist(err) {
      return
    } else {
      log.Fatalf("Failed to create directory: %v", err)
    }
  }
}

func MoveOldState() {
  if _, err := os.Stat(STAFF_STATE_FILE); err == nil {
    MakeMigrateDir()
    err := os.Rename(STAFF_STATE_FILE, MIGRATE_DIR + "state.json")
    if err != nil {
      log.Fatalf("Failed to move old staffState: %v", err)
    }
    log.Println("Successfully migrated staff state")
  }
  if _, err := os.Stat(ROSTER_DIR); err == nil {
    MakeMigrateDir()
    err := os.Rename(ROSTER_DIR, MIGRATE_DIR + "rosters")
    if err != nil {
      log.Fatalf("Failed to move old rosters: %v", err)
    }
    log.Println("Successfully migrated rosters")
  }
}
