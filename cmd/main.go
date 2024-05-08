package main

import (
  "log"
  "net/http"

  "github.com/joho/godotenv"
  "roster/cmd/server"
)

func main() {
  if err := godotenv.Load(); err != nil {
    log.Printf("No .env file found")
  }
  s, err := server.LoadState(server.STATE_FILE)
  if err != nil {
    log.Fatalf("Error loading state: %v", err)
  }
  http.HandleFunc("/", s.VerifySession(s.HandleIndex))
  http.HandleFunc("/landing", s.HandleLanding)

  http.HandleFunc("/app.css", func(w http.ResponseWriter, r *http.Request) {
    http.ServeFile(w, r, "./www/app.css")
  })
  http.HandleFunc("/timesheet", s.VerifySession(s.HandleTimesheet))
  http.HandleFunc("/root", s.VerifySession(s.HandleRoot))
  http.HandleFunc("/submitLeave", s.VerifySession(s.HandleSubmitLeave))
  http.HandleFunc("/profile", s.VerifySession(s.HandleProfileIndex))
  http.HandleFunc("/profileBody", s.VerifySession(s.HandleProfile))
  http.HandleFunc("/auth/login", s.HandleGoogleLogin)
  http.HandleFunc("/auth/logout", s.HandleGoogleLogout)
  http.HandleFunc("/auth/callback", s.HandleGoogleCallback)

  http.HandleFunc("/toggleHideByIdeal", s.VerifySession(s.HandleToggleHideByIdeal))
  http.HandleFunc("/toggleHideByPreferences", s.VerifySession(s.HandleToggleHideByPreferences))
  http.HandleFunc("/toggleHideByLeave", s.VerifySession(s.HandleToggleHideByLeave))

  http.HandleFunc("/toggleAdmin", s.VerifySession(s.HandleToggleAdmin))
  http.HandleFunc("/toggleHidden", s.VerifySession(s.HandleToggleHidden))
  http.HandleFunc("/toggleLive", s.VerifySession(s.HandleToggleLive))
  http.HandleFunc("/toggleAmelia", s.VerifySession(s.HandleToggleAmelia))
  http.HandleFunc("/toggleClosed", s.VerifySession(s.HandleToggleClosed))
  http.HandleFunc("/deleteAcc", s.VerifySession(s.HandleDeleteAccount))
  http.HandleFunc("/addTrial", s.VerifySession(s.HandleAddTrial))
  http.HandleFunc("/shiftWindow", s.VerifySession(s.HandleShiftWindow))
  http.HandleFunc("/modifyProfile", s.VerifySession(s.HandleModifyProfile))
  http.HandleFunc("/modifyRows", s.VerifySession(s.HandleModifyRows))
  http.HandleFunc("/modifySlot", s.VerifySession(s.HandleModifySlot))
  http.HandleFunc("/modifyTimeSlot", s.VerifySession(s.HandleModifyTimeSlot))
  http.HandleFunc("/modifyDescriptionSlot", s.VerifySession(s.HandleModifyDescriptionSlot))
  http.HandleFunc("/deleteLeaveReq", s.VerifySession(s.HandleDeleteLeaveReq))

  http.HandleFunc("/shiftTimesheetWindow", s.VerifySession(s.HandleShiftTimesheetWindow))
  http.HandleFunc("/addTimesheetEntry", s.VerifySession(s.HandleAddTimesheetEntry))
  http.HandleFunc("/deleteTimesheetEntry", s.VerifySession(s.HandleDeleteTimesheetEntry))
  http.HandleFunc("/modifyTimesheetEntry", s.VerifySession(s.HandleModifyTimesheetEntry))

  log.Println(http.ListenAndServe(":6969", nil))
}

// func CleanAndSortLeaveReqs(staff []*StaffMember) []LeaveRequest {
//   reqList := []LeaveRequest{}
//   today := time.Now()
//   for _, s := range staff {
//     changed := false
//     for i, r := range s.LeaveRequests {
//       if r.EndDate.Before(today) {
//         changed = true
//         s.LeaveRequests = append(s.LeaveRequests[:i], s.LeaveRequests[i+1:]...)
//       }
//     }
//     if changed {
//       SaveState(s)
//     }
//   }
//   return reqList
// }
