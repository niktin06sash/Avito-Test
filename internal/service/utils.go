package service

import (
	"test-backend/internal/apperrors"
	"time"
)

func parseClockRange(start, end string) (time.Time, time.Time, error) {
	s, err := time.Parse("15:04", start)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	e, err := time.Parse("15:04", end)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !s.Before(e) {
		return time.Time{}, time.Time{}, apperrors.ErrFooBadRequest
	}
	return s, e, nil
}

func containsDay(days []int, value int) bool {
	for _, day := range days {
		if day == value {
			return true
		}
	}
	return false
}
