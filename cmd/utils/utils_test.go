package utils

import (
	"testing"
	"time"
)

// Test WeekStartFromOffset and WeekOffsetFromDate are inverses for a range of offsets.
func TestWeekOffsetMappings(t *testing.T) {
	for _, off := range []int{-5, -1, 0, 1, 10, 100} {
		start := WeekStartFromOffset(off)
		got := WeekOffsetFromDate(start)
		if got != off {
			t.Errorf("WeekOffsetFromDate(WeekStartFromOffset(%d)) = %d; want %d", off, got, off)
		}
		// Also test a mid-week date: add 3 days should give same offset
		mid := start.AddDate(0, 0, 3)
		if WeekOffsetFromDate(mid) != off {
			t.Errorf("WeekOffsetFromDate(midweek) != %d; got %d", off, WeekOffsetFromDate(mid))
		}
	}
}

func TestWeekStartKnownEpoch(t *testing.T) {
	// offset 0 should be TuesdayEpoch
	ws := WeekStartFromOffset(0)
	if !ws.Equal(TuesdayEpoch) {
		t.Errorf("WeekStartFromOffset(0) = %v; want %v", ws, TuesdayEpoch)
	}
}

func TestLastAndNextWholeHour(t *testing.T) {
	last := LastWholeHour()
	next := NextWholeHour()
	// last should be <= now, next > now
	now := time.Now()
	if last.After(now) {
		t.Errorf("LastWholeHour %v is after now %v", last, now)
	}
	if !next.After(now) {
		t.Errorf("NextWholeHour %v is not after now %v", next, now)
	}
	// gap should be exactly one hour
	delta := next.Sub(last)
	if delta != time.Hour {
		t.Errorf("NextWholeHour - LastWholeHour = %v; want 1h", delta)
	}
}

func TestGetNextAndLastTuesdayWeekday(t *testing.T) {
	nt := GetNextTuesday()
	if nt.Weekday() != time.Tuesday {
		t.Errorf("GetNextTuesday weekday = %v; want Tuesday", nt.Weekday())
	}
	lt := GetLastTuesday()
	if lt.Weekday() != time.Tuesday {
		t.Errorf("GetLastTuesday weekday = %v; want Tuesday", lt.Weekday())
	}
	// Ensure times are at midnight local
	if nt.Hour() != 0 || nt.Minute() != 0 || nt.Second() != 0 {
		t.Errorf("GetNextTuesday time = %v; want midnight", nt)
	}
	if lt.Hour() != 0 || lt.Minute() != 0 || lt.Second() != 0 {
		t.Errorf("GetLastTuesday time = %v; want midnight", lt)
	}
}
