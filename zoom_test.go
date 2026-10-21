package main

import (
	"testing"
	"time"
)

func TestGenerateZoomSlots(t *testing.T) {
	input := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	zoomSlots := generateZoomSlots(input)

	want := []struct {
		startHour int
		endHour   int
	}{
		{8, 10},
		{10, 12},
		{12, 13},
		{13, 15},
		{15, 17},
	}

	if len(zoomSlots) != len(want) {
		t.Fatalf("got %d slots, want %d", len(zoomSlots), len(want))
	}

	for i, expected := range want {
		if zoomSlots[i].Start.Hour() != expected.startHour {
			t.Errorf("slot %d starts at %d, want %d", i, zoomSlots[i].Start.Hour(), expected.startHour)
		}
		if zoomSlots[i].End.Hour() != expected.endHour {
			t.Errorf("slot %d ends at %d, want %d", i, zoomSlots[i].End.Hour(), expected.endHour)
		}
	}
}

func TestIsAvailable(t *testing.T) {
	shiftStart := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	shiftEnd := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	zoomStart := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	zoomEnd := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)

	available := isAvailable(
		shiftStart,
		shiftEnd,
		zoomStart,
		zoomEnd,
	)

	if !available {
		t.Error("expected the employee to be available")
	}
}

func TestIsAvailableRejectsPartialCoverage(t *testing.T) {
	shiftStart := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	shiftEnd := time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)

	zoomStart := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	zoomEnd := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	available := isAvailable(
		shiftStart,
		shiftEnd,
		zoomStart,
		zoomEnd,
	)

	if available {
		t.Error("expected the employee to be not available")
	}
}

func TestAssignZoomSlotsWithoutEmployees(t *testing.T) {
	input := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	slots := generateZoomSlots(input)

	assignedSlots := assignZoomSlots(slots, nil, nil)
	if assignedSlots[0].UserID != "Unassigned" {
		t.Errorf("got %q, want %q", assignedSlots[0].UserID, "Unassigned")
	}
}

func TestAssignZoomSlotsSplitsUnavailableTwoHourBlock(t *testing.T) {
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	slots := []ZoomShift{
		{
			Start: day.Add(8 * time.Hour),
			End:   day.Add(10 * time.Hour),
		},
	}
	employees := []EmployeeAvailability{
		{
			UserID: "student-1",
			Start:  day.Add(8 * time.Hour),
			End:    day.Add(9 * time.Hour),
		},
		{
			UserID: "student-2",
			Start:  day.Add(9 * time.Hour),
			End:    day.Add(10 * time.Hour),
		},
	}

	assignedSlots := assignZoomSlots(slots, employees, nil)
	if len(assignedSlots) != 2 {
		t.Fatalf("got %d slots, want 2", len(assignedSlots))
	}
	if assignedSlots[0].UserID != "student-1" {
		t.Errorf("first slot got %q, want %q", assignedSlots[0].UserID, "student-1")
	}
	if assignedSlots[1].UserID != "student-2" {
		t.Errorf("second slot got %q, want %q", assignedSlots[1].UserID, "student-2")
	}
}

func TestAssignZoomSlotsSplitsTwoHourBlockForFairness(t *testing.T) {
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	slots := []ZoomShift{
		{Start: day.Add(8 * time.Hour), End: day.Add(9 * time.Hour)},
		{Start: day.Add(9 * time.Hour), End: day.Add(11 * time.Hour)},
	}
	employees := []EmployeeAvailability{
		{
			UserID: "student-1",
			Start:  day.Add(8 * time.Hour),
			End:    day.Add(11 * time.Hour),
		},
		{
			UserID: "student-2",
			Start:  day.Add(9 * time.Hour),
			End:    day.Add(10 * time.Hour),
		},
		{
			UserID: "student-3",
			Start:  day.Add(10 * time.Hour),
			End:    day.Add(11 * time.Hour),
		},
	}

	assignedSlots := assignZoomSlots(slots, employees, nil)
	if len(assignedSlots) != 3 {
		t.Fatalf("got %d slots, want 3", len(assignedSlots))
	}

	wantUserIDs := []string{"student-1", "student-2", "student-3"}
	for i, wantUserID := range wantUserIDs {
		if assignedSlots[i].UserID != wantUserID {
			t.Errorf("slot %d got %q, want %q", i, assignedSlots[i].UserID, wantUserID)
		}
	}
}

func TestAssignZoomSlotsWithEmployee(t *testing.T) {
	employees := []EmployeeAvailability{
		{
			UserID: "student-1",
			Start:  time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC),
			End:    time.Date(2026, 10, 20, 17, 0, 0, 0, time.UTC),
		},
	}

	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	slots := generateZoomSlots(day)

	assignedSlots := assignZoomSlots(slots, employees, nil)
	if assignedSlots[0].UserID != "student-1" {
		t.Errorf("got %q, want %q", assignedSlots[0].UserID, "Unassigned")
	}

}

func TestAssignZoomSlotsBalancesEmployees(t *testing.T) {
	employees := []EmployeeAvailability{
		{
			UserID: "student-1",
			Start:  time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC),
			End:    time.Date(2026, 10, 20, 17, 0, 0, 0, time.UTC),
		},
		{
			UserID: "student-2",
			Start:  time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC),
			End:    time.Date(2026, 10, 20, 17, 0, 0, 0, time.UTC),
		},
	}
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	slots := generateZoomSlots(day)

	assignedSlots := assignZoomSlots(slots, employees, nil)
	if assignedSlots[0].UserID == assignedSlots[1].UserID {
		t.Error("expected the first two slots to use different employees")
	}
}

func TestWorkedYesterday(t *testing.T) {
	yesterday := []ZoomShift{
		{
			UserID: "student-1",
			Start:  time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC),
			End:    time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC),
		},
	}

	start := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)

	worked := workedYesterday("student-1", start, end, yesterday)

	if !worked {
		t.Error("expected student-1 to have worked this time yesterday")
	}
}

func TestFormatZoomSlotUnassigned(t *testing.T) {
	slot := ZoomShift{
		UserID: "Unassigned",
		Start:  time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC),
		End:    time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC),
	}

	got, err := formatZoomSlot("", slot)
	if err != nil {
		t.Fatalf("formatZoomSlot returned an error: %v", err)
	}

	if got != "8-9 Unassigned" {
		t.Errorf("got %q, want %q", got, "8-9 Unassigned")
	}
}
