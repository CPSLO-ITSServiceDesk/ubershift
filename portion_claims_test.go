package main

import (
	"testing"
	"time"
)

func TestSplitOpenShiftByClaims(t *testing.T) {
	day := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	openStart := day.Add(13 * time.Hour) // 1 PM
	openEnd := day.Add(17 * time.Hour)   // 5 PM

	claims := []ShiftPortionClaim{{
		OpenShiftID: "open-shift-1",
		Start:       day.Add(13 * time.Hour), // 1 PM
		End:         day.Add(15 * time.Hour), // 3 PM
		SlackUserID: "U123",
	}}

	segments := splitOpenShiftByClaims(openStart, openEnd, claims)

	if len(segments) != 2 {
		t.Fatalf("got %d segments, want 2", len(segments))
	}

	if segments[0].SlackUserID != "U123" {
		t.Errorf("first segment got user %q, want %q", segments[0].SlackUserID, "U123")
	}

	if !segments[0].Start.Equal(openStart) || segments[0].End.Hour() != 15 {
		t.Errorf("first segment got %v-%v, want 1 PM-3 PM", segments[0].Start, segments[0].End)
	}

	if segments[1].SlackUserID != "" {
		t.Errorf("second segment got user %q, want open", segments[1].SlackUserID)
	}

	if segments[1].Start.Hour() != 15 || !segments[1].End.Equal(openEnd) {
		t.Errorf("second segment got %v-%v, want 3 PM-5 PM", segments[1].Start, segments[1].End)
	}
}

func TestFormatOpenShiftSegments(t *testing.T) {
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}

	day := time.Date(2026, 10, 28, 0, 0, 0, 0, pacific)
	segments := []OpenShiftSegment{
		{
			Start:       day.Add(13 * time.Hour),
			End:         day.Add(15 * time.Hour),
			SlackUserID: "U123",
		},
		{
			Start: day.Add(15 * time.Hour),
			End:   day.Add(17 * time.Hour),
		},
	}

	got := formatOpenShiftSegments("Kai", segments, pacific)
	want := "Kai 1-3 --> <@U123> 1-3\nKai 3-5 --> Open Shift\n"

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
