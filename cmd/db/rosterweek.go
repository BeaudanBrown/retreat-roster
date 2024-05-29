package db

import (
	"log"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

//TODO: Flatten rosterweeks into days or maybe even just rows

type RosterWeek struct {
	ID        uuid.UUID    `bson:"id"`
	StartDate time.Time    `bson:"startDate"`
	Days      []*RosterDay `bson:"days"`
	IsLive    bool         `bson:"isLive"`
}

type RosterDay struct {
	ID         uuid.UUID
	DayName    string
	Rows       []*Row
	Colour     string
	Offset     int
	IsClosed   bool
	AmeliaOpen bool
}

type Row struct {
	ID     uuid.UUID
	Amelia Slot
	Early  Slot
	Mid    Slot
	Late   Slot
}

type Slot struct {
	ID            uuid.UUID
	StartTime     string
	AssignedStaff *uuid.UUID
	StaffString   *string
	Flag          Highlight
	Description   string
}

type Highlight int

const (
	None Highlight = iota
	Duplicate
	PrefConflict
	PrefRefuse
	LeaveConflict
	IdealMet
	IdealExceeded
)

func (rw RosterWeek) MarshalBSON() ([]byte, error) {
	type Alias RosterWeek
	aux := &struct {
		*Alias `bson:",inline"`
	}{
		Alias: (*Alias)(&rw),
	}
	year, month, day := aux.StartDate.Date()
	aux.StartDate = time.Date(year, month, day, 0, 0, 0, 0, aux.StartDate.Location())
	return bson.Marshal(aux)
}

func (rw *RosterWeek) UnmarshalBSON(data []byte) error {
	type Alias RosterWeek
	aux := &struct {
		*Alias `bson:",inline"`
	}{
		Alias: (*Alias)(rw),
	}
	if err := bson.Unmarshal(data, aux); err != nil {
		return err
	}
	aux.StartDate = aux.StartDate.In(time.Now().Location())
	return nil
}

func (d *Database) SaveRosterWeek(w RosterWeek) error {
	allStaff := d.LoadAllStaff()
	w = d.CheckFlags(allStaff, w)
	collection := d.DB.Collection("rosters")
	filter := bson.M{"id": w.ID}
	update := bson.M{"$set": w}
	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(d.Context, filter, update, opts)
	if err != nil {
		log.Println("Failed to save rosterWeek")
		return err
	}
	log.Println("Saved roster week")
	return nil
}

func (d *Database) LoadRosterWeek(startDate time.Time) *RosterWeek {
	var rosterWeek RosterWeek
	log.Printf("Loading week starting: %v", startDate)
	filter := bson.M{"startDate": startDate}
	collection := d.DB.Collection("rosters")
	err := collection.FindOne(d.Context, filter).Decode(&rosterWeek)
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
	if saveErr := d.SaveRosterWeek(rosterWeek); saveErr != nil {
		log.Printf("Error saving roster week: %v", saveErr)
		return nil
	}

	return &rosterWeek
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
			Colour: colour,
			Offset: i,
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

func newRow() *Row {
	return &Row{
		ID:     uuid.New(),
		Amelia: newSlot(),
		Early:  newSlot(),
		Mid:    newSlot(),
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

func countShifts(shiftCounts map[uuid.UUID][]int, day RosterDay, dayIndex int) map[uuid.UUID][]int {
	recordShifts := func(slot *Slot) {
		if slot.AssignedStaff != nil {
			if _, exists := shiftCounts[*slot.AssignedStaff]; !exists {
				shiftCounts[*slot.AssignedStaff] = make([]int, 7)
			}
			shiftCounts[*slot.AssignedStaff][dayIndex]++
		}
	}

	for _, row := range day.Rows {
		if day.AmeliaOpen {
			recordShifts(&row.Amelia)
		}
		recordShifts(&row.Early)
		recordShifts(&row.Mid)
		recordShifts(&row.Late)
	}

	return shiftCounts
}

func getCurentShifts(counts []int) int {
	total := 0
	for _, value := range counts {
		total += value
	}
	return total
}

func (d *Database) CheckFlags(allStaff []*StaffMember, week RosterWeek) RosterWeek {
	staffMap := make(map[uuid.UUID]*StaffMember, len(allStaff))
	for _, staff := range allStaff {
		staff.CurrentShifts = 0
		staffMap[staff.ID] = staff
	}
	shiftCounts := make(map[uuid.UUID][]int)

	for i := range week.Days {
		shiftCounts = countShifts(shiftCounts, *week.Days[i], i)
	}
	for staffID, counts := range shiftCounts {
		total := getCurentShifts(counts)
		if staff, ok := staffMap[staffID]; ok {
			staff.CurrentShifts = total
		}
	}
	err := d.SaveStaffMembers(allStaff)
	if err != nil {
		// TODO: Handle this?
		log.Println("Error updating staff in checkflags")
	}

	for i := range week.Days {
		newDay := assignFlags(week.Days[i], week.StartDate.AddDate(0, 0, i), shiftCounts, staffMap, i)
		week.Days[i] = &newDay
	}
	return week
}

func assignFlags(day *RosterDay, date time.Time, shiftCounts map[uuid.UUID][]int, staffMap map[uuid.UUID]*StaffMember, i int) RosterDay {
	processSlot := func(slot *Slot, dayIndex int) {
		if slot.AssignedStaff != nil {
			if shiftCounts[*slot.AssignedStaff][dayIndex] > 1 {
				slot.Flag = Duplicate
			} else if staff, ok := staffMap[*slot.AssignedStaff]; ok {
				for _, req := range staff.LeaveRequests {
					if !req.StartDate.After(date) && req.EndDate.After(date) {
						slot.Flag = LeaveConflict
						return
					}
				}
				availability := staff.Availability[dayIndex]
				if !availability.Late {
					slot.Flag = PrefConflict
					if !availability.Early && !availability.Mid {
						slot.Flag = PrefRefuse
					}
				} else if staff.CurrentShifts == staff.IdealShifts {
					slot.Flag = IdealMet
				} else if staff.CurrentShifts > staff.IdealShifts {
					slot.Flag = IdealExceeded
				}
			}
		}
	}

	for _, row := range day.Rows {
		row.Amelia.Flag = None
		row.Early.Flag = None
		row.Mid.Flag = None
		row.Late.Flag = None

		if day.AmeliaOpen {
			processSlot(&row.Amelia, i)
		}
		processSlot(&row.Early, i)
		processSlot(&row.Mid, i)
		processSlot(&row.Late, i)
	}

	return *day
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

func (s *Slot) HasThisStaff(staffId uuid.UUID) bool {
	if s.AssignedStaff != nil && *s.AssignedStaff == staffId {
		return true
	}
	return false
}

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
	if flag == IdealMet {
		return "#B2E1B0"
	}
	if flag == IdealExceeded {
		return "#D7A9A9"
	}
	return defaultCol
}

func (d *Database) ChangeDayRowCount(
	startDate time.Time,
	dayID uuid.UUID,
	action string) (*RosterDay, bool) {
	week := d.LoadRosterWeek(startDate)
	for i := range week.Days {
		day := week.Days[i]
		if day.ID == dayID {
			if action == "+" {
				week.Days[i].Rows = append(week.Days[i].Rows, newRow())
			} else {
				if len(week.Days[i].Rows) > 4 {
					week.Days[i].Rows = week.Days[i].Rows[:len(week.Days[i].Rows)-1]
				}
			}
			d.SaveRosterWeek(*week)
			return day, week.IsLive
		}
	}
	return nil, false
}
