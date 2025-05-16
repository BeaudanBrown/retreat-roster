package migrate

import (
	"roster/cmd/models"
	"roster/cmd/server"
	"roster/cmd/utils"
	"time"
)

const STATE_FILE = "./data/state.json"

type Version struct {
	ID      string
	Version int
}

func DoMigration(v models.Version, s *server.Server) error {
	if v.Version == 1 {
		utils.PrintLog("Migrating to V2")
		allWeeks, err := s.Repos.RosterWeek.LoadAllRosterWeeks()
		if err != nil {
			utils.PrintError(err, "Failed to load all roster weeks")
			return err
		}
		for _, week := range allWeeks {
			week.WeekOffset = utils.WeekOffsetFromDate(week.StartDate)
		}
		err = s.Repos.RosterWeek.SaveAllRosterWeeks(allWeeks)
		if err != nil {
			utils.PrintError(err, "Failed to save all weeks")
			return err
		}

		allStaff, err := s.Repos.Staff.LoadAllStaff()
		if err != nil {
			utils.PrintError(err, "Failed to load all staff")
			return err
		}
		for _, staffMember := range allStaff {
			staffMember.Config.RosterDateOffset = utils.WeekOffsetFromDate(utils.GetLastTuesday())
			staffMember.Config.TimesheetDateOffset = utils.WeekOffsetFromDate(utils.GetLastTuesday())
		}
		err = s.Repos.Staff.SaveStaffMembers(allStaff)
		if err != nil {
			utils.PrintError(err, "Failed to save all staff")
			return err
		}

		allTimesheets, err := s.Repos.Timesheet.GetAllTimesheetEntries()
		if err != nil {
			utils.PrintError(err, "Failed to load all timesheets")
			return err
		}
		for _, timesheet := range *allTimesheets {
			timesheet.WeekOffset = utils.WeekOffsetFromDate(timesheet.StartDate)
			timesheet.DayOffset = int((timesheet.StartDate.Weekday() - time.Tuesday + 7) % 7)
		}
		err = s.Repos.Timesheet.SaveAllTimesheetEntries(*allTimesheets)
		if err != nil {
			utils.PrintError(err, "Failed to save all timesheets")
			return err
		}

		v.Version = 2
		err = s.Repos.Config.SaveVersion(v)
		return err
	}
	return nil
}
