package autoshift

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDailyScheduleBlocksRenderClaimedAndRemainingPortions(t *testing.T) {
	db, err := openClaimDBAt(filepath.Join(t.TempDir(), "claims.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	day := time.Now().In(pacific)
	line := "~Kai 1-5~ --> Open Shift"
	_, start, _, ok := parseOpenShiftLine(line, day)
	if !ok {
		t.Fatal("expected Open Shift line to parse")
	}

	saved, err := savePortionClaim(db, ShiftPortionClaim{
		OpenShiftID: dailyOpenShiftID(line),
		Start:       start,
		End:         start.Add(2 * time.Hour),
		SlackUserID: "U123",
	})
	if err != nil || !saved {
		t.Fatalf("save portion claim: saved=%v, err=%v", saved, err)
	}

	renderedText, blocks := dailyScheduleBlocksWithPortionClaims(
		"*Today's Schedule*\n\n"+line+"\n",
		db,
	)
	if !strings.Contains(renderedText, "~Kai 1-3~ --> <@U123> 1-3") {
		t.Errorf("missing claimed portion in %q", renderedText)
	}
	if !strings.Contains(renderedText, "~Kai 3-5~ --> Open Shift") {
		t.Errorf("missing open remainder in %q", renderedText)
	}
	if len(blocks) != 2 {
		t.Errorf("got %d blocks, want schedule and one remaining-portion button", len(blocks))
	}

}
