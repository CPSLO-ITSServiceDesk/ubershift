package autoshift

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

const (
	supabaseURLVariable       = "SUPABASE_URL"
	autoshiftSupabaseKeyVar   = "AUTOSHIFT_SUPABASE_SECRET_KEY"
	ubershiftSupabaseKeyVar   = "UBERSHIFT_SUPABASE_SECRET_KEY"
	supabaseSecretKeyVar      = "SUPABASE_SECRET_KEY"
	supabaseServiceRoleKeyVar = "SUPABASE_SERVICE_ROLE_KEY"
	slackSigningSecretVar     = "SLACK_SIGNING_SECRET"
	scheduleMessagesTableName = "autoshift_schedule_messages"
	shiftClaimsTableName      = "autoshift_shift_claims"
)

// supabaseServerKey first uses AutoShift's dedicated key. Vercel's Supabase
// connector can reserve SUPABASE_SECRET_KEY for its own sk_live credential,
// which is not a Supabase Data API secret key. The legacy service-role key is
// retained as a fallback for existing deployments.
func supabaseServerKey() string {
	if key := os.Getenv(autoshiftSupabaseKeyVar); key != "" {
		return key
	}
	if key := os.Getenv(ubershiftSupabaseKeyVar); key != "" {
		return key
	}
	if key := os.Getenv(supabaseSecretKeyVar); key != "" {
		if strings.HasPrefix(key, "sb_secret_") {
			return key
		}
	}
	return os.Getenv(supabaseServiceRoleKeyVar)
}

// setSupabaseAuthHeaders supports both current opaque sb_ keys and legacy
// JWT-based service-role keys. Supabase requires current keys in apikey only.
func setSupabaseAuthHeaders(req *http.Request, key string) {
	req.Header.Set("apikey", key)
	if !strings.HasPrefix(key, "sb_") {
		req.Header.Set("Authorization", "Bearer "+key)
	}
}

func supabaseRequestError(method, table string, status string, responseBody []byte) error {
	detail := strings.TrimSpace(string(responseBody))
	// Supabase error bodies describe request problems but never need to include
	// credentials. Keep logs useful without allowing unexpectedly large output.
	if len(detail) > 500 {
		detail = detail[:500] + "..."
	}
	if detail == "" {
		return fmt.Errorf("Supabase %s %s returned %s", method, table, status)
	}
	return fmt.Errorf("Supabase %s %s returned %s: %s", method, table, status, detail)
}

// VercelClaimsConfigured reports whether a serverless function can persist
// claims outside Vercel's temporary filesystem.
func VercelClaimsConfigured() bool {
	return os.Getenv(supabaseURLVariable) != "" && supabaseServerKey() != ""
}

// VercelShiftClaimsConfigured ensures Vercel can both store claims and verify
// that an incoming claim action genuinely came from Slack.
func VercelShiftClaimsConfigured() bool {
	return VercelClaimsConfigured() && os.Getenv(slackSigningSecretVar) != ""
}

func supabaseRequest(ctx context.Context, method, table string, query url.Values, body any, prefer string) ([]byte, int, error) {
	baseURL := strings.TrimSuffix(os.Getenv(supabaseURLVariable), "/")
	serviceKey := supabaseServerKey()
	if baseURL == "" || serviceKey == "" {
		return nil, 0, fmt.Errorf("%s and either %s or %s must be set", supabaseURLVariable, autoshiftSupabaseKeyVar, supabaseServiceRoleKeyVar)
	}

	endpoint := baseURL + "/rest/v1/" + table
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("encode Supabase request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, 0, fmt.Errorf("create Supabase request: %w", err)
	}
	setSupabaseAuthHeaders(req, serviceKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if prefer != "" {
		req.Header.Set("Prefer", prefer)
	}

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("call Supabase: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, response.StatusCode, fmt.Errorf("read Supabase response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return responseBody, response.StatusCode, supabaseRequestError(method, table, response.Status, responseBody)
	}
	return responseBody, response.StatusCode, nil
}

// PostDashboardScheduleWithClaims posts buttons that can be handled by the
// Vercel interaction endpoint, then saves the original schedule text.
func PostDashboardScheduleWithClaims(scheduleText, destination string, scheduleDay time.Time) error {
	if strings.TrimSpace(scheduleText) == "" {
		return fmt.Errorf("schedule text is empty")
	}
	if !VercelShiftClaimsConfigured() {
		return fmt.Errorf("Vercel claims require %s, either %s or %s, and %s", supabaseURLVariable, autoshiftSupabaseKeyVar, supabaseServiceRoleKeyVar, slackSigningSecretVar)
	}

	var channelEnv string
	switch destination {
	case "test":
		channelEnv = "SLACK_TEST_CHANNEL_ID"
	case "work":
		channelEnv = "SLACK_SCHEDULE_CHANNEL_ID"
	default:
		return fmt.Errorf("invalid Slack destination")
	}
	channelID := os.Getenv(channelEnv)
	botToken := os.Getenv("SLACK_BOT_TOKEN")
	if channelID == "" || botToken == "" {
		return fmt.Errorf("%s and SLACK_BOT_TOKEN must be set", channelEnv)
	}

	renderedText, blocks := BuildDashboardScheduleBlocks(scheduleText, scheduleDay, nil)
	api := slack.New(botToken)
	postedChannel, timestamp, err := api.PostMessage(
		channelID,
		slack.MsgOptionText(renderedText, false),
		slack.MsgOptionBlocks(blocks...),
	)
	if err != nil {
		return fmt.Errorf("post schedule to Slack: %w", err)
	}

	message := DashboardScheduleMessage{
		ChannelID:    postedChannel,
		MessageTS:    timestamp,
		ScheduleDate: scheduleDay,
		SourceText:   scheduleText,
	}
	if err := SaveDashboardScheduleMessage(context.Background(), message); err != nil {
		_, _, _ = api.DeleteMessage(postedChannel, timestamp)
		return fmt.Errorf("save posted schedule for claims: %w", err)
	}
	return nil
}

// SaveDashboardScheduleMessage persists the source text used to redraw a
// Slack schedule after a claim.
func SaveDashboardScheduleMessage(ctx context.Context, message DashboardScheduleMessage) error {
	payload := map[string]string{
		"channel_id":    message.ChannelID,
		"message_ts":    message.MessageTS,
		"schedule_date": message.ScheduleDate.Format("2006-01-02"),
		"source_text":   message.SourceText,
	}
	query := url.Values{"on_conflict": {"channel_id,message_ts"}}
	_, _, err := supabaseRequest(ctx, http.MethodPost, scheduleMessagesTableName, query, payload, "resolution=merge-duplicates,return=minimal")
	return err
}

func LoadDashboardScheduleMessage(ctx context.Context, channelID, messageTS string) (DashboardScheduleMessage, error) {
	query := url.Values{
		"select":     {"channel_id,message_ts,schedule_date,source_text"},
		"channel_id": {"eq." + channelID},
		"message_ts": {"eq." + messageTS},
		"limit":      {"1"},
	}
	body, _, err := supabaseRequest(ctx, http.MethodGet, scheduleMessagesTableName, query, nil, "")
	if err != nil {
		return DashboardScheduleMessage{}, err
	}
	var rows []struct {
		ChannelID    string `json:"channel_id"`
		MessageTS    string `json:"message_ts"`
		ScheduleDate string `json:"schedule_date"`
		SourceText   string `json:"source_text"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return DashboardScheduleMessage{}, fmt.Errorf("decode stored schedule: %w", err)
	}
	if len(rows) == 0 {
		return DashboardScheduleMessage{}, fmt.Errorf("stored schedule was not found")
	}
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return DashboardScheduleMessage{}, err
	}
	day, err := time.ParseInLocation("2006-01-02", rows[0].ScheduleDate, pacific)
	if err != nil {
		return DashboardScheduleMessage{}, fmt.Errorf("parse stored schedule date: %w", err)
	}
	return DashboardScheduleMessage{ChannelID: rows[0].ChannelID, MessageTS: rows[0].MessageTS, ScheduleDate: day, SourceText: rows[0].SourceText}, nil
}

// SaveDashboardShiftClaim saves one all-or-part shift claim. The Supabase
// exclusion constraint in supabase/claims.sql rejects overlapping ranges.
func SaveDashboardShiftClaim(ctx context.Context, message DashboardScheduleMessage, claim ShiftPortionClaim) (bool, error) {
	payload := map[string]string{
		"channel_id":    message.ChannelID,
		"message_ts":    message.MessageTS,
		"open_shift_id": claim.OpenShiftID,
		"start_time":    claim.Start.UTC().Format(time.RFC3339),
		"end_time":      claim.End.UTC().Format(time.RFC3339),
		"slack_user_id": claim.SlackUserID,
	}
	_, status, err := supabaseRequest(ctx, http.MethodPost, shiftClaimsTableName, nil, payload, "return=minimal")
	if err != nil && status == http.StatusConflict {
		return false, nil
	}
	return err == nil, err
}

func LoadDashboardShiftClaims(ctx context.Context, message DashboardScheduleMessage) ([]ShiftPortionClaim, error) {
	query := url.Values{
		"select":     {"open_shift_id,start_time,end_time,slack_user_id"},
		"channel_id": {"eq." + message.ChannelID},
		"message_ts": {"eq." + message.MessageTS},
		"order":      {"start_time.asc"},
	}
	body, _, err := supabaseRequest(ctx, http.MethodGet, shiftClaimsTableName, query, nil, "")
	if err != nil {
		return nil, err
	}
	var rows []struct {
		OpenShiftID string `json:"open_shift_id"`
		Start       string `json:"start_time"`
		End         string `json:"end_time"`
		SlackUserID string `json:"slack_user_id"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode stored claims: %w", err)
	}
	claims := make([]ShiftPortionClaim, 0, len(rows))
	for _, row := range rows {
		start, err := time.Parse(time.RFC3339, row.Start)
		if err != nil {
			return nil, fmt.Errorf("parse stored claim start: %w", err)
		}
		end, err := time.Parse(time.RFC3339, row.End)
		if err != nil {
			return nil, fmt.Errorf("parse stored claim end: %w", err)
		}
		claims = append(claims, ShiftPortionClaim{OpenShiftID: row.OpenShiftID, Start: start, End: end, SlackUserID: row.SlackUserID})
	}
	return claims, nil
}

// UpdateDashboardScheduleClaims redraws the original message after a valid
// claim while leaving the corresponding Teams shift unchanged.
func UpdateDashboardScheduleClaims(ctx context.Context, message DashboardScheduleMessage) error {
	claims, err := LoadDashboardShiftClaims(ctx, message)
	if err != nil {
		return err
	}
	renderedText, blocks := BuildDashboardScheduleBlocks(message.SourceText, message.ScheduleDate, claims)
	api := slack.New(os.Getenv("SLACK_BOT_TOKEN"))
	_, _, _, err = api.UpdateMessage(message.ChannelID, message.MessageTS,
		slack.MsgOptionText(renderedText, false),
		slack.MsgOptionBlocks(blocks...),
	)
	if err != nil {
		return fmt.Errorf("update Slack schedule: %w", err)
	}
	return nil
}
