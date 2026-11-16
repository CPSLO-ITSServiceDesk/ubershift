package handler

import (
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
