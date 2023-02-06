package date

import (
	"fmt"
	"strings"
	"time"
)

type TimeRange struct {
	Start string // HH:MM:SS
	End   string
}

func (t TimeRange) ToString() string {
	return fmt.Sprintf("%v - %v", t.Start, t.End)
}

func ParseDate(s string) (*time.Time, error) {
	dateTime, err := time.ParseInLocation("2006-01-02", s, time.Local)

	if err != nil {
		return nil, err
	}

	return &dateTime, nil
}

func ParseDateTime(s string) (*time.Time, error) {
	dateTime, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)

	if err != nil {
		return nil, err
	}

	return &dateTime, nil
}

func ParseTimeRange(s string) (*TimeRange, error) {
	timeRange := strings.Split(s, " - ")

	if len(timeRange) != 2 {
		return nil, fmt.Errorf("invalid reservation time range: %v, "+
			"expect format: HH:MM:SS - HH:MM:SS", s)
	}

	for _, timeStr := range timeRange {
		_, err := time.ParseInLocation("15:04:05", timeStr, time.Local)
		if err != nil {
			return nil, err
		}
	}

	return &TimeRange{Start: timeRange[0], End: timeRange[1]}, nil
}

func ToAtString(in *time.Time) string {
	return in.Format("15:04 02.01.2006")
}
