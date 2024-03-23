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

type Highlight int

const (
	None Highlight = iota
	Duplicate
	PrefConflict
)

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
	Name   string
	Early   bool
	Mid   bool
	Late   bool
}

type ProfileData struct {
	StaffMember
	ShowSuccess bool
	ShowError bool
}

var emptyAvailability = []DayAvailability{
	{
		Name: "Tuesday",
	},
	{
		Name: "Wednesday",
	},
	{
		Name: "Thursday",
	},
	{
		Name: "Friday",
	},
	{
		Name: "Saturday",
	},
	{
		Name: "Sunday",
	},
	{
		Name: "Monday",
	},
}

type StaffMember struct {
	ID   uuid.UUID
	GoogleID   string
	FirstName string
	LastName string
	Email string
	Phone string
	Availability []DayAvailability
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
	Early  Slot
	Mid Slot
	Late   Slot
}

type Slot struct {
	ID            uuid.UUID
	StartTime     string
	AssignedStaff *uuid.UUID
	Flag	Highlight
}

type GoogleUserInfo struct {
	ID            string `json:"id"`
}

func SaveState(s *Server) error {
	s.CheckFlags()
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
		"GetHighlightCol": GetHighlightCol,
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
		Mid: newSlot(),
		Late:   newSlot(),
	}
}

func newSlot() Slot {
	return Slot{
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
			Availability: emptyAvailability,
		},
		{
			FirstName: "Jamie",
			ID:   uuid.New(),
			Availability: emptyAvailability,
		},
		{
			FirstName: "Kerryn",
			ID:   uuid.New(),
			Availability: emptyAvailability,
		},
		{
			FirstName: "James",
			ID:   uuid.New(),
			Availability: emptyAvailability,
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
	http.HandleFunc("/profile", s.VerifySession(s.HandleProfileIndex))
	http.HandleFunc("/profileBody", s.VerifySession(s.HandleProfile))
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

func GetHighlightCol(defaultCol string, flag Highlight) string {
	if flag == Duplicate {
		return "#FFA07A"
	}
	if flag == PrefConflict {
		return "#FF6666"
	}
	return defaultCol
}

func MakeDayStruct(day RosterDay, staff *[]*StaffMember) DayStruct {
	return DayStruct{
		day,
		staff,
	}
}

func (s *Server) HandleProfileIndex(w http.ResponseWriter, r *http.Request) {
	err := s.Templates.ExecuteTemplate(w, "profileIndex", s.CacheBust)
	if err != nil {
		log.Fatalf("Error executing template: %v", err)
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

	data := ProfileData{
		StaffMember: *staff,
		ShowSuccess: false,
		ShowError: false,
	}
	err := s.Templates.ExecuteTemplate(w, "profile", data)
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
				return &row.Early
			}
			if row.Mid.ID == slotID {
				return &row.Mid
			}
			if row.Late.ID == slotID {
				return &row.Late
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
	dayID, err := uuid.Parse(r.FormValue("dayID"))
	if err != nil {
		log.Printf("Invalid dayID: %v", err)
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
	slot := s.GetSlotByID(slotID)
	if slot == nil {
		log.Printf("Invalid slotID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	staffID, err := uuid.Parse(staffIDStr)
	if err != nil {
		slot.AssignedStaff = nil
	} else {
		log.Printf("Modify %v slot id: %v, staffid: %v", slotID, slotID, staffID)
		member := s.GetStaffByID(staffID)
		if member != nil {
			slot.AssignedStaff = &member.ID
		}
	}

	SaveState(s)

	day := s.GetDayByID(dayID)
	if day == nil {
		log.Printf("Error executing template: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err = s.Templates.ExecuteTemplate(w, "rosterDay", MakeDayStruct(*day, s.Staff))
	if err != nil {
		log.Printf("Error executing template: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
}

type ModifyRows struct {
	Action string `json:"action"`
	DayID  string `json:"dayID"`
}

type ModifyProfile struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email  string `json:"email"`
	Phone  string `json:"phone"`
	TuesEarly  string `json:"Tuesday-early-avail"`
	TuesMid  string `json:"Tuesday-mid-avail"`
	TuesLate  string `json:"Tuesday-late-avail"`
	WedEarly  string `json:"Wednesday-early-avail"`
	WedMid  string `json:"Wednesday-mid-avail"`
	WedLate  string `json:"Wednesday-late-avail"`
	ThursEarly  string `json:"Thursday-early-avail"`
	ThursMid  string `json:"Thursday-mid-avail"`
	ThursLate  string `json:"Thursday-late-avail"`
	FriEarly  string `json:"Friday-early-avail"`
	FriMid  string `json:"Friday-mid-avail"`
	FriLate  string `json:"Friday-late-avail"`
	SatEarly  string `json:"Saturday-early-avail"`
	SatMid  string `json:"Saturday-mid-avail"`
	SatLate  string `json:"Saturday-late-avail"`
	SunEarly  string `json:"Sunday-early-avail"`
	SunMid  string `json:"Sunday-mid-avail"`
	SunLate  string `json:"Sunday-late-avail"`
	MonEarly  string `json:"Monday-early-avail"`
	MonMid  string `json:"Monday-mid-avail"`
	MonLate  string `json:"Monday-late-avail"`
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
		log.Printf("json: %v", bytes)
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
			staff.Phone = reqBody.Phone

			staff.Availability = []DayAvailability{
				{
					Name: "Tuesday",
					Early: reqBody.TuesEarly == "on",
					Mid: reqBody.TuesMid == "on",
					Late: reqBody.TuesLate == "on",
				},
				{
					Name: "Wednesday",
					Early: reqBody.WedEarly == "on",
					Mid: reqBody.WedMid == "on",
					Late: reqBody.WedLate == "on",
				},
				{
					Name: "Thursday",
					Early: reqBody.ThursEarly == "on",
					Mid: reqBody.ThursMid == "on",
					Late: reqBody.ThursLate == "on",
				},
				{
					Name: "Friday",
					Early: reqBody.FriEarly == "on",
					Mid: reqBody.FriMid == "on",
					Late: reqBody.FriLate == "on",
				},
				{
					Name: "Saturday",
					Early: reqBody.SatEarly == "on",
					Mid: reqBody.SatMid == "on",
					Late: reqBody.SatLate == "on",
				},
				{
					Name: "Sunday",
					Early: reqBody.SunEarly == "on",
					Mid: reqBody.SunMid == "on",
					Late: reqBody.SunLate == "on",
				},
				{
					Name: "Monday",
					Early: reqBody.MonEarly == "on",
					Mid: reqBody.MonMid == "on",
					Late: reqBody.MonLate == "on",
				},
			}
		}
		data := ProfileData{
			StaffMember: *staff,
			ShowSuccess: true,
			ShowError: false,
		}
		SaveState(s)
		err = s.Templates.ExecuteTemplate(w, "profile", data)
		if err != nil {
			log.Printf("Error executing template: %v\n", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	log.Printf("Error executing template: %v\n", err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

func (s *Server) CheckFlags() {
	for _, day := range s.Days {
		// Create a new map for each day to track occurrences of staff IDs within that day
		staffIDOccurrences := make(map[uuid.UUID]int)

		for _, row := range day.Rows {
			if row.Early.AssignedStaff != nil {
				staffIDOccurrences[*row.Early.AssignedStaff]++
			}
			if row.Mid.AssignedStaff != nil {
				staffIDOccurrences[*row.Mid.AssignedStaff]++
			}
			if row.Late.AssignedStaff != nil {
				staffIDOccurrences[*row.Late.AssignedStaff]++
			}
		}

		for i, row := range day.Rows {
			row.Early.Flag = None
			row.Mid.Flag = None
			row.Late.Flag = None

			if row.Early.AssignedStaff != nil {
				if staffIDOccurrences[*row.Early.AssignedStaff] > 1 {
					row.Early.Flag = Duplicate
				} else {
					staff := s.GetStaffByID(*row.Early.AssignedStaff)
					if staff != nil {
						if !staff.Availability[i].Early {
							row.Early.Flag = PrefConflict
						}
					}
				}
			}

			if row.Mid.AssignedStaff != nil {
				if staffIDOccurrences[*row.Mid.AssignedStaff] > 1 {
					row.Mid.Flag = Duplicate
				} else {
					staff := s.GetStaffByID(*row.Mid.AssignedStaff)
					if staff != nil {
						if !staff.Availability[i].Mid {
							row.Mid.Flag = PrefConflict
						}
					}
				}
			}

			if row.Late.AssignedStaff != nil {
				if staffIDOccurrences[*row.Late.AssignedStaff] > 1 {
					row.Late.Flag = Duplicate
				} else {
					staff := s.GetStaffByID(*row.Late.AssignedStaff)
					if staff != nil {
						if !staff.Availability[i].Late {
							row.Late.Flag = PrefConflict
						}
					}
				}
			}
		}
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
			if reqBody.Action == "+" {
				s.Days[i].Rows = append(s.Days[i].Rows, newRow())
			} else {
				if len(s.Days[i].Rows) > 2 {
					s.Days[i].Rows = s.Days[i].Rows[:len(s.Days[i].Rows)-1]
				}
			}
			SaveState(s)
			err := s.Templates.ExecuteTemplate(w, "rosterDay", MakeDayStruct(*s.Days[i], s.Staff))
			if err != nil {
				log.Printf("Error executing template: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			break
		}
	}
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
	expirationTime := token.Expiry
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
			Availability: emptyAvailability,
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
