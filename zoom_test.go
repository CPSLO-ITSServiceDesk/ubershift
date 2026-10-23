package main

import (
	"testing"
	"time"
)

func TestZoomExcludedUserIDs(t *testing.T) {
	t.Setenv("MS_ZOOM_EXCLUDED_USER_IDS", "lead-1, lead-2,  ")

	excluded := zoomExcludedUserIDs()
	if !excluded["lead-1"] || !excluded["lead-2"] {
		t.Error("expected both lead IDs to be excluded")
	}
	if excluded["student-1"] {
		t.Error("did not expect student-1 to be excluded")
	}
}

func TestGetAvailabilityExcludesLeads(t *testing.T) {
	t.Setenv("MS_SERVICE_DESK_GROUP_ID", "service-desk")
	t.Setenv("MS_ZOOM_EXCLUDED_USER_IDS", "lead-1")

	availability := getAvailability([]Shift{
		{
			UserID:            "lead-1",
			SchedulingGroupID: "service-desk",
			SharedShift: &SharedShift{
				StartDateTime: "2026-10-20T15:00:00Z",
				EndDateTime:   "2026-10-20T17:00:00Z",
			},
		},
		{
			UserID:            "student-1",
			SchedulingGroupID: "service-desk",
			SharedShift: &SharedShift{
				StartDateTime: "2026-10-20T15:00:00Z",
				EndDateTime:   "2026-10-20T17:00:00Z",
			},
		},
	})

	if len(availability) != 1 {
		t.Fatalf("got %d eligible employees, want 1", len(availability))
	}
	if availability[0].UserID != "student-1" {
		t.Errorf("got %q, want %q", availability[0].UserID, "student-1")
	}
}

func TestExcludeZoomLeads(t *testing.T) {
	employees := []EmployeeAvailability{
		{UserID: "lead-1"},
		{UserID: "student-1"},
		{UserID: "lead-2"},
	}
	users := map[string]MicrosoftUser{
		"lead-1":    {DisplayName: "Tyler Peterson"},
		"student-1": {DisplayName: "Student Assistant"},
		"lead-2":    {DisplayName: "Samuel Example"},
	}

	filtered, err := excludeZoomLeads(employees, func(userID string) (MicrosoftUser, error) {
		return users[userID], nil
	})
	if err != nil {
		t.Fatalf("excludeZoomLeads returned an error: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("got %d Zoom candidates, want 1", len(filtered))
	}
	if filtered[0].UserID != "student-1" {
		t.Errorf("got %q, want %q", filtered[0].UserID, "student-1")
	}
}

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

func TestAssignZoomSlotsUsesShortAvailability(t *testing.T) {
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	slots := []ZoomShift{{
		Start: day.Add(12 * time.Hour),
		End:   day.Add(13 * time.Hour),
	}}
	employees := []EmployeeAvailability{{
		UserID: "student-1",
		Start:  day.Add(12*time.Hour + 30*time.Minute),
		End:    day.Add(13 * time.Hour),
	}}

	assignedSlots := assignZoomSlots(slots, employees, nil)
	if len(assignedSlots) != 2 {
		t.Fatalf("got %d slots, want 2", len(assignedSlots))
	}
	if assignedSlots[0].UserID != "Unassigned" {
		t.Errorf("first slot got %q, want %q", assignedSlots[0].UserID, "Unassigned")
	}
	if assignedSlots[1].UserID != "student-1" {
		t.Errorf("second slot got %q, want %q", assignedSlots[1].UserID, "student-1")
	}
	if assignedSlots[1].Start.Minute() != 30 {
		t.Errorf("second slot starts at minute %d, want 30", assignedSlots[1].Start.Minute())
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

func TestWorkedYesterdayDetectsMinuteOverlap(t *testing.T) {
	yesterday := []ZoomShift{{
		UserID: "student-1",
		Start:  time.Date(2026, 10, 19, 9, 15, 0, 0, time.UTC),
		End:    time.Date(2026, 10, 19, 9, 45, 0, 0, time.UTC),
	}}

	start := time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)
	end := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	if !workedYesterday("student-1", start, end, yesterday) {
		t.Error("expected minute-level overlap to be detected")
	}
}

func TestIsZoomCoverageDay(t *testing.T) {
	if !isZoomCoverageDay(time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)) {
		t.Error("expected Monday to be a Zoom coverage day")
	}
	if isZoomCoverageDay(time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)) {
		t.Error("expected Saturday not to be a Zoom coverage day")
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
