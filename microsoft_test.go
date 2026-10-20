package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseShiftTime(t *testing.T) {
	input := "2026-10-08T17:00:00Z"

	got, err := parseShiftTime(input)
	if err != nil {
		t.Fatalf("parseShiftTime returned an error: %v", err)
	}

	want := time.Date(2026, 10, 8, 17, 0, 0, 0, time.UTC)

	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestConvertToPacific(t *testing.T) {
	input := time.Date(2026, 10, 8, 17, 0, 0, 0, time.UTC)

	got, err := convertToPacific(input)
	if err != nil {
		t.Fatalf("convertToPacific returned an error: %v", err)
	}

	if got.Hour() != 10 {
		t.Errorf("got hour %d, want 10", got.Hour())
	}
}

func TestResolvesSlackUserToMicrosoft(t *testing.T) {
	fakeMSLookup := func(token string, identifier string) (MicrosoftUser, error) {
		return MicrosoftUser{
			ID:          "123",
			DisplayName: "Test User",
			Mail:        identifier,
		}, nil
	}
	_ = fakeMSLookup
}

func TestTimeOffRequestResponse(t *testing.T) {
	jsonResponse := `{
		"value": [
			{
				"id": "request-123",
				"state": "approved",
				"senderUserId": "user-456",
				"startDateTime": "2026-10-20T16:00:00Z",
				"endDateTime": "2026-10-20T20:00:00Z"
			}
		],
		"@odata.nextLink": ""
	}`

	var response TimeOffRequestResponse

	err := json.Unmarshal([]byte(jsonResponse), &response)
	if err != nil {
		t.Fatalf("json.Unmarshal returned an error: %v", err)
	}

	if len(response.Value) != 1 {
		t.Fatalf("got %d requests, want 1", len(response.Value))
	}

	// Put your field assertions below the length check.
	request := response.Value[0]

	if request.State != "approved" {
		t.Errorf("got state %q, want %q", request.State, "approved")
	}

	if request.SenderUserID != "user-456" {
		t.Errorf("got sender user ID %q, want %q", request.SenderUserID, "user-456")
	}

	if request.StartDateTime != "2026-10-20T16:00:00Z" {
		t.Errorf("got start %q, want %q", request.StartDateTime, "2026-10-20T16:00:00Z")
	}

	if request.EndDateTime != "2026-10-20T20:00:00Z" {
		t.Errorf("got end %q, want %q", request.EndDateTime, "2026-10-20T20:00:00Z")
	}
}
