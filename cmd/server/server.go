package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"time"

	"roster/cmd/db"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/mongo"
)

const SESSION_KEY = "sessionToken"
const DEV_MODE = false

type Server struct {
	CacheBust string
	Templates *template.Template
	db.Database
}

func (s *Server) renderTemplate(w http.ResponseWriter, templateName string, data interface{}) {
	err := s.Templates.ExecuteTemplate(w, templateName, data)
	if err != nil {
		log.Printf("Error executing template: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (s *Server) VerifyAdmin(handler http.HandlerFunc) http.HandlerFunc {
	return s.VerifySession(func(w http.ResponseWriter, r *http.Request) {
		staff := s.GetSessionUser(w, r)
		if staff == nil || !staff.IsAdmin {
			http.Redirect(w, r, "/profile", http.StatusSeeOther)
			return
		}
		handler(w, r)
	})
}

func (s *Server) VerifySession(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("Verify")
		cookie, err := r.Cookie("session_token")
		if err != nil {
			if err == http.ErrNoCookie {
				http.Redirect(w, r, "/landing", http.StatusSeeOther)
			} else {
				http.Error(w, "Bad Request", http.StatusBadRequest)
			}
			return
		}

		sessionTokenStr := cookie.Value

		sessionToken, err := uuid.Parse(sessionTokenStr)
		if err != nil {
			http.Redirect(w, r, "/landing", http.StatusSeeOther)
			return
		}

		if !s.isValidSession(sessionToken) {
			http.Redirect(w, r, "/landing", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), SESSION_KEY, sessionToken)
		reqWithToken := r.WithContext(ctx)
		handler(w, reqWithToken)
	}
}

func (s *Server) isValidSession(token uuid.UUID) bool {
	return s.GetStaffByToken(token) != nil
}

func ReadAndUnmarshal(w http.ResponseWriter, r *http.Request, reqBody interface{}) error {
	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return err
	}
	defer r.Body.Close()

	err = json.Unmarshal(bytes, reqBody)
	if err != nil {
		log.Printf("json: %v", string(bytes))
		log.Printf("Error parsing json: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return err
	}

	return nil
}

func LoadServerState(d *mongo.Database, context context.Context) (*Server, error) {
	var serverState Server
	var err error
	serverState = Server{
		CacheBust: fmt.Sprintf("%v", time.Now().UnixNano()),
		Templates: template.New("").Funcs(template.FuncMap{
			"MakeHeaderStruct":         MakeHeaderStruct,
			"MakeDayStruct":            MakeDayStruct,
			"GetHighlightCol":          db.GetHighlightCol,
			"MakeProfileStruct":        MakeProfileStruct,
			"MemberIsAssigned":         MemberIsAssigned,
			"MakeTimesheetEntryStruct": MakeTimesheetEntryStruct,
			"ShiftTypeToString":        db.ShiftTypeToString,
			"addDays": func(t time.Time, days int) time.Time {
				return t.AddDate(0, 0, days)
			},
		}),
		Database: db.Database{
			DB:      d,
			Context: context,
		},
	}
	serverState.Templates, err = serverState.Templates.ParseGlob("./www/*.html")
	if err != nil {
		return nil, err
	}
	return &serverState, nil
}

func (s *Server) GetSessionUser(w http.ResponseWriter, r *http.Request) *db.StaffMember {
	sessionToken, ok := r.Context().Value(SESSION_KEY).(uuid.UUID)
	if !ok {
		log.Printf("No session for using")
		return nil
	}
	staff := s.GetStaffByToken(sessionToken)
	if staff == nil {
		log.Printf("Error retrieving session user")
		return nil
	}
	return staff
}

func (s *Server) HandleLanding(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "landing", s.CacheBust)
}

type HeaderData struct {
	RosterLive bool
	IsAdmin    bool
}

func MakeHeaderStruct(isAdmin bool, rosterLive bool) HeaderData {
	return HeaderData{
		RosterLive: rosterLive,
		IsAdmin:    isAdmin,
	}
}
