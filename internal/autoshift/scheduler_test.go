package autoshift

import (
	"testing"
	"time"
)

func TestSchedulePostTimeAtSkipsWeekend(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "Friday after posting time schedules Monday",
			now:  time.Date(2026, 10, 23, 8, 0, 0, 0, loc),
			want: time.Date(2026, 10, 26, 7, 30, 0, 0, loc),
		},
		{
			name: "Saturday schedules Monday",
			now:  time.Date(2026, 10, 24, 9, 0, 0, 0, loc),
			want: time.Date(2026, 10, 26, 7, 30, 0, 0, loc),
		},
		{
			name: "weekday before posting time schedules today",
			now:  time.Date(2026, 10, 26, 7, 0, 0, 0, loc),
			want: time.Date(2026, 10, 26, 7, 30, 0, 0, loc),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := schedulePostTimeAt(test.now, loc); !got.Equal(test.want) {
				t.Errorf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestScheduleDayAtUsesMondayOnWeekend(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 10, 25, 10, 0, 0, 0, loc) // Sunday
	want := time.Date(2026, 10, 26, 0, 0, 0, 0, loc)
	if got := scheduleDayAt(now, loc); !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestScheduleDayForPostAtAcceptsOnlyWeekdays(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 26, 9, 0, 0, 0, loc)

	got, err := scheduleDayForPostAt(now, "2026-10-27", loc)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 27, 0, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}

	if _, err := scheduleDayForPostAt(now, "2026-10-31", loc); err == nil {
		t.Error("expected Saturday to be rejected")
	}
}
