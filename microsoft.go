package autoshift

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
	"github.com/slack-go/slack"
)

// decoding Graph JSON - add JSON tag
type ShiftResponse struct {
	Value []Shift `json:"value"`
}

type Shift struct {
	UserID            string       `json:"userId"`
	SchedulingGroupID string       `json:"schedulingGroupId"`
	ID                string       `json:"id"`
	SharedShift       *SharedShift `json:"sharedShift"`
}

type SharedShift struct {
	StartDateTime string `json:"startDateTime"`
	EndDateTime   string `json:"endDateTime"`
	Notes         string `json:"notes"`
}

type UserSchedule struct {
	Name   string      `json:"name"`
	Shifts []TimeRange `json:"shifts"`
}

type TimeRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type MicrosoftUser struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	Mail              string `json:"mail"`
	UserPrincipleName string `json:"userPrincipleName"`
}

type OpenShift struct {
	ID                string `json:"id"`
	SchedulingGroupID string `json:"schedulingGroupId"`
	SharedOpenShift   *struct {
		StartDateTime string `json:"startDateTime"`
		EndDateTime   string `json:"endDateTime"`
	} `json:"sharedOpenShift"`
}

type TimeOffRequest struct {
	ID            string `json:"id"`
	State         string `json:"state"`
	SenderUserID  string `json:"senderUserId"`
	StartDateTime string `json:"startDateTime"`
	EndDateTime   string `json:"endDateTime"`
}

type TimeOffRequestResponse struct {
	Value    []TimeOffRequest `json:"value"`
	NextLink string           `json:"@odata.nextLink"`
}

type SlackEmailLookup func(*slack.Client, string) (string, error)
type MicrosoftUserLookup func(string, string) (MicrosoftUser, error)

func getMicrosoftToken() (string, error) {
	clientid := os.Getenv("MS_CLIENT_ID")
	tenantid := os.Getenv("MS_TENANT_ID")
	clientSecret := os.Getenv("MS_CLIENT_SECRET")

	if clientid == "" || tenantid == "" {
		return "", fmt.Errorf("MS_CLIENT_ID or MS_TENANT_ID not found")
	}

	authority := "https://login.microsoftonline.com/" + tenantid
	if clientSecret != "" {
		credential, err := confidential.NewCredFromSecret(clientSecret)
		if err != nil {
			return "", fmt.Errorf("create Microsoft client-secret credential: %w", err)
		}

		app, err := confidential.New(
			authority,
			clientid,
			credential,
		)
		if err != nil {
			return "", fmt.Errorf("create Microsoft confidential client: %w", err)
		}

		result, err := app.AcquireTokenByCredential(
			context.Background(),
			[]string{"https://graph.microsoft.com/.default"},
		)
		if err != nil {
			return "", fmt.Errorf("get Microsoft app token: %w", err)
		}
		return result.AccessToken, nil
	}

	app, err := public.New(
		clientid,
		public.WithAuthority(authority),
		public.WithCache(&TokenCache{path: ".msal_cache"}),
	)

	if err != nil {
		return "", err
	}

	scopes := []string{
		"Schedule.Read.All",
		"Schedule.ReadWrite.All",
		"User.ReadBasic.All",
	}
	accounts, err := app.Accounts(context.Background())
	if err == nil && len(accounts) > 0 {
		result, err := app.AcquireTokenSilent(
			context.Background(),
			scopes,
			public.WithSilentAccount(accounts[0]),
		)

		if err == nil {
			return result.AccessToken, nil
		}
	}

	deviceCode, err := app.AcquireTokenByDeviceCode(
		context.Background(),
		scopes,
	)
	if err != nil {
		return "", err
	}

	fmt.Println(deviceCode.Result.Message)

	result, err := deviceCode.AuthenticationResult(
		context.Background(),
	)
	if err != nil {
		return "", err
	}
	return result.AccessToken, nil
}

func getShifts(token string) ([]Shift, error) {
	return getShiftsForDay(token, time.Now())
}

func getShiftsForDay(token string, day time.Time) ([]Shift, error) {
	teamID := os.Getenv("MS_TEAM_ID")

	if teamID == "" {
		return nil, fmt.Errorf("MS_TEAM_ID not found")
	}

	pacificDay, err := convertToPacific(day)
	if err != nil {
		return nil, err
	}

	midnight := time.Date(
		pacificDay.Year(),
		pacificDay.Month(),
		pacificDay.Day(),
		0, 0, 0, 0,
		pacificDay.Location(),
	)

	startDate := midnight.UTC().Format(time.RFC3339)
	filter := fmt.Sprintf(
		"sharedShift/startDateTime ge %s",
		startDate,
	)

	baseURL := "https://graph.microsoft.com/v1.0/teams/" +
		teamID +
		"/schedule/shifts"

	params := url.Values{}
	params.Set("$filter", filter)

	finalURL := baseURL + "?" + params.Encode()
	req, err := http.NewRequest("GET", finalURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// turns raw JSON into a ShiftResponse
	var shift_response ShiftResponse
	err = json.Unmarshal(body, &shift_response)
	if err != nil {
		return nil, err
	}

	var todaysShifts []Shift
	for _, shift := range shift_response.Value {
		if shift.SharedShift == nil {
			continue
		}

		// 1. parse start time
		startTime, err := parseShiftTime(shift.SharedShift.StartDateTime)
		if err != nil {
			continue
		}

		// 2. convert to pacific
		pacificStartTime, err := convertToPacific(startTime)
		if err != nil {
			return nil, err
		}

		// Check if the shift starts on the requested Pacific calendar day.
		if pacificStartTime.Year() == pacificDay.Year() &&
			pacificStartTime.Month() == pacificDay.Month() &&
			pacificStartTime.Day() == pacificDay.Day() {

			todaysShifts = append(todaysShifts, shift)
		}
	}
	fmt.Println("Graph status:", resp.Status)

	return todaysShifts, nil
}

func formatShiftTime(utcString string) (string, error) {
	shiftTime, err := parseShiftTime(utcString)
	if err != nil {
		return "", err
	}
	pacificTime, err := convertToPacific(shiftTime)
	if err != nil {
		return "", err
	}
	formattedTime := pacificTime.Format("3:04")
	formattedTime = strings.TrimSuffix(formattedTime, ":00")
	return formattedTime, nil
}

// func handleClaim() error {

// }
/*
slack user ID --> email
*/
func getSlackUserEmail(api *slack.Client, slackUserID string) (string, error) {
	user, err := api.GetUserInfo(slackUserID)
	if err != nil {
		return "", err
	}
	log.Println("Slack profile email: ", user.Profile.Email)
	return user.Profile.Email, nil
}

// get Microsoft ID from a Shift
// user, err := getMicrosoftUser(token, shift.UserId)
// get Microsoft user from Cal Poly email from Slack
// user, err := getMicrosoftUser(token, email)
func getMicrosoftUser(token string, identifier string) (MicrosoftUser, error) {
	url := "https://graph.microsoft.com/v1.0/users/" + identifier

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return MicrosoftUser{}, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return MicrosoftUser{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return MicrosoftUser{}, fmt.Errorf("Microsoft user lookup failed: %s", resp.Status)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return MicrosoftUser{}, err
	}

	// turns raw JSON into a MicrosoftUser
	var ms_user MicrosoftUser
	err = json.Unmarshal(body, &ms_user)
	if err != nil {
		return MicrosoftUser{}, err
	}

	return ms_user, nil
}

func resolvesSlackUserToMicrosoft(api *slack.Client, token string, slackUserId string, getEmail SlackEmailLookup, getMSUser MicrosoftUserLookup) (MicrosoftUser, error) {
	slack_email, err := getEmail(api, slackUserId)
	log.Println("Slack email:", slack_email)
	if err != nil {
		return MicrosoftUser{}, err
	}
	ms_user, err := getMSUser(token, slack_email)
	if err != nil {
		return MicrosoftUser{}, err
	}

	return ms_user, nil
}

/*
1. Microsft user ID + shiftiID
2. update Slack meessage: claimed by @Kai
*/
func assignShift(token string, teamID string, shiftID string, ms_userID string) error {
	if os.Getenv("UBERSHIFT_DRY_RUN") == "true" {
		log.Printf(
			"DRY RUN: would assign shift %s to Microsoft user %s",
			shiftID,
			ms_userID,
		)
		return nil
	}
	return nil
}

func sortShiftsByStart(shifts []Shift) {
	sort.Slice(shifts, func(i, j int) bool {
		return shifts[i].SharedShift.StartDateTime <
			shifts[j].SharedShift.StartDateTime
	})
}

// parses a formatted string and returns its corresponding time.Time
func parseShiftTime(utcTime string) (time.Time, error) {
	shiftTime, err := time.Parse(time.RFC3339, utcTime)
	if err != nil {
		return time.Time{}, err
	}
	return shiftTime, nil
}

func convertToPacific(utcTime time.Time) (time.Time, error) {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.Time{}, err
	}
	pacificTime := utcTime.In(location)

	return pacificTime, nil
}
func getSchedulingGroups(token string) error {
	teamID := os.Getenv("MS_TEAM_ID")

	url := fmt.Sprintf(
		"https://graph.microsoft.com/v1.0/teams/%s/schedule/schedulingGroups",
		teamID,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Graph API error: %s: %s", resp.Status, string(body))
	}
	return nil
}

func getOpenShifts(token string) ([]OpenShift, error) {
	teamID := os.Getenv("MS_TEAM_ID")

	url := fmt.Sprintf(
		"https://graph.microsoft.com/v1.0/teams/%s/schedule/openShifts",
		teamID,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Graph API returned %s", resp.Status)
	}

	var result struct {
		Value []OpenShift `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var serviceDeskShifts []OpenShift

	for _, shift := range result.Value {
		if shift.SchedulingGroupID == os.Getenv("MS_SERVICE_DESK_GROUP_ID") {
			serviceDeskShifts = append(serviceDeskShifts, shift)
		}
	}

	return serviceDeskShifts, nil
}

// getTimeOffRequests retrieves time-off requests without changing Teams data.
func getTimeOffRequests(token string) ([]TimeOffRequest, error) {
	teamID := os.Getenv("MS_TEAM_ID")
	if teamID == "" {
		return nil, fmt.Errorf("MS_TEAM_ID not found")
	}

	requestURL := fmt.Sprintf(
		"https://graph.microsoft.com/v1.0/teams/%s/schedule/timeOffRequests",
		teamID,
	)

	requests, err := getAllTimeOffRequests(http.DefaultClient, token, requestURL)
	if err != nil {
		return nil, err
	}

	return requests, nil
}

// timeOffRequestsForDay keeps only requests that overlap the calendar day of
// the schedule being built. Graph sends request times in UTC; day uses Pacific
// time because that is the schedule's display timezone.
func timeOffRequestsForDay(requests []TimeOffRequest, day time.Time) ([]TimeOffRequest, error) {
	dayStart := time.Date(
		day.Year(), day.Month(), day.Day(),
		0, 0, 0, 0,
		day.Location(),
	)
	dayEnd := dayStart.AddDate(0, 0, 1)

	var matching []TimeOffRequest
	for _, request := range requests {
		start, err := parseShiftTime(request.StartDateTime)
		if err != nil {
			return nil, fmt.Errorf("parse time-off request %s start: %w", request.ID, err)
		}
		end, err := parseShiftTime(request.EndDateTime)
		if err != nil {
			return nil, fmt.Errorf("parse time-off request %s end: %w", request.ID, err)
		}

		if start.Before(dayEnd) && end.After(dayStart) {
			matching = append(matching, request)
		}
	}

	return matching, nil
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func getAllTimeOffRequests(
	client httpDoer,
	token string,
	requestURL string,
) ([]TimeOffRequest, error) {
	const maxPages = 100

	var requests []TimeOffRequest
	for page := 0; requestURL != ""; page++ {
		if page == maxPages {
			return nil, fmt.Errorf("time-off request pagination exceeded %d pages", maxPages)
		}

		req, err := http.NewRequest(http.MethodGet, requestURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("Graph API returned %s", resp.Status)
		}

		var result TimeOffRequestResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		requests = append(requests, result.Value...)
		requestURL = result.NextLink
	}

	return requests, nil
}
