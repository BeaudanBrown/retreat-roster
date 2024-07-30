package utils

import (
	"time"
)

func GetLastTuesday() time.Time {
	nextTuesday := GetNextTuesday()
	lastTuesday := nextTuesday.AddDate(0, 0, -7)
	return time.Date(
		lastTuesday.Year(),
		lastTuesday.Month(),
		lastTuesday.Day(),
		0, 0, 0, 0,
		lastTuesday.Location())
}

func GetNextTuesday() time.Time {
	today := time.Now()
	daysUntilTuesday := int((7 + (time.Tuesday - today.Weekday())) % 7)
	if daysUntilTuesday == 0 {
		daysUntilTuesday = 7
	}
	nextTuesday := today.AddDate(0, 0, daysUntilTuesday)
	return time.Date(
		nextTuesday.Year(),
		nextTuesday.Month(),
		nextTuesday.Day(),
		0, 0, 0, 0,
		nextTuesday.Location())
}
