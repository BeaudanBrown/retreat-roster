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

func (row Row) getSlot(slotStr string) *Slot {
	switch slotStr {
	case "Amelia":
		return &row.Amelia
	case "Early":
		return &row.Early
	case "Mid":
		return &row.Mid
	case "Late":
		return &row.Late
	}
	return nil
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
	IdealMet
	IdealExceeded
	PrefConflict
	LateToEarly
	PrefRefuse
	Duplicate
	LeaveConflict
)

func (rw RosterWeek) MarshalBSON() ([]byte, error) {
	type Alias RosterWeek
	aux := &struct {
		*Alias `bson:",inline"`
	}{
		Alias: (*Alias)(&rw),
	}
	year, month, day := aux.StartDate.Date()
	startDateLocal := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	aux.StartDate = startDateLocal.UTC()
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
	aux.StartDate = aux.StartDate.In(time.Local)
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

func (d *Database) SaveAllRosterWeeks(weeks []*RosterWeek) error {
	collection := d.DB.Collection("rosters")
	bulkWriteModels := make([]mongo.WriteModel, len(weeks))
	for i, week := range weeks {
		filter := bson.M{"id": week.ID}
		update := bson.M{"$set": *week}
		bulkWriteModels[i] = mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update).SetUpsert(true)
	}

	opts := options.BulkWrite().SetOrdered(false)
	results, err := collection.BulkWrite(d.Context, bulkWriteModels, opts)
	if err != nil {
		log.Printf("Failed to save roster weeks: %v\n", err)
		return err
	}
	log.Printf("Saved %v roster weeks, Upserted %v roster weeks", results.ModifiedCount, results.UpsertedCount)
	return nil
}

func (d *Database) LoadAllRosterWeeks() []*RosterWeek {
	collection := d.DB.Collection("rosters")
	cursor, err := collection.Find(d.Context, bson.M{})
	if err != nil {
		log.Printf("Error executing query: %v", err)
		return []*RosterWeek{}
	}
	defer cursor.Close(d.Context)

	allWeeks := []*RosterWeek{}
	for cursor.Next(d.Context) {
		var week RosterWeek
		if err := cursor.Decode(&week); err != nil {
			log.Printf("Error loading all roster weeks: %v", err)
			continue
		}
		allWeeks = append(allWeeks, &week)
	}
	return allWeeks
}

func (d *Database) LoadRosterWeek(startDate time.Time) *RosterWeek {
	var rosterWeek RosterWeek
	log.Printf("Loading week starting: %v", startDate)
	filter := bson.M{"startDate": startDate.UTC()}
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

	for i := 0; i < len(week.Days)-1; i++ {
		currentDay := week.Days[i]
		nextDay := week.Days[i+1]
		if currentDay.IsClosed || nextDay.IsClosed {
			continue
		}
		checkLateToEarly(currentDay, nextDay)
	}
	return week
}

func checkLateToEarly(day *RosterDay, nextDay *RosterDay) {
	for _, row := range day.Rows {
		if row.Late.Flag > LateToEarly {
			// Don't overwrite more important flags
			continue
		}
		staff := row.Late.AssignedStaff
		if staff == nil {
			continue
		}
		for _, row2 := range nextDay.Rows {
			if row2.Early.Flag > LateToEarly {
				// Don't overwrite more important flags
				continue
			}
			if row2.Early.HasThisStaff(*staff) {
				row2.Early.Flag = LateToEarly
				row.Late.Flag = LateToEarly
			}
		}
	}
}

func assignFlags(day *RosterDay, date time.Time, shiftCounts map[uuid.UUID][]int, staffMap map[uuid.UUID]*StaffMember, dayIdx int) RosterDay {
	log.Printf("%v on date %v", day.DayName, date)
	processSlot := func(row Row, slotStr string, dayIndex int) Highlight {
		slot := row.getSlot(slotStr)
		if slot.AssignedStaff == nil {
			return None
		}
		if shiftCounts[*slot.AssignedStaff][dayIndex] > 1 {
			return Duplicate
		}
		if staff, ok := staffMap[*slot.AssignedStaff]; ok {
			for _, req := range staff.LeaveRequests {
				if !req.StartDate.After(date) && req.EndDate.After(date) {
					return LeaveConflict
				}
			}
			conflict := staff.GetConflict(slotStr, dayIndex)
			if conflict != None {
				return conflict
			}
			if staff.CurrentShifts == staff.IdealShifts {
				//TODO: move this to a better place for viewing
				// return IdealMet
				return None
			}
			if staff.CurrentShifts > staff.IdealShifts {
				return IdealExceeded
			}
		}
		return None
	}

	for _, row := range day.Rows {
		if day.AmeliaOpen {
			row.Amelia.Flag = processSlot(*row, "Amelia", dayIdx)
		} else {
			row.Amelia.Flag = None
		}
		row.Early.Flag = processSlot(*row, "Early", dayIdx)
		row.Mid.Flag = processSlot(*row, "Mid", dayIdx)
		row.Late.Flag = processSlot(*row, "Late", dayIdx)
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
	if flag == LateToEarly {
		return "#117593"
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
