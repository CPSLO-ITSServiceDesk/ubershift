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

	availability, err := getAvailability([]Shift{
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
	}, nil)
	if err != nil {
		t.Fatalf("getAvailability returned an error: %v", err)
	}

	if len(availability) != 1 {
		t.Fatalf("got %d eligible employees, want 1", len(availability))
	}
	if availability[0].UserID != "student-1" {
		t.Errorf("got %q, want %q", availability[0].UserID, "student-1")
	}
}

func TestGetAvailabilitySubtractsApprovedTimeOff(t *testing.T) {
	t.Setenv("MS_SERVICE_DESK_GROUP_ID", "service-desk")
	t.Setenv("MS_ZOOM_EXCLUDED_USER_IDS", "")

	availability, err := getAvailability([]Shift{{
		UserID:            "student-1",
		SchedulingGroupID: "service-desk",
		SharedShift: &SharedShift{
			StartDateTime: "2026-10-20T15:00:00Z",
			EndDateTime:   "2026-10-21T00:00:00Z",
		},
	}}, []TimeOffRequest{{
		State:         "approved",
		SenderUserID:  "student-1",
		StartDateTime: "2026-10-20T19:00:00Z",
		EndDateTime:   "2026-10-20T20:00:00Z",
	}})
	if err != nil {
		t.Fatalf("getAvailability returned an error: %v", err)
	}

	if len(availability) != 2 {
		t.Fatalf("got %d availability windows, want 2", len(availability))
	}
	if availability[0].Start.Hour() != 15 || availability[0].End.Hour() != 19 {
		t.Errorf("first window got %v-%v, want 15:00-19:00", availability[0].Start, availability[0].End)
	}
	if availability[1].Start.Hour() != 20 || availability[1].End.Hour() != 0 {
		t.Errorf("second window got %v-%v, want 20:00-00:00", availability[1].Start, availability[1].End)
	}
}

func TestGetAvailabilitySplitsServiceDeskShiftForMultipleTimeOffRequests(t *testing.T) {
	t.Setenv("MS_SERVICE_DESK_GROUP_ID", "service-desk")
	t.Setenv("MS_ZOOM_EXCLUDED_USER_IDS", "")

	availability, err := getAvailability([]Shift{{
		UserID:            "student-1",
		SchedulingGroupID: "service-desk",
		SharedShift: &SharedShift{
			StartDateTime: "2026-10-23T17:00:00Z", // 10 AM Pacific
			EndDateTime:   "2026-10-23T22:00:00Z", // 3 PM Pacific
		},
	}}, []TimeOffRequest{
		{
			State:         "approved",
			SenderUserID:  "student-1",
			StartDateTime: "2026-10-23T19:00:00Z", // 12 PM Pacific
			EndDateTime:   "2026-10-23T20:00:00Z", // 1 PM Pacific
		},
		{
			State:         "approved",
			SenderUserID:  "student-1",
			StartDateTime: "2026-10-23T20:00:00Z", // 1 PM Pacific
			EndDateTime:   "2026-10-23T21:00:00Z", // 2 PM Pacific
		},
	})
	if err != nil {
		t.Fatalf("getAvailability returned an error: %v", err)
	}
	if len(availability) != 2 {
		t.Fatalf("got %d availability windows, want 2", len(availability))
	}
	if availability[0].Start.Hour() != 17 || availability[0].End.Hour() != 19 {
		t.Errorf("first window got %v-%v, want 17:00-19:00", availability[0].Start, availability[0].End)
	}
	if availability[1].Start.Hour() != 21 || availability[1].End.Hour() != 22 {
		t.Errorf("second window got %v-%v, want 21:00-22:00", availability[1].Start, availability[1].End)
	}
}

func TestFormatServiceDeskAvailabilityShowsOriginalAndAvailableRanges(t *testing.T) {
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("load Pacific timezone: %v", err)
	}
	shiftStart := time.Date(2026, 10, 23, 17, 0, 0, 0, time.UTC)
	shiftEnd := time.Date(2026, 10, 23, 22, 0, 0, 0, time.UTC)

	got := formatServiceDeskAvailability("Kai", shiftStart, shiftEnd, []EmployeeAvailability{
		{Start: shiftStart, End: time.Date(2026, 10, 23, 19, 0, 0, 0, time.UTC)},
		{Start: time.Date(2026, 10, 23, 21, 0, 0, 0, time.UTC), End: shiftEnd},
	}, pacific)

	want := "Kai 10-3 --> 10-12, 2-3\nKai 12-2 --> Open Shift\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGetAvailabilityDropsSecondsLongFragment(t *testing.T) {
	availability, err := subtractApprovedTimeOff(
		"student-1",
		time.Date(2026, 10, 23, 21, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC),
		[]TimeOffRequest{{
			State:         "approved",
			SenderUserID:  "student-1",
			StartDateTime: "2026-10-23T21:00:01Z",
			EndDateTime:   "2026-10-24T00:00:00Z",
		}},
	)
	if err != nil {
		t.Fatalf("subtractApprovedTimeOff returned an error: %v", err)
	}
	if len(availability) != 0 {
		t.Errorf("got %d availability windows, want 0", len(availability))
	}
}

func TestFormatServiceDeskAvailabilityShowsOpenShiftWhenFullyUnavailable(t *testing.T) {
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("load Pacific timezone: %v", err)
	}
	shiftStart := time.Date(2026, 10, 23, 17, 0, 0, 0, time.UTC)
	shiftEnd := time.Date(2026, 10, 23, 19, 0, 0, 0, time.UTC)

	got := formatServiceDeskAvailability("Kai", shiftStart, shiftEnd, nil, pacific)
	want := "~Kai 10-12~ --> Open Shift\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDailyScheduleBlocksRendersClaimedOpenShift(t *testing.T) {
	dailyOpenShiftClaims = NewClaimStore()
	scheduleText := "*Thursday's Schedule*\n\nKai 10-12 --> Open Shift\n"

	renderedText, blocks := dailyScheduleBlocks(scheduleText)
	if renderedText != scheduleText {
		t.Errorf("got %q, want %q", renderedText, scheduleText)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want schedule section and Claim button", len(blocks))
	}

	requestID := dailyOpenShiftID("Kai 10-12 --> Open Shift")
	if success, _ := dailyOpenShiftClaims.Claim(requestID, "U123"); !success {
		t.Fatal("expected first claim to succeed")
	}

	renderedText, blocks = dailyScheduleBlocks(scheduleText)
	want := "*Thursday's Schedule*\n\nKai 10-12 --> <@U123> 10-12\n"
	if renderedText != want {
		t.Errorf("got %q, want %q", renderedText, want)
	}
	if len(blocks) != 1 {
		t.Errorf("got %d blocks after claim, want only the schedule section", len(blocks))
	}
}

func TestOpenShiftButtonLabel(t *testing.T) {
	got := openShiftButtonLabel("Kai 12-2 --> Open Shift")
	if got != "Claim Kai 12-2" {
		t.Errorf("got %q, want %q", got, "Claim Kai 12-2")
	}
}

func TestGetAvailabilityIgnoresPendingTimeOff(t *testing.T) {
	t.Setenv("MS_SERVICE_DESK_GROUP_ID", "service-desk")
	t.Setenv("MS_ZOOM_EXCLUDED_USER_IDS", "")

	availability, err := getAvailability([]Shift{{
		UserID:            "student-1",
		SchedulingGroupID: "service-desk",
		SharedShift: &SharedShift{
			StartDateTime: "2026-10-20T15:00:00Z",
			EndDateTime:   "2026-10-20T17:00:00Z",
		},
	}}, []TimeOffRequest{{
		State:         "pending",
		SenderUserID:  "student-1",
		StartDateTime: "2026-10-20T15:00:00Z",
		EndDateTime:   "2026-10-20T17:00:00Z",
	}})
	if err != nil {
		t.Fatalf("getAvailability returned an error: %v", err)
	}
	if len(availability) != 1 {
		t.Fatalf("got %d availability windows, want 1", len(availability))
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
