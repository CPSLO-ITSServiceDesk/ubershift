package autoshift

import (
	"strings"
	"testing"
	"time"
)

func TestBuildDashboardScheduleBlocksUsesStoredClaims(t *testing.T) {
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, time.November, 16, 0, 0, 0, 0, pacific)
	line := "Kai 1-5 --> Open Shift"
	openShiftID := dailyOpenShiftIDForDay(line, day)

	rendered, blocks := BuildDashboardScheduleBlocks(
		"*Monday's Schedule*\n\n"+line+"\n",
		day,
		[]ShiftPortionClaim{{
			OpenShiftID: openShiftID,
			Start:       time.Date(2026, time.November, 16, 13, 0, 0, 0, pacific),
			End:         time.Date(2026, time.November, 16, 15, 0, 0, 0, pacific),
			SlackUserID: "U123",
		}},
	)

	if !strings.Contains(rendered, "Kai 1-3 --> <@U123> 1-3") {
		t.Errorf("missing claimed segment in %q", rendered)
	}
	if !strings.Contains(rendered, "Kai 3-5 --> Open Shift") {
		t.Errorf("missing open segment in %q", rendered)
	}
	if len(blocks) != 2 {
		t.Errorf("got %d blocks, want schedule and one claim button", len(blocks))
	}
}
