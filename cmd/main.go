package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

const STATE_FILE = "./state.json"

type Server struct {
	CacheBust string
	Templates *template.Template
	ServerDisc
}

type ServerDisc struct {
	Days  []*RosterDay   `json:"days"`
	Staff []*StaffMember `json:"staff"`
}

type StaffMember struct {
	ID   uuid.UUID
	Name string
}

type RosterDayTmp struct {
	RosterDay
	Colour         string
	AvailableStaff []StaffMember
}

type RosterDay struct {
	ID             uuid.UUID
	DayName        string
	Rows           []*Row
	Date           time.Time
	Staff          []*StaffMember
	Colour         string
	AvailableStaff []*StaffMember
}

type Row struct {
	ID     uuid.UUID
	Early  *Slot
	Middle *Slot
	Late   *Slot
}

type Slot struct {
	ID            uuid.UUID
	StartTime     string
	AssignedStaff *uuid.UUID
}

func SaveState(s *Server) error {
	data, err := json.Marshal(s.ServerDisc)
	if err != nil {
		return err
	}
	log.Println("Saving state")
	if err := os.WriteFile(STATE_FILE, data, 0666); err != nil {
		return err
	}
	return nil
}

func LoadState(filename string) (*Server, error) {
	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			// The file does not exist, return a new state
			newState := newState()
			SaveState(newState)
			return newState, nil
		}
		return nil, err
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var state Server
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	log.Println("Loaded state")
	state.CacheBust = fmt.Sprintf("%v", time.Now().UnixNano())
	state.Templates = template.Must(template.ParseGlob("./www/*.html"))
	return &state, nil
}

func newRow() *Row {
	return &Row{
		ID:     uuid.New(),
		Early:  newSlot(),
		Middle: newSlot(),
		Late:   newSlot(),
	}
}

func newSlot() *Slot {
	return &Slot{
		ID:            uuid.New(),
		StartTime:     "12PM",
		AssignedStaff: nil,
	}
}

func newState() *Server {
	dayNames := []string{"Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday", "Monday"}
	today := time.Now()
	daysUntilTuesday := int(time.Tuesday - today.Weekday())
	if daysUntilTuesday <= 0 { // If today is Tuesday or beyond, add 7 to get to next Tuesday
		daysUntilTuesday += 7
	}
	nextTuesday := today.AddDate(0, 0, daysUntilTuesday)

	var Days []*RosterDay

	staff := []*StaffMember{
		{
			Name: "Beaudan",
			ID:   uuid.New(),
		},
		{
			Name: "Jamie",
			ID:   uuid.New(),
		},
		{
			Name: "Kerryn",
			ID:   uuid.New(),
		},
		{
			Name: "James",
			ID:   uuid.New(),
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
		Days = append(Days, &RosterDay{
			ID:      uuid.New(), // Generates a new UUID
			DayName: dayName,
			Rows: []*Row{
				newRow(),
				newRow(),
			},
			Date:           date,
			Colour:         colour,
			AvailableStaff: staff,
		})
	}

	templates := template.Must(template.ParseGlob("./www/*.html"))
	s := &Server{
		CacheBust: fmt.Sprintf("%v", time.Now().UnixNano()),
		Templates: templates,
		ServerDisc: ServerDisc{
			Days:  Days,
			Staff: staff,
		},
	}
	return s
}

func main() {
	s, err := LoadState(STATE_FILE)
	if err != nil {
		log.Fatalf("Error loading state: %v", err)
	}
	http.HandleFunc("/", s.Handle)
	log.Println(http.ListenAndServe(":6969", nil))
}

func (s *Slot) HasThisStaff(staffId uuid.UUID) bool {
	if s.AssignedStaff != nil && *s.AssignedStaff == staffId {
		return true
	}
	return false
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
	case "/modifySlot":
		s.HandleModifySlot(w, r)
	case "/modifyTimeSlot":
		s.HandleModifyTimeSlot(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) GetStaffByID(staffID uuid.UUID) *StaffMember {
	for i := range s.Staff {
		if s.Staff[i].ID == staffID {
			return s.Staff[i]
		}
	}
	return nil
}

func (s *Server) GetSlotByID(slotID uuid.UUID) *Slot {
	for i := range s.Days {
		day := s.Days[i]
		for j := range day.Rows {
			row := day.Rows[j]
			if row.Early.ID == slotID {
				return row.Early
			}
			if row.Middle.ID == slotID {
				return row.Middle
			}
			if row.Late.ID == slotID {
				return row.Late
			}
		}
	}
	return nil
}

func (s *Server) GetDayByID(dayID uuid.UUID) *RosterDay {
	for i := range s.Days {
		if s.Days[i].ID == dayID {
			return s.Days[i]
		}
	}
	return nil
}

func (s *Server) HandleModifyTimeSlot(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	slotIDStr := r.FormValue("slotID")
	timeVal := r.FormValue("timeVal")
	slotID, err := uuid.Parse(slotIDStr)
	if err != nil {
		log.Printf("Invalid SlotID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Printf("Modify %v timeslot id: %v", slotID, timeVal)
	slot := s.GetSlotByID(slotID)
	if slot == nil {
		log.Printf("Invalid slotID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	slot.StartTime = timeVal
	SaveState(s)
}

func (s *Server) HandleModifySlot(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	slotIDStr := r.FormValue("slotID")
	staffIDStr := r.FormValue("staffID")
	slotID, err := uuid.Parse(slotIDStr)
	if err != nil {
		log.Printf("Invalid SlotID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	staffID, err := uuid.Parse(staffIDStr)
	if err != nil {
		log.Printf("Invalid StaffID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Printf("Modify %v slot id: %v, staffid: %v", slotID, slotID, staffID)
	slot := s.GetSlotByID(slotID)
	if slot == nil {
		log.Printf("Invalid slotID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	member := s.GetStaffByID(staffID)
	if slot == nil {
		log.Printf("Invalid staffID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	slot.AssignedStaff = &member.ID
	SaveState(s)
}

type ModifyRows struct {
	Action string `json:"action"`
	DayID  string `json:"dayID"`
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

	dayID, err := uuid.Parse(reqBody.DayID)
	if err != nil {
		log.Printf("Invalid dayID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	for i := range s.Days {
		if s.Days[i].ID == dayID {
			// Found the day, modifying its value
			if reqBody.Action == "+" {
				s.Days[i].Rows = append(s.Days[i].Rows, newRow())
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
	SaveState(s)
}
