package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"math"
	"net/http"
	"time"

	"roster/cmd/db"
	"roster/cmd/models"
	"roster/cmd/repository"
	"roster/cmd/utils"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/mongo"
)

const SESSION_KEY = "sessionToken"
const DEV_MODE = false

type Server struct {
	CacheBust string
	Templates *template.Template
	db.Database
	Repos Repositories
}

type Repositories struct {
	Staff      repository.StaffRepository
	RosterWeek repository.RosterWeekRepository
	Timesheet  repository.TimesheetRepository
}

func (s *Server) renderTemplate(w http.ResponseWriter, templateName string, data any) {
	err := s.Templates.ExecuteTemplate(w, templateName, data)
	if err != nil {
		utils.PrintError(err, "Error executing template")
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

func GetTokenFromCookies(r *http.Request) *uuid.UUID {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		utils.PrintError(err, "Error getting session_token")
		return nil
	}
	sessionTokenStr := cookie.Value

	sessionToken, err := uuid.Parse(sessionTokenStr)
	if err != nil {
		utils.PrintError(err, "Error parsing session token")
		return nil
	}
	utils.PrintLog("Retrieved token: %v", sessionToken)
	return &sessionToken
}

func (s *Server) VerifySession(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionToken := GetTokenFromCookies(r)
		if sessionToken == nil {
			http.Redirect(w, r, "/landing", http.StatusSeeOther)
			return
		}

		staffMember, err := s.Repos.Staff.GetStaffByToken(*sessionToken)
		if err != nil {
			utils.PrintError(err, "Invalid session")
			http.Redirect(w, r, "/landing", http.StatusSeeOther)
			return
		}
		if staffMember.FirstName == "" && r.URL.String() != "/newAccount" && r.URL.String() != "/createAccount" {
			utils.PrintLog("Account not created yet")
			http.Redirect(w, r, "/newAccount", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), SESSION_KEY, *sessionToken)
		reqWithToken := r.WithContext(ctx)
		handler(w, reqWithToken)
	}
}

func ReadAndUnmarshal(w http.ResponseWriter, r *http.Request, reqBody any) error {
	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		utils.PrintError(err, "Error reading body")
		w.WriteHeader(http.StatusBadRequest)
		return err
	}
	defer r.Body.Close()

	err = json.Unmarshal(bytes, reqBody)
	if err != nil {
		utils.PrintError(err, "Error parsing json")
		w.WriteHeader(http.StatusBadRequest)
		return err
	}

	return nil
}

func LoadServerState(d *mongo.Database, context context.Context) (*Server, error) {
	var serverState Server
	var err error
	staffRepo := repository.NewMongoStaffRepository(context, d)
	rosterWeekRepo := repository.NewMongoRosterWeekRepository(context, d)
	timesheetRepo := repository.NewMongoTimesheetRepository(context, d)
	serverState = Server{
		CacheBust: fmt.Sprintf("%v", time.Now().UnixNano()),
		Templates: template.New("").Funcs(template.FuncMap{
			"MakeHeaderStruct":         MakeHeaderStruct,
			"MakeDayStruct":            MakeDayStruct,
			"GetHighlightCol":          models.GetHighlightCol,
			"MakeProfileStruct":        MakeProfileStruct,
			"MemberIsAssigned":         MemberIsAssigned,
			"MakeTimesheetEntryStruct": MakeTimesheetEntryStruct,
			"GetSortedLeaveReqs":       GetSortedLeaveReqs,
			"GetAllShiftTypes":         models.GetAllShiftTypes,
			"DisableTimesheet":         models.DisableTimesheet,
			"addDays": func(t time.Time, days int) time.Time {
				return t.AddDate(0, 0, days)
			},
			"roundFloat": func(val float64) float64 {
				return math.Round(val*100) / 100
			},
			"IsKichenShift": func(shiftType models.ShiftType) bool {
				return shiftType == models.Kitchen
			},
			"AdminShiftTypeStart": func() models.ShiftType {
				return models.DayManager
			},
			"MakePickerStruct":   MakePickerStruct,
			"MakeLeaveReqStruct": MakeLeaveReqStruct,
			"GetTimesheetTimes": func(date time.Time) [][]string {
				var intervals [][]string
				start := time.Date(date.Year(), date.Month(), date.Day(), 8, 0, 0, 0, time.Local)
				end := time.Date(date.Year(), date.Month(), date.Day()+1, 5, 45, 0, 0, time.Local)
				current := start

				for !current.After(end) {
					hour := current.Format("3")
					minute := current.Format("04")
					period := current.Format("pm")

					display := fmt.Sprintf("%s-%02s-%s", hour, minute, period)
					readable := current.Format("3:04 PM")
					full := current.Format("2006-01-02T15:04:05-07:00")

					intervals = append(intervals, []string{display, readable, full})
					current = current.Add(15 * time.Minute)
				}

				return intervals
			},
		}),
		Database: db.Database{
			DB:      d,
			Context: context,
		},
		Repos: Repositories{
			Staff:      staffRepo,
			RosterWeek: rosterWeekRepo,
			Timesheet:  timesheetRepo,
		},
	}
	serverState.Templates, err = serverState.Templates.ParseGlob("./www/*.html")
	if err != nil {
		return nil, err
	}
	return &serverState, nil
}

func (s *Server) GetSessionUser(w http.ResponseWriter, r *http.Request) *models.StaffMember {
	sessionToken, ok := r.Context().Value(SESSION_KEY).(uuid.UUID)
	if !ok {
		utils.PrintLog("No session for user")
		return nil
	}
	staff, err := s.Repos.Staff.GetStaffByToken(sessionToken)
	if err != nil {
		utils.PrintError(err, "Error retrieving session user")
		return nil
	}
	refreshedStaff, err := s.Repos.Staff.RefreshStaffConfig(*staff)
	if err != nil {
		utils.PrintError(err, "Error refreshing staff config")
		return nil
	}
	return &refreshedStaff
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
