package main

import (
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"

	"github.com/google/uuid"
)

func main() {
	templates := template.Must(template.ParseGlob("./www/*.html"))
	s := &Server{
		templates: templates,
		Days: []RosterDay{
			{
				ID:      uuid.New(),
				DayName: "Tuesday",
				Rows: []Row{
					{
						Early: Slot{
							Name: "guy",
						},
					},
				},
			},
			{
				ID:      uuid.New(),
				DayName: "Wednesday",
				Rows: []Row{
					{
						Early: Slot{
							Name: "gurl",
						},
					},
				},
			},
			{
				ID:      uuid.New(),
				DayName: "Thursday",
				Rows: []Row{
					{
						Early: Slot{
							Name: "gerl",
						},
					},
				},
			},
		},
	}
	http.HandleFunc("/", s.Handle)
	http.ListenAndServe(":6969", nil)
}

type RosterDay struct {
	ID      uuid.UUID
	DayName string
	Rows    []Row
}

type Row struct {
	Early  Slot
	Middle Slot
	Late   Slot
}

type Slot struct {
	Name string
}

type Server struct {
	templates *template.Template
	Days      []RosterDay
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
		http.ServeFile(w, r, "./www/index.html")
	case "/root":
		s.HandleRoot(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) HandleRoot(w http.ResponseWriter, r *http.Request) {
	// rosterTmp, err := template.ParseFiles("www/root.html")
	err := s.templates.ExecuteTemplate(w, "root.html", s.Days)
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
				s.Days[i].Rows = append(s.Days[i].Rows, Row{
					Early: Slot{
						Name: "guy2",
					},
				})
			} else {
				if len(s.Days[i].Rows) >= 2 {
					s.Days[i].Rows = s.Days[i].Rows[:len(s.Days[i].Rows)-1]
				}
			}
			err := s.templates.ExecuteTemplate(w, "rosterDay", s.Days[i])
			if err != nil {
				log.Printf("Error executing template: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			break
		}
	}
}
