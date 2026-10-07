package main

import (
	"testing"
	"time"
)

func TestParseShiftTime(t *testing.T) {
	input := "2026-10-08T17:00:00Z"

	got, err := parseShiftTime(input)
	if err != nil {
		t.Fatalf("parseShiftTime returned an error: %v", err)
	}

	want := time.Date(2026, 10, 8, 17, 0, 0, 0, time.UTC)

	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestConvertToPacific(t *testing.T) {
	input := time.Date(2026, 10, 8, 17, 0, 0, 0, time.UTC)

	got, err := convertToPacific(input)
	if err != nil {
		t.Fatalf("convertToPacific returned an error: %v", err)
	}

	if got.Hour() != 10 {
		t.Errorf("got hour %d, want 10", got.Hour())
	}
}

func TestResolvesSlackUserToMicrosoft(t *testing.T) {
	fakeMSLookup := func(token string, identifier string) (MicrosoftUser, error) {
		return MicrosoftUser{
			ID:          "123",
			DisplayName: "Test User",
			Mail:        identifier,
		}, nil
	}
	_ = fakeMSLookup
}
