package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoadPreviousWorkdayZoomSlotsSkipsWeekend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zoom-history.json")
	friday := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	want := []ZoomShift{{
		UserID: "student-1",
		Start:  friday.Add(8 * time.Hour),
		End:    friday.Add(10 * time.Hour),
	}}

	if err := saveZoomScheduleHistory(path, friday, want); err != nil {
		t.Fatalf("saveZoomScheduleHistory returned an error: %v", err)
	}

	monday := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	got, err := loadPreviousWorkdayZoomSlots(path, monday)
	if err != nil {
		t.Fatalf("loadPreviousWorkdayZoomSlots returned an error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d slots, want 1", len(got))
	}
	if got[0].UserID != "student-1" {
		t.Errorf("got user %q, want %q", got[0].UserID, "student-1")
	}
}
