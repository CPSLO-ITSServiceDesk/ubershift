package autoshift

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

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

func TestGetAllTimeOffRequestsFollowsNextLink(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization header = %q, want Bearer test-token", request.Header.Get("Authorization"))
		}

		responses := map[string]string{
			"https://graph.example/first": `{
				"value": [{"id":"request-1"}],
				"@odata.nextLink": "https://graph.example/second"
			}`,
			"https://graph.example/second": `{
				"value": [{"id":"request-2"}]
			}`,
		}
		body, ok := responses[request.URL.String()]
		if !ok {
			t.Fatalf("unexpected request URL %q", request.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}

	requests, err := getAllTimeOffRequests(client, "test-token", "https://graph.example/first")
	if err != nil {
		t.Fatalf("getAllTimeOffRequests returned an error: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("got %d requests, want 2", len(requests))
	}
	if requests[0].ID != "request-1" || requests[1].ID != "request-2" {
		t.Errorf("got IDs %q and %q, want request-1 and request-2", requests[0].ID, requests[1].ID)
	}
}

func TestTimeOffRequestsForDay(t *testing.T) {
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("load Pacific timezone: %v", err)
	}
	day := time.Date(2026, 10, 23, 0, 0, 0, 0, pacific)

	matching, err := timeOffRequestsForDay([]TimeOffRequest{
		{
			ID:            "today",
			StartDateTime: "2026-10-23T19:00:00Z",
			EndDateTime:   "2026-10-23T20:00:00Z",
		},
		{
			ID:            "previous-day",
			StartDateTime: "2026-10-22T19:00:00Z",
			EndDateTime:   "2026-10-22T20:00:00Z",
		},
		{
			ID:            "next-day",
			StartDateTime: "2026-10-24T19:00:00Z",
			EndDateTime:   "2026-10-24T20:00:00Z",
		},
	}, day)
	if err != nil {
		t.Fatalf("timeOffRequestsForDay returned an error: %v", err)
	}
	if len(matching) != 1 {
		t.Fatalf("got %d requests, want 1", len(matching))
	}
	if matching[0].ID != "today" {
		t.Errorf("got request %q, want today", matching[0].ID)
	}
}
