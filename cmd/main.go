package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type Server struct {
	CacheBust string
	Templates *template.Template
	Days      []RosterDay
	Staff     []StaffMember
}

type StaffMember struct {
	Name string
}

func main() {
	templates := template.Must(template.ParseGlob("./www/*.html"))
	dayNames := []string{"Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday", "Monday"}
	today := time.Now()
	daysUntilTuesday := int(time.Tuesday - today.Weekday())
	if daysUntilTuesday <= 0 { // If today is Tuesday or beyond, add 7 to get to next Tuesday
		daysUntilTuesday += 7
	}
	nextTuesday := today.AddDate(0, 0, daysUntilTuesday)

	var Days []RosterDay

	staff := []StaffMember{
		{
			Name: "Beaudan",
		},
		{
			Name: "Jamie",
		},
		{
			Name: "Kerryn",
		},
		{
			Name: "James",
		},
	}

	// Loop over dayNames to fill Days slice
	for i, dayName := range dayNames {
		date := nextTuesday.AddDate(0, 0, i)
		var colour string
		if i%2 == 0 {
			colour = "#b7b7b7"
		} else {
			colour = "#ffffff"
		}
		Days = append(Days, RosterDay{
			ID:      uuid.New(), // Generates a new UUID
			DayName: dayName,
			Rows: []Row{
				{},
				{},
			},
			Date:           date,
			Colour:         colour,
			AvailableStaff: staff,
		})
	}

	s := &Server{
		CacheBust: fmt.Sprintf("%v", time.Now().UnixNano()),
		Templates: templates,
		Days:      Days,
		Staff:     staff,
	}
	http.HandleFunc("/", s.Handle)
	log.Println(http.ListenAndServe(":6969", nil))
}

type RosterDayTmp struct {
	RosterDay
	Colour         string
	AvailableStaff []StaffMember
}

type RosterDay struct {
	ID             uuid.UUID
	DayName        string
	Rows           []Row
	Date           time.Time
	Staff          []StaffMember
	Colour         string
	AvailableStaff []StaffMember
}

type Row struct {
	Early  Slot
	Middle Slot
	Late   Slot
}

type Slot struct {
	StartTime time.Time
	Name      string
}

func (s *Server) Handle(w http.ResponseWriter, r *http.Request) {
	log.Printf("%v", r.URL.Path)

	switch r.Method {
	case "GET":
		s.HandleGetRequest(w, r)
	case "POST":
		s.HandlePostRequest(w, r)
	default:
	}
}

func (s *Server) HandleGetRequest(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		err := s.Templates.ExecuteTemplate(w, "index", s.CacheBust)
		if err != nil {
			log.Fatalf("Error executing template: %v", err)
		}
	case "/app.css":
		http.ServeFile(w, r, "./www/app.css")
	case "/root":
		s.HandleRoot(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) HandleRoot(w http.ResponseWriter, r *http.Request) {
	err := s.Templates.ExecuteTemplate(w, "root", s.Days)
	if err != nil {
		log.Fatalf("Error executing template: %v", err)
	}
}

func (s *Server) HandlePostRequest(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/modifyRows":
		s.HandleModifyRows(w, r)
	default:
		http.NotFound(w, r)
	}
}

type ModifyRows struct {
	Action string `json:"action"`
	Day    string `json:"day"`
}

func (s *Server) HandleModifyRows(w http.ResponseWriter, r *http.Request) {
	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var reqBody ModifyRows
	err = json.Unmarshal(bytes, &reqBody)
	if err != nil {
		log.Printf("Error parsing json: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	for i := range s.Days {
		if s.Days[i].DayName == reqBody.Day {
			// Found the day, modifying its value
			log.Printf("Found: %v", s.Days[i].DayName)
			if reqBody.Action == "+" {
				s.Days[i].Rows = append(s.Days[i].Rows, Row{})
			} else {
				if len(s.Days[i].Rows) > 2 {
					s.Days[i].Rows = s.Days[i].Rows[:len(s.Days[i].Rows)-1]
				}
			}
			err := s.Templates.ExecuteTemplate(w, "rosterDay", s.Days[i])
			if err != nil {
				log.Printf("Error executing template: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			break
		}
	}
}
