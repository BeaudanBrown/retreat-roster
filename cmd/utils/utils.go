package utils

import (
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/pkg/errors"
)

var tuesdayEpoch = time.Date(1970, time.January, 6, 0, 0, 0, 0, time.UTC)

const week = 7 * 24 * time.Hour

// weekStartFromOffset returns the Tuesday-midnight that begins
// the block identified by weekOffset.
func WeekStartFromOffset(weekOffset int) time.Time {
	// each offset is exactly 7 days
	daysToAdd := weekOffset * 7
	return tuesdayEpoch.AddDate(0, 0, daysToAdd)
}

func WeekOffsetFromDate(t time.Time) int {
	d := t.UTC().Sub(tuesdayEpoch)
	off := int(d / week)
	// If d is negative but not an exact multiple of 7 days,
	// we need to subtract 1 to get the mathematical floor.
	if d < 0 && d%week != 0 {
		off--
	}
	return off
}

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

func PrintLog(format string, args ...any) {
	pc, _, _, ok := runtime.Caller(1)
	funcName := "unknown"
	if ok {
		funcName = runtime.FuncForPC(pc).Name()
	}
	msg := fmt.Sprintf(format, args...)
	log.Printf("%v: %v", funcName, msg)
}

func PrintError(err error, msg string) {
	pc, _, _, ok := runtime.Caller(1)
	funcName := "unknown"
	if ok {
		funcName = runtime.FuncForPC(pc).Name()
	}
	wrappedErr := errors.Wrap(err, funcName+": "+msg)
	log.Printf("%v", wrappedErr)
}

func LastWholeHour() time.Time {
	t := time.Now()
	return t.Truncate(time.Hour)
}

func NextWholeHour() time.Time {
	t := time.Now()
	return t.Truncate(time.Hour).Add(time.Hour)
}
