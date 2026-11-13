package autoshift

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveAndLoadClaims(t *testing.T) {
	db, err := openClaimDBAt(filepath.Join(t.TempDir(), "claims.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	saved, err := saveClaims(db, "shift-123", "U123")
	if err != nil {
		t.Fatal(err)
	}
	if !saved {
		t.Fatal("expected first claim to save")
	}

	saved, err = saveClaims(db, "shift-123", "U456")
	if err != nil {
		t.Fatal(err)
	}
	if saved {
		t.Fatal("expected duplicate claim to be rejected")
	}

	claims, err := loadClaims(db)
	if err != nil {
		t.Fatal(err)
	}

	if got := claims["shift-123"]; got != "U123" {
		t.Errorf("got %q, want %q", got, "U123")
	}
}

func TestSavedClaimRestoresScheduleDisplay(t *testing.T) {
	db, err := openClaimDBAt(filepath.Join(t.TempDir(), "claims.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	line := "Kai 12-2 --> Open Shift"
	openShiftID := dailyOpenShiftID(line)

	saved, err := saveClaims(db, openShiftID, "U123")
	if err != nil || !saved {
		t.Fatalf("save claim: saved=%v, err=%v", saved, err)
	}

	claims, err := loadClaims(db)
	if err != nil {
		t.Fatal(err)
	}

	dailyOpenShiftClaims = NewClaimStore()
	for claimID, slackUserID := range claims {
		dailyOpenShiftClaims.Claim(claimID, slackUserID)
	}

	scheduleText := "*Tuesday's Schedule*\n\n" + line + "\n"
	renderedText, _ := dailyScheduleBlocks(scheduleText)

	want := "Kai 12-2 --> <@U123> 12-2"
	if !strings.Contains(renderedText, want) {
		t.Errorf("got %q, want schedule to contain %q", renderedText, want)
	}
}

func TestSavePortionClaimRejectsOverlap(t *testing.T) {
	db, err := openClaimDBAt(filepath.Join(t.TempDir(), "claims.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	day := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)

	firstClaim := ShiftPortionClaim{
		OpenShiftID: "open-shift-1",
		Start:       day.Add(13 * time.Hour), // 1 PM
		End:         day.Add(15 * time.Hour), // 3 PM
		SlackUserID: "U123",
	}

	saved, err := savePortionClaim(db, firstClaim)
	if err != nil || !saved {
		t.Fatalf("first claim: saved=%v, err=%v", saved, err)
	}

	savedClaims, err := loadPortionClaims(db, "open-shift-1")
	if err != nil {
		t.Fatal(err)
	}

	if len(savedClaims) != 1 {
		t.Fatalf("got %d saved claims, want 1", len(savedClaims))
	}

	if got := savedClaims[0].SlackUserID; got != "U123" {
		t.Errorf("got user %q, want %q", got, "U123")
	}

	if !savedClaims[0].Start.Equal(firstClaim.Start) {
		t.Errorf("got start %v, want %v", savedClaims[0].Start, firstClaim.Start)
	}

	if !savedClaims[0].End.Equal(firstClaim.End) {
		t.Errorf("got end %v, want %v", savedClaims[0].End, firstClaim.End)
	}

	overlappingClaim := ShiftPortionClaim{
		OpenShiftID: "open-shift-1",
		Start:       day.Add(14 * time.Hour), // 2 PM
		End:         day.Add(16 * time.Hour), // 4 PM
		SlackUserID: "U456",
	}

	saved, err = savePortionClaim(db, overlappingClaim)
	if err != nil {
		t.Fatal(err)
	}
	if saved {
		t.Fatal("expected overlapping claim to be rejected")
	}
}
