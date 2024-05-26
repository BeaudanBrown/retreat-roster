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

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const SESSION_KEY = "sessionToken"
const DEV_MODE = false

type Server struct {
  CacheBust string
  Templates *template.Template
  DB *mongo.Database
  Context context.Context
}

type StaffState struct {
  Staff *[]*StaffMember `bson:"staff"`
}

type RosterWeek struct {
  ID uuid.UUID  `bson:"_id"`
  StartDate  time.Time   `bson:"startDate"`
  Days  []*RosterDay   `bson:"days"`
  IsLive         bool `bson:"isLive"`
}

func (s StaffMember) MarshalBSON() ([]byte, error) {
    type Alias StaffMember
    aux := &struct {
        *Alias `bson:",inline"`
    }{
        Alias: (*Alias)(&s),
    }
    return bson.Marshal(aux)
}

func (s *StaffMember) UnmarshalBSON(data []byte) error {
    type Alias StaffMember
    aux := &struct {
        *Alias `bson:",inline"`
    }{
        Alias: (*Alias)(s),
    }

    if err := bson.Unmarshal(data, aux); err != nil {
        return err
    }

    s.Config.RosterStartDate = s.Config.RosterStartDate.In(time.Now().Location())
    s.Config.TimesheetStartDate = s.Config.TimesheetStartDate.In(time.Now().Location())

    return nil
}
func (rw RosterWeek) MarshalBSON() ([]byte, error) {
    type Alias RosterWeek
    return bson.Marshal(&struct {
        StartDate string `bson:"startDate"`
        *Alias `bson:",inline"`
    }{
        StartDate: rw.StartDate.Format("2006-01-02"),
        Alias:     (*Alias)(&rw),
    })
}

// UnmarshalBSON customizes the BSON unmarshaling for RosterWeek
func (rw *RosterWeek) UnmarshalBSON(data []byte) error {
    type Alias RosterWeek
    aux := &struct {
        StartDate string `bson:"startDate"`
        *Alias `bson:",inline"`
    }{
        Alias: (*Alias)(rw),
    }

    if err := bson.Unmarshal(data, aux); err != nil {
        return err
    }

    parsedDate, err := time.Parse("2006-01-02", aux.StartDate)
    parsedDate = parsedDate.In(time.Now().Location())
    log.Printf("Unmarshalled date: %v", parsedDate)
    if err != nil {
        return err
    }
    return nil
}

type Highlight int

const (
  None Highlight = iota
  Duplicate
  PrefConflict
  PrefRefuse
  LeaveConflict
)

func GetHighlightCol(defaultCol string, flag Highlight) string {
  if flag == Duplicate {
    return "#FFA07A"
  }
  if flag == PrefConflict {
    return "#FF9999"
  }
  if flag == LeaveConflict || flag == PrefRefuse {
    return "#CC3333"
  }
  return defaultCol
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
    if (staff == nil || !staff.IsAdmin) {
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

func (s *Server) SaveStaffMember (staffMember StaffMember) error {
  collection := s.DB.Collection("staff")
  filter := bson.M{"_id": staffMember.ID}
  update := bson.M{"$set": staffMember}
  opts := options.Update().SetUpsert(true)
  _, err := collection.UpdateOne(s.Context, filter, update, opts)
  if err != nil {
      log.Println("Failed to save staffMember")
      return err
  }
  log.Println("Saved staff member")
  return nil
}

func (s *Server) SaveRosterWeek (w RosterWeek) error {
  staffState := s.LoadStaffState()
  w = s.CheckFlags(staffState, w)
  d := w.StartDate.In(time.Now().Location())
  w.StartDate = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.Now().Location())
  log.Printf("Start Date: %v", w.StartDate)
  collection := s.DB.Collection("rosters")
  filter := bson.M{"_id": w.ID}
  update := bson.M{"$set": w}
  opts := options.Update().SetUpsert(true)
  _, err := collection.UpdateOne(s.Context, filter, update, opts)
  if err != nil {
      log.Println("Failed to save rosterWeek")
      return err
  }
  log.Println("Saved roster week")
  return nil
}

func (s *Server) LoadRosterWeek(startDate time.Time) *RosterWeek {
  var rosterWeek RosterWeek
  d := startDate.In(time.Now().Location())
  startDate = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.Now().Location())
  log.Printf("Loading week starting: %v", startDate)
  filter := bson.M{"startDate": startDate.Format("2006-01-02")}
  collection := s.DB.Collection("rosters")
  err := collection.FindOne(s.Context, filter).Decode(&rosterWeek)
  if err == nil {
    return &rosterWeek
  }

  if err != mongo.ErrNoDocuments {
    log.Printf("Error loading roster week: %v", err)
    return nil
  }

  // No document found, create a new RosterWeek
  log.Printf("Making new roster week")
  rosterWeek = newRosterWeek(startDate)
  if saveErr := s.SaveRosterWeek(rosterWeek); saveErr != nil {
    log.Printf("Error saving roster week: %v", saveErr)
    return nil
  }

  return &rosterWeek
}

func (s *Server) LoadStaffState() (StaffState) {
  collection := s.DB.Collection("staff")
  cursor, err := collection.Find(s.Context, bson.M{})
  if err != nil {
      log.Printf("Error executing query: %v", err)
  }
  defer cursor.Close(s.Context)

  newStaff := []*StaffMember{}

  for cursor.Next(s.Context) {
      var staffMember StaffMember
      if err := cursor.Decode(&staffMember); err != nil {
        fmt.Printf("Error loading staff state: %v", err)
      }
      newStaff = append(newStaff, &staffMember)
  }
  staffState := StaffState{
    &newStaff,
  }
  return staffState
}

func LoadServerState(db *mongo.Database, context context.Context) (*Server, error) {
  var serverState Server
  var err error
  serverState = Server{
    CacheBust: fmt.Sprintf("%v", time.Now().UnixNano()),
    Templates: template.New("").Funcs(template.FuncMap{
      "MakeHeaderStruct": MakeHeaderStruct,
      "MakeDayStruct": MakeDayStruct,
      "GetHighlightCol": GetHighlightCol,
      "MakeProfileStruct": MakeProfileStruct,
      "MemberIsAssigned": MemberIsAssigned,
      "MakeTimesheetEntryStruct": MakeTimesheetEntryStruct,
      "addDays": func(t time.Time, days int) time.Time {
        return t.AddDate(0, 0, days)
      },
    }),
    DB: db,
    Context: context,
  }
  serverState.Templates, err = serverState.Templates.ParseGlob("./www/*.html")
  if err != nil {
    return nil, err
  }
  return &serverState, nil
}

func newRow() *Row {
  return &Row{
    ID:     uuid.New(),
    Amelia:  newSlot(),
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

func newRosterWeek(startDate time.Time) RosterWeek {
  dayNames := []string{"Tues", "Wed", "Thurs", "Fri", "Sat", "Sun", "Mon"}
  var Days []*RosterDay

  for i, dayName := range dayNames {
    var colour string
    if i%2 == 0 {
      colour = "#b7b7b7"
    } else {
      colour = "#ffffff"
    }
    Days = append(Days, &RosterDay{
      ID:      uuid.New(),
      DayName: dayName,
      Rows: []*Row{
        newRow(),
        newRow(),
        newRow(),
        newRow(),
      },
      Colour:         colour,
      Offset:         i,
    })
  }
  week := RosterWeek{
    uuid.New(),
    startDate,
    Days,
    false,
  }
  return week
}

func NewStaffState() StaffState {
  staff := []*StaffMember{}
  s := StaffState{
    &staff,
  }
  return s
}

func (s *Server) GetStaffByGoogleID(googleID string) *StaffMember {
  collection := s.DB.Collection("staff")
  filter := bson.M{"googleid": googleID}
  var staffMember StaffMember
  err := collection.FindOne(s.Context, filter).Decode(&staffMember)
  if err != nil {
    if err == mongo.ErrNoDocuments {
      log.Printf("No staff with google id found: %v", err)
      return nil
    }
    log.Printf("Error getting staff by google id: %v", err)
    return nil
  }
  return &staffMember
}


func (s *Server) DeleteLeaveReqByID(staffMember StaffMember, leaveReqID uuid.UUID) {
  for i, leaveReq := range staffMember.LeaveRequests {
    if leaveReq.ID != leaveReqID {
      continue
    }
    staffMember.LeaveRequests = append(
      staffMember.LeaveRequests[:1],
      staffMember.LeaveRequests[i+1:]...)
      err := s.SaveStaffMember(staffMember)
      if err != nil {
        log.Printf("Error deleting leave request: %v", err)
      }
      return
  }
}

func (s *Server) GetStaffByLeaveReqID(leaveReqID uuid.UUID) *StaffMember {
  collection := s.DB.Collection("staff")
  var staffMember StaffMember
  filter := bson.M{
    "leaveRequests": bson.M{
      "$elemMatch": bson.M{
        "id": leaveReqID,
      },
    },
  }
  err := collection.FindOne(s.Context, filter).Decode(&staffMember)
  if err != nil {
      if err == mongo.ErrNoDocuments {
          return nil
      }
      return nil
  }
  return &staffMember
}

func (s *Server) GetStaffByID(staffID uuid.UUID) *StaffMember {
  collection := s.DB.Collection("staff")
  filter := bson.M{"id": staffID}
  var staffMember StaffMember
  err := collection.FindOne(s.Context, filter).Decode(&staffMember)
  if err != nil {
      if err == mongo.ErrNoDocuments {
          return nil
      }
      return nil
  }
  return &staffMember
}


func (s *Server) GetStaffByToken(token uuid.UUID) *StaffMember {
  collection := s.DB.Collection("staff")
  filter := bson.M{"token": token}
  var staffMember StaffMember
  err := collection.FindOne(s.Context, filter).Decode(&staffMember)
  if err != nil {
      if err == mongo.ErrNoDocuments {
          return nil
      }
      return nil
  }
  return &staffMember
}

func (s *Server) GetSessionUser(w http.ResponseWriter, r *http.Request) *StaffMember {
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

func (week *RosterWeek) GetSlotByID(slotID uuid.UUID) *Slot {
  for _, day := range week.Days {
    for j := range day.Rows {
      row := day.Rows[j]
      if row.Amelia.ID == slotID {
        return &row.Amelia
      }
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

func (week *RosterWeek) GetDayByID(dayID uuid.UUID) *RosterDay {
  for _, day := range week.Days {
    if day.ID == dayID {
      return day
    }
  }
  return nil
}

func (s *Server) HandleLanding(w http.ResponseWriter, r *http.Request) {
  s.renderTemplate(w, "landing", s.CacheBust)
}

type HeaderData struct {
  RosterLive  bool
  IsAdmin  bool
}

func MakeHeaderStruct(isAdmin bool, rosterLive bool) HeaderData {
  return HeaderData{
    RosterLive: rosterLive,
    IsAdmin: isAdmin,
  }
}

func (s *Server) GetStaffMap() map[uuid.UUID]StaffMember {
  staffMap := map[uuid.UUID]StaffMember{}
  staffState := s.LoadStaffState().Staff
  for _, staff := range *staffState {
    staffMap[staff.ID] = *staff
  }
  return staffMap
}
