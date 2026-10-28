package main

import (
	"path/filepath"
	"testing"
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