package main

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

const zoomScheduleHistoryPath = ".zoom_schedule_history.json"

var zoomScheduleHistoryMu sync.Mutex

type ZoomScheduleHistory struct {
	Days map[string][]ZoomShift `json:"days"`
}

func saveZoomScheduleHistory(path string, day time.Time, slots []ZoomShift) error {
	zoomScheduleHistoryMu.Lock()
	defer zoomScheduleHistoryMu.Unlock()

	history, err := readZoomScheduleHistory(path)
	if err != nil {
		return err
	}

	if history.Days == nil {
		history.Days = make(map[string][]ZoomShift)
	}
	history.Days[day.Format("2006-01-02")] = slots

	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func loadPreviousWorkdayZoomSlots(path string, day time.Time) ([]ZoomShift, error) {
	zoomScheduleHistoryMu.Lock()
	defer zoomScheduleHistoryMu.Unlock()

	history, err := readZoomScheduleHistory(path)
	if err != nil {
		return nil, err
	}

	return history.Days[previousWorkday(day).Format("2006-01-02")], nil
}

func readZoomScheduleHistory(path string) (ZoomScheduleHistory, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ZoomScheduleHistory{Days: make(map[string][]ZoomShift)}, nil
	}
	if err != nil {
		return ZoomScheduleHistory{}, err
	}

	var history ZoomScheduleHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return ZoomScheduleHistory{}, err
	}
	if history.Days == nil {
		history.Days = make(map[string][]ZoomShift)
	}

	return history, nil
}

func previousWorkday(day time.Time) time.Time {
	previous := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	for {
		previous = previous.AddDate(0, 0, -1)
		if previous.Weekday() != time.Saturday && previous.Weekday() != time.Sunday {
			return previous
		}
	}
}
