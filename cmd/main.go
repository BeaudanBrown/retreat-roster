package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"roster/cmd/migrate"
	"roster/cmd/server"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("No .env file found")
	}
	ctx := context.TODO()

	db_host := os.Getenv("DB_HOST")
	db_user := os.Getenv("DB_USER")
	db_pass := os.Getenv("DB_PASS")
	db_port := os.Getenv("DB_PORT")
	uri := fmt.Sprintf("mongodb://%s:%s@%s:%s", db_user, db_pass, db_host, db_port)
	clientOptions := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}
	db := client.Database("mongodb")
	defer client.Disconnect(ctx)

	s, err := server.LoadServerState(db, ctx)
	if err != nil {
		log.Fatalf("Error loading server state: %v", err)
	}

	err = migrate.MigrateToMongo(s)
	if err != nil {
		log.Printf("Error migrating old data: %v", err)
	}

	err = migrate.MigrateV2(s)
	if err != nil {
		log.Printf("Error migrating to V2: %v", err)
	}

	http.HandleFunc("/", s.VerifySession(s.HandleIndex))
	http.HandleFunc("/landing", s.HandleLanding)

	http.HandleFunc("/app.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./www/app.css")
	})
	http.HandleFunc("/timesheets", s.VerifySession(s.HandleTimesheet))
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
	http.HandleFunc("/getTimesheetEditModal", s.VerifySession(s.HandleGetTimesheetEditModal))
	http.HandleFunc("/toggleHideApproved", s.VerifyAdmin(s.HandleToggleHideApproved))
	http.HandleFunc("/toggleShowAll", s.VerifyAdmin(s.HandleToggleShowAll))
	http.HandleFunc("/importRosterWeek", s.VerifyAdmin(s.HandleImportRosterWeek))
	http.HandleFunc("/exportWageReport", s.VerifyAdmin(s.HandleExportWageReport))
	http.HandleFunc("/exportEvanReport", s.VerifyAdmin(s.HandleExportEvanReport))
	http.HandleFunc("/toggleApproved", s.VerifyAdmin(s.HandleToggleApproved))

	log.Println(http.ListenAndServe(":6969", nil))
}
