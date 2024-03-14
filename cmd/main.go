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
	"context"

	"github.com/joho/godotenv"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const STATE_FILE = "./state.json"
const SESSION_KEY = "sessionToken"

func googleOauthConfig() *oauth2.Config {
    return &oauth2.Config{
        ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
        ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
        RedirectURL:  "http://localhost:6969/auth/callback",
        Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email"},
        Endpoint:     google.Endpoint,
    }
}

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
	GoogleID   string
	Name string
	Token *uuid.UUID
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

type GoogleUserInfo struct {
	ID            string `json:"id"`
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
		StartTime:     "",
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
	if err := godotenv.Load(); err != nil {
		log.Printf("No .env file found")
	}
	s, err := LoadState(STATE_FILE)
	if err != nil {
		log.Fatalf("Error loading state: %v", err)
	}
	http.HandleFunc("/", s.VerifySession(s.HandleIndex))

	http.HandleFunc("/app.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./www/app.css")
	})
	http.HandleFunc("/root", s.VerifySession(s.HandleRoot))
	http.HandleFunc("/auth/login", s.handleGoogleLogin)
	http.HandleFunc("/auth/logout", s.handleGoogleLogout)
	http.HandleFunc("/auth/callback", s.handleGoogleCallback)

	http.HandleFunc("/modifyRows", s.VerifySession(s.HandleModifyRows))
	http.HandleFunc("/modifySlot", s.VerifySession(s.HandleModifySlot))
	http.HandleFunc("/modifyTimeSlot", s.VerifySession(s.HandleModifyTimeSlot))

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
	case "/auth/login":
		s.handleGoogleLogin(w, r)
	case "/auth/callback":
		s.handleGoogleCallback(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
	err := s.Templates.ExecuteTemplate(w, "index", s.CacheBust)
	if err != nil {
		log.Fatalf("Error executing template: %v", err)
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

func (s *Server) GetStaffByToken(token uuid.UUID) *StaffMember {
	for i := range s.Staff {
		if s.Staff[i].Token == &token {
			return s.Staff[i]
		}
	}
	return nil
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
	if member == nil {
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

func (s *Server) handleGoogleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite:  http.SameSiteLaxMode,
	})

	sessionToken, ok := r.Context().Value(SESSION_KEY).(uuid.UUID)
	if ok {
		staff := s.GetStaffByToken(sessionToken)
		if staff != nil {
			staff.Token = nil
		}
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	url := googleOauthConfig().AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.SetAuthURLParam("prompt", "select_account"))
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (s *Server) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := r.URL.Query().Get("code")
	token, err := googleOauthConfig().Exchange(ctx, code)
	if err != nil {
		http.Error(w, "Failed to exchange token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token="+token.AccessToken)
	if err != nil {
		http.Error(w, "Failed to login: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var userInfo GoogleUserInfo
	if err = json.NewDecoder(response.Body).Decode(&userInfo); err != nil {
		http.Error(w, "Error decoding user information: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sessionIdentifier := uuid.New()
	expirationTime := token.Expiry // The token's expiration time can be used, or set your own
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    sessionIdentifier.String(),
		Expires:  expirationTime,
		HttpOnly: true,
		Secure:   true,
		SameSite:  http.SameSiteLaxMode,
		Path:     "/",
	})

	found := false
	for i := range s.Staff {
		if s.Staff[i].GoogleID == userInfo.ID {
			found = true
			s.Staff[i].Token = &sessionIdentifier
		}
	}

	if !found {
		s.Staff = append(s.Staff, &StaffMember{
			ID:    uuid.New(),
			GoogleID:    userInfo.ID,
			Name:  "",
			Token: &sessionIdentifier,
		})
	}

	SaveState(s)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) VerifySession(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err != nil {
			if err == http.ErrNoCookie {
				http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			} else {
				http.Error(w, "Bad Request", http.StatusBadRequest)
			}
			return
		}

		sessionTokenStr := cookie.Value

		// Convert sessionTokenStr (type string) to type uuid.UUID
		sessionToken, err := uuid.Parse(sessionTokenStr)
		if err != nil {
				http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
				return
		}

		if !s.isValidSession(sessionToken) {
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), SESSION_KEY, sessionToken)
		reqWithToken := r.WithContext(ctx)
		handler(w, reqWithToken)
	}
}

func (s *Server) isValidSession(token uuid.UUID) bool {
	for i := range s.Staff {
		if *s.Staff[i].Token == token {
			return true
		}
	}
	return false
}
