package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
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
	Staff *[]*StaffMember `json:"staff"`
}

type DayAvailability struct {
	Early   bool
	Mid   bool
	Late   bool
}

type WeekAvailability struct {
	Tuesday	DayAvailability
	Wednesday	DayAvailability
	Thursday	DayAvailability
	Friday	DayAvailability
	Saturday	DayAvailability
	Sunday	DayAvailability
	Monday	DayAvailability
}

type StaffMember struct {
	ID   uuid.UUID
	GoogleID   string
	FirstName string
	LastName string
	Email string
	Availability WeekAvailability
	Token *uuid.UUID
}

type RosterDay struct {
	ID             uuid.UUID
	DayName        string
	Rows           []*Row
	Date           time.Time
	Colour         string
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
	var state *Server
	var err error
	if _, err = os.Stat(filename); err != nil {
		state = newState()
		SaveState(state)
	} else {
		var data []byte
		if data, err = os.ReadFile(filename); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &state); err != nil {
			return nil, err
		}
	}

	log.Println("Loaded state")
	state.CacheBust = fmt.Sprintf("%v", time.Now().UnixNano())
	state.Templates = template.New("").Funcs(template.FuncMap{
			"MakeDayStruct": MakeDayStruct,
	})
	state.Templates, err = state.Templates.ParseGlob("./www/*.html")
	if err != nil {
			return nil, err
	}
	return state, nil
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
	if daysUntilTuesday <= 0 {
		daysUntilTuesday += 7
	}
	nextTuesday := today.AddDate(0, 0, daysUntilTuesday)

	var Days []*RosterDay

	staff := []*StaffMember{
		{
			FirstName: "Beaudan",
			ID:   uuid.New(),
		},
		{
			FirstName: "Jamie",
			ID:   uuid.New(),
		},
		{
			FirstName: "Kerryn",
			ID:   uuid.New(),
		},
		{
			FirstName: "James",
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
		})
	}

	s := &Server{
		CacheBust: fmt.Sprintf("%v", time.Now().UnixNano()),
		ServerDisc: ServerDisc{
			Days:  Days,
			Staff: &staff,
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
	http.HandleFunc("/profile", s.VerifySession(s.HandleProfile))
	http.HandleFunc("/auth/login", s.handleGoogleLogin)
	http.HandleFunc("/auth/logout", s.handleGoogleLogout)
	http.HandleFunc("/auth/callback", s.handleGoogleCallback)

	http.HandleFunc("/modifyProfile", s.VerifySession(s.HandleModifyProfile))
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

type DayStruct struct {
	RosterDay
	Staff *[]*StaffMember
}

func MakeDayStruct(day RosterDay, staff *[]*StaffMember) DayStruct {
	return DayStruct{
		day,
		staff,
	}
}

func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
	err := s.Templates.ExecuteTemplate(w, "index", s.CacheBust)
	if err != nil {
		log.Fatalf("Error executing template: %v", err)
	}
}

func (s *Server) HandleProfile(w http.ResponseWriter, r *http.Request) {
    sessionToken, ok := r.Context().Value(SESSION_KEY).(uuid.UUID)
    if !ok {
        log.Println("Invalid or missing session token")
        http.Redirect(w, r, "/login", http.StatusSeeOther)
        return
    }

    staff := s.GetStaffByToken(sessionToken)
    if staff == nil {
        log.Println("Couldn't find staff with given token")
        http.Redirect(w, r, "/login", http.StatusSeeOther)
        return
    }

    err := s.Templates.ExecuteTemplate(w, "profile", staff)
    if err != nil {
        log.Printf("Error executing template: %v\n", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
    }
}

func (s *Server) HandleRoot(w http.ResponseWriter, r *http.Request) {
	err := s.Templates.ExecuteTemplate(w, "root", s)
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
	for i := range *s.Staff {
		if (*s.Staff)[i].Token != nil && *(*s.Staff)[i].Token == token {
			return (*s.Staff)[i]
		}
	}
	return nil
}

func (s *Server) GetStaffByID(staffID uuid.UUID) *StaffMember {
	for i := range *s.Staff {
		if (*s.Staff)[i].ID == staffID {
			return (*s.Staff)[i]
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

type ModifyProfile struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email  string `json:"email"`
	TuesEarly  string `json:"tuesday-early-avail"`
	TuesMid  string `json:"tuesday-mid-avail"`
	TuesLate  string `json:"tuesday-late-avail"`
	WedEarly  string `json:"wednesday-early-avail"`
	WedMid  string `json:"wednesday-mid-avail"`
	WedLate  string `json:"wednesday-late-avail"`
	ThursEarly  string `json:"thursday-early-avail"`
	ThursMid  string `json:"thursday-mid-avail"`
	ThursLate  string `json:"thursday-late-avail"`
	FriEarly  string `json:"friday-early-avail"`
	FriMid  string `json:"friday-mid-avail"`
	FriLate  string `json:"friday-late-avail"`
	SatEarly  string `json:"saturday-early-avail"`
	SatMid  string `json:"saturday-mid-avail"`
	SatLate  string `json:"saturday-late-avail"`
	SunEarly  string `json:"sunday-early-avail"`
	SunMid  string `json:"sunday-mid-avail"`
	SunLate  string `json:"sunday-late-avail"`
	MonEarly  string `json:"monday-early-avail"`
	MonMid  string `json:"monday-mid-avail"`
	MonLate  string `json:"monday-late-avail"`
}

func (s *Server) HandleModifyProfile(w http.ResponseWriter, r *http.Request) {
	log.Println("Modify profile")
	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var reqBody ModifyProfile
	err = json.Unmarshal(bytes, &reqBody)
	if err != nil {
		log.Printf("Error parsing json: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sessionToken, ok := r.Context().Value(SESSION_KEY).(uuid.UUID)
	if ok {
		staff := s.GetStaffByToken(sessionToken)
		if staff != nil {
			staff.FirstName = reqBody.FirstName
			staff.LastName = reqBody.LastName
			staff.Email = reqBody.Email

			staff.Availability.Tuesday.Early = reqBody.TuesEarly == "on"
			staff.Availability.Tuesday.Mid = reqBody.TuesMid == "on"
			staff.Availability.Tuesday.Late = reqBody.TuesLate == "on"

			staff.Availability.Wednesday.Early = reqBody.WedEarly == "on"
			staff.Availability.Wednesday.Mid = reqBody.WedMid == "on"
			staff.Availability.Wednesday.Late = reqBody.WedLate == "on"

			staff.Availability.Thursday.Early = reqBody.ThursEarly == "on"
			staff.Availability.Thursday.Mid = reqBody.ThursMid == "on"
			staff.Availability.Thursday.Late = reqBody.ThursLate == "on"

			staff.Availability.Friday.Early = reqBody.FriEarly == "on"
			staff.Availability.Friday.Mid = reqBody.FriMid == "on"
			staff.Availability.Friday.Late = reqBody.FriLate == "on"

			staff.Availability.Saturday.Early = reqBody.SatEarly == "on"
			staff.Availability.Saturday.Mid = reqBody.SatMid == "on"
			staff.Availability.Saturday.Late = reqBody.SatLate == "on"

			staff.Availability.Sunday.Early = reqBody.SunEarly == "on"
			staff.Availability.Sunday.Mid = reqBody.SunMid == "on"
			staff.Availability.Sunday.Late = reqBody.SunLate == "on"

			staff.Availability.Monday.Early = reqBody.MonEarly == "on"
			staff.Availability.Monday.Mid = reqBody.MonMid == "on"
			staff.Availability.Monday.Late = reqBody.MonLate == "on"
		}
		SaveState(s)
	}
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
			err := s.Templates.ExecuteTemplate(w, "rosterDay", MakeDayStruct(*s.Days[i], s.Staff))
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
	for i := range *s.Staff {
		if (*s.Staff)[i].GoogleID == userInfo.ID {
			found = true
			(*s.Staff)[i].Token = &sessionIdentifier
		}
	}

	if !found {
		new := (append(*s.Staff, &StaffMember{
			ID:    uuid.New(),
			GoogleID:    userInfo.ID,
			FirstName:  "",
			Token: &sessionIdentifier,
		}))
		s.Staff = &new
	}

	SaveState(s)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) VerifySession(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("Verify")
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
	for i := range *s.Staff {
		if (*s.Staff)[i].Token != nil && *(*s.Staff)[i].Token == token {
			return true
		}
	}
	return false
}
