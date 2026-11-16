package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

func TestAutoPostSlotIsDueAtEightPacificAcrossDaylightSaving(t *testing.T) {
	utc := time.UTC
	tests := []struct {
		name string
		now  time.Time
		slot string
		want bool
	}{
		{
			name: "summer uses 15 UTC for 8 AM Pacific daylight time",
			now:  time.Date(2026, time.July, 6, 15, 0, 0, 0, utc),
			slot: "15",
			want: true,
		},
		{
			name: "summer skips the standard-time UTC slot",
			now:  time.Date(2026, time.July, 6, 16, 0, 0, 0, utc),
			slot: "16",
			want: false,
		},
		{
			name: "winter uses 16 UTC for 8 AM Pacific standard time",
			now:  time.Date(2026, time.December, 7, 16, 0, 0, 0, utc),
			slot: "16",
			want: true,
		},
		{
			name: "weekends never post",
			now:  time.Date(2026, time.December, 6, 16, 0, 0, 0, utc),
			slot: "16",
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := autoPostSlotIsDue(test.now, test.slot); got != test.want {
				t.Errorf("autoPostSlotIsDue(%v, %q) = %t, want %t", test.now, test.slot, got, test.want)
			}
		})
	}
}

func TestAutomaticPostDestination(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
		valid bool
	}{
		{name: "defaults to work", want: "work", valid: true},
		{name: "supports test channel", value: "test", want: "test", valid: true},
		{name: "supports work channel", value: "work", want: "work", valid: true},
		{name: "rejects unknown destination", value: "other", valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("UBERSHIFT_AUTO_POST_DESTINATION", test.value)
			got, err := automaticPostDestination()
			if test.valid && err != nil {
				t.Fatalf("automaticPostDestination() returned error: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("automaticPostDestination() returned nil error")
			}
			if test.valid && got != test.want {
				t.Errorf("automaticPostDestination() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSlackSignatureValid(t *testing.T) {
	t.Setenv("SLACK_SIGNING_SECRET", "test-signing-secret")
	now := time.Date(2026, time.November, 16, 8, 0, 0, 0, time.UTC)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	body := []byte("payload=%7B%7D")
	base := "v0:" + timestamp + ":" + string(body)
	mac := hmac.New(sha256.New, []byte("test-signing-secret"))
	_, _ = mac.Write([]byte(base))
	signature := "v0=" + hex.EncodeToString(mac.Sum(nil))

	if !slackSignatureValid(timestamp, signature, body, now) {
		t.Fatal("expected valid Slack signature")
	}
	if slackSignatureValid(timestamp, "v0=wrong", body, now) {
		t.Fatal("expected invalid signature to be rejected")
	}
	if slackSignatureValid("1", signature, body, now) {
		t.Fatal("expected stale timestamp to be rejected")
	}
}
