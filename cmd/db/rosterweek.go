package db

import (
	"log"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type RosterWeek struct {
	ID        uuid.UUID    `bson:"_id"`
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
	staffState := d.LoadStaffState()
	w = d.CheckFlags(staffState, w)
	collection := d.DB.Collection("rosters")
	filter := bson.M{"_id": w.ID}
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

func (d *Database) CheckFlags(staffState StaffState, week RosterWeek) RosterWeek {
	allStaff := staffState.Staff
	for _, staff := range *allStaff {
		staff.CurrentShifts = 0
		d.SaveStaffMember(*staff)
	}
	for i, day := range week.Days {
		// Create a new map for each day to track occurrences of staff IDs within that day
		// TODO: Improve this by batching staff queries and updates
		// TODO: Move staff current shifts check somewhere more sensible
		staffIDOccurrences := make(map[uuid.UUID]int)

		for _, row := range day.Rows {
			if day.AmeliaOpen && row.Amelia.AssignedStaff != nil {
				staffIDOccurrences[*row.Amelia.AssignedStaff]++
				staff := d.GetStaffByID(*row.Amelia.AssignedStaff)
				if staff != nil {
					staff.CurrentShifts += 1
					d.SaveStaffMember(*staff)
				}
			}
			if row.Early.AssignedStaff != nil {
				staffIDOccurrences[*row.Early.AssignedStaff]++
				staff := d.GetStaffByID(*row.Early.AssignedStaff)
				if staff != nil {
					staff.CurrentShifts += 1
					d.SaveStaffMember(*staff)
				}
			}
			if row.Mid.AssignedStaff != nil {
				staffIDOccurrences[*row.Mid.AssignedStaff]++
				staff := d.GetStaffByID(*row.Mid.AssignedStaff)
				if staff != nil {
					staff.CurrentShifts += 1
					d.SaveStaffMember(*staff)
				}
			}
			if row.Late.AssignedStaff != nil {
				staffIDOccurrences[*row.Late.AssignedStaff]++
				staff := d.GetStaffByID(*row.Late.AssignedStaff)
				if staff != nil {
					staff.CurrentShifts += 1
					d.SaveStaffMember(*staff)
				}
			}
		}

		for _, row := range day.Rows {
			row.Amelia.Flag = None
			row.Early.Flag = None
			row.Mid.Flag = None
			row.Late.Flag = None
			date := week.StartDate.AddDate(0, 0, day.Offset)

			if day.AmeliaOpen && row.Amelia.AssignedStaff != nil {
				if staffIDOccurrences[*row.Amelia.AssignedStaff] > 1 {
					row.Amelia.Flag = Duplicate
				} else {
					staff := d.GetStaffByID(*row.Amelia.AssignedStaff)
					for _, req := range staff.LeaveRequests {
						if !req.StartDate.After(date) && req.EndDate.After(date) {
							row.Amelia.Flag = LeaveConflict
							break
						}
					}
					if staff != nil {
						if !staff.Availability[i].Late {
							row.Amelia.Flag = PrefConflict
						}
					}
				}
			}

			if row.Early.AssignedStaff != nil {
				if staffIDOccurrences[*row.Early.AssignedStaff] > 1 {
					row.Early.Flag = Duplicate
				} else {
					staff := d.GetStaffByID(*row.Early.AssignedStaff)
					for _, req := range staff.LeaveRequests {
						if !req.StartDate.After(date) && req.EndDate.After(date) {
							row.Early.Flag = LeaveConflict
							break
						}
					}
					if staff != nil {
						if !staff.Availability[i].Early {
							if !staff.Availability[i].Mid && !staff.Availability[i].Late {
								row.Early.Flag = PrefRefuse
							} else {
								row.Early.Flag = PrefConflict
							}
						}
					}
				}
			}

			if row.Mid.AssignedStaff != nil {
				if staffIDOccurrences[*row.Mid.AssignedStaff] > 1 {
					row.Mid.Flag = Duplicate
				} else {
					staff := d.GetStaffByID(*row.Mid.AssignedStaff)
					if staff != nil {
						for _, req := range staff.LeaveRequests {
							if !req.StartDate.After(date) && req.EndDate.After(date) {
								row.Mid.Flag = LeaveConflict
								break
							}
						}
						if !staff.Availability[i].Mid {
							if !staff.Availability[i].Early && !staff.Availability[i].Late {
								row.Mid.Flag = PrefRefuse
							} else {
								row.Mid.Flag = PrefConflict
							}
						}
					}
				}
			}

			if row.Late.AssignedStaff != nil {
				if staffIDOccurrences[*row.Late.AssignedStaff] > 1 {
					row.Late.Flag = Duplicate
				} else {
					staff := d.GetStaffByID(*row.Late.AssignedStaff)
					for _, req := range staff.LeaveRequests {
						if !req.StartDate.After(date) && req.EndDate.After(date) {
							row.Late.Flag = LeaveConflict
							break
						}
					}
					if staff != nil {
						if !staff.Availability[i].Late {
							if !staff.Availability[i].Early && !staff.Availability[i].Mid {
								row.Late.Flag = PrefRefuse
							} else {
								row.Late.Flag = PrefConflict
							}
						}
					}
				}
			}
		}
	}
	return week
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
