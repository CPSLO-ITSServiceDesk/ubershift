package autoshift

import "testing"

func TestPostDashboardScheduleValidatesInputBeforePosting(t *testing.T) {
	tests := []struct {
		name        string
		schedule    string
		destination string
		want        string
	}{
		{
			name:        "empty schedule",
			destination: "test",
			want:        "schedule text is empty",
		},
		{
			name:        "unknown destination",
			schedule:    "Service Desk schedule",
			destination: "unknown",
			want:        "invalid Slack destination",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := PostDashboardSchedule(tt.schedule, tt.destination)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("PostDashboardSchedule() error = %v, want %q", err, tt.want)
			}
		})
	}
}
