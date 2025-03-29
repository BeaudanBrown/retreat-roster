package utils

import (
	"time"
	"log"

	"github.com/pkg/errors"
)

func GetLastTuesday() time.Time {
	nextTuesday := GetNextTuesday()
	lastTuesday := nextTuesday.AddDate(0, 0, -7)
	return time.Date(
		lastTuesday.Year(),
		lastTuesday.Month(),
		lastTuesday.Day(),
		0, 0, 0, 0,
		time.Local)
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
		time.Local)
}

func PrintError(err error, msg string) {
	wrappedErr := errors.Wrap(err, msg)
	log.Printf("%+v", wrappedErr)
}
