package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type ClaimStore struct {
	mu      sync.Mutex
	claimed map[string]string // requestID -> userID
}

var dailyOpenShiftClaims = NewClaimStore()

func NewClaimStore() *ClaimStore {
	return &ClaimStore{claimed: make(map[string]string)}
}

func (s *ClaimStore) Claim(requestID, userID string) (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.claimed[requestID]; ok {
		return false, existing
	}
	s.claimed[requestID] = userID
	return true, userID
}

func (s *ClaimStore) Claimer(requestID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, ok := s.claimed[requestID]
	return userID, ok
}

const claimDailyOpenShiftActionID = "claim_daily_open_shift"

func dailyOpenShiftID(line string) string {
	return "daily-open-shift:" + line
}

func claimedDailyOpenShiftLine(line string, claimerID string) string {
	parts := strings.SplitN(line, " --> Open Shift", 2)
	if len(parts) != 2 {
		return line
	}
	fields := strings.Fields(parts[0])
	if len(fields) < 2 {
		return line
	}
	return fmt.Sprintf("%s --> <@%s> %s", parts[0], claimerID, fields[len(fields)-1])
}

func openShiftButtonLabel(line string) string {
	parts := strings.SplitN(line, " --> Open Shift", 2)
	return "Claim " + parts[0]
}

func dailyScheduleBlocks(scheduleText string) (string, []slack.Block) {
	lines := strings.Split(strings.TrimSuffix(scheduleText, "\n"), "\n")
	renderedLines := make([]string, 0, len(lines))
	var openLines []string
	for _, line := range lines {
		if !strings.Contains(line, " --> Open Shift") {
			renderedLines = append(renderedLines, line)
			continue
		}
		requestID := dailyOpenShiftID(line)
		if claimerID, ok := dailyOpenShiftClaims.Claimer(requestID); ok {
			renderedLines = append(renderedLines, claimedDailyOpenShiftLine(line, claimerID))
			continue
		}
		renderedLines = append(renderedLines, line)
		openLines = append(openLines, line)
	}

	renderedText := strings.Join(renderedLines, "\n") + "\n"
	blocks := []slack.Block{slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", renderedText, false, false), nil, nil,
	)}
	for _, line := range openLines {
		blocks = append(blocks, slack.NewActionBlock(
			dailyOpenShiftID(line),
			slack.NewButtonBlockElement(
				claimDailyOpenShiftActionID,
				dailyOpenShiftID(line),
				slack.NewTextBlockObject("plain_text", openShiftButtonLabel(line), false, false),
			).WithStyle(slack.StylePrimary),
		))
	}
	return renderedText, blocks
}

type ShiftRequest struct {
	Name  string `json:"name"`
	Start string `json:"start"`
	End   string `json:"end"`
	Role  string `json:"role"`
}

func buildShiftBlocks(text, requestID string) []slack.Block {
	return []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", text, false, false),
			nil, nil,
		),
		slack.NewActionBlock(
			"claim_actions",
			slack.NewButtonBlockElement(
				"claim_shift",
				requestID,
				slack.NewTextBlockObject("plain_text", "Claim", false, false),
			).WithStyle(slack.StylePrimary),
		),
	}
}

func claimedBlocks(shiftText string, success bool, existingUserID, claimantUserID string) []slack.Block {
	status := ":no_entry: Already claimed by <@" + existingUserID + ">."
	if success {
		status = ":white_check_mark: Claimed by <@" + claimantUserID + ">."
	}
	return []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", shiftText, false, false),
			nil, nil,
		),
		slack.NewContextBlock(
			"claim_status",
			slack.NewTextBlockObject("mrkdwn", status, false, false),
		),
	}
}

func startHTTPServer(api *slack.Client) {
	http.HandleFunc("/new-shift", func(w http.ResponseWriter, r *http.Request) {
		var req ShiftRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		requestID := fmt.Sprintf("shift-%s-%s", req.Start, req.Role)
		shiftText := fmt.Sprintf("*Shift available*\n%s - %s\n%s", req.Start, req.End, req.Role)
		blocks := buildShiftBlocks(shiftText, requestID)

		_, _, err := api.PostMessage(
			"C0AP09MEL9Z",
			slack.MsgOptionText(shiftText, false),
			slack.MsgOptionBlocks(blocks...),
		)
		if err != nil {
			log.Println("post message error:", err)
			http.Error(w, "failed to post shift", http.StatusInternalServerError)
			return
		}
		log.Println("schedule posted to slack")
		w.WriteHeader(http.StatusOK)
	})
	go http.ListenAndServe(":8080", nil)
}

func postDailySchedule(api *slack.Client, token string) error {
	scheduleDay, err := scheduleDayForPost()
	if err != nil {
		return err
	}

	scheduleText, err := buildDailyScheduleTextForDay(token, scheduleDay)
	if err != nil {
		return err
	}

	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return err
	}

	renderedText, blocks := dailyScheduleBlocks(scheduleText)
	saved, err := loadDailyScheduleMessage()
	scheduleDate := scheduleDay.In(pacific).Format("2006-01-02")
	if err == nil && saved.Date == scheduleDate {
		return updateDailyScheduleWithBlocks(api, renderedText, blocks)
	}

	postedChannel, timestamp, err := api.PostMessage(
		"C0AP09MEL9Z",
		slack.MsgOptionText(renderedText, false),
		slack.MsgOptionBlocks(blocks...),
	)
	if err != nil {
		return err
	}

	err = saveDailyScheduleMessage(DailyScheduleMessage{
		Date:      scheduleDate,
		ChannelID: postedChannel,
		Timestamp: timestamp,
	})
	if err != nil {
		return err
	}
	log.Printf("Daily schedule posted: channel=%s, ts=%s", postedChannel, timestamp)
	return nil
}

func buildDailyScheduleText(token string) (string, error) {
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return "", err
	}
	return buildDailyScheduleTextForDay(token, time.Now().In(pacific))
}

func buildDailyScheduleTextForDay(token string, scheduleDay time.Time) (string, error) {
	shifts, err := getShiftsForDay(token, scheduleDay)
	if err != nil {
		return "", err
	}

	sortShiftsByStart(shifts)

	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return "", err
	}

	scheduleDay = scheduleDay.In(pacific)
	day := scheduleDay.Weekday().String()
	scheduleText := fmt.Sprintf("*%s's Schedule*\n\n", day)
	timeOffRequests, err := getTimeOffRequests(token)
	if err != nil {
		return "", fmt.Errorf("get time-off requests: %w", err)
	}
	timeOffRequests, err = timeOffRequestsForDay(timeOffRequests, scheduleDay)
	if err != nil {
		return "", fmt.Errorf("filter time-off requests for schedule day: %w", err)
	}
	log.Printf("Time-off requests for %s: %d", scheduleDay.Format("2006-01-02"), len(timeOffRequests))
	for _, request := range timeOffRequests {
		log.Printf(
			"Time-off request detected: id=%s state=%s userID=%s start=%s end=%s",
			request.ID,
			request.State,
			request.SenderUserID,
			request.StartDateTime,
			request.EndDateTime,
		)
	}

	for _, shift := range shifts {
		if shift.SchedulingGroupID != os.Getenv("MS_SERVICE_DESK_GROUP_ID") {
			continue
		}

		if shift.SharedShift == nil {
			continue
		}

		msUser, err := getMicrosoftUser(token, shift.UserID)
		if err != nil {
			return "", err
		}

		start, err := parseShiftTime(shift.SharedShift.StartDateTime)
		if err != nil {
			return "", err
		}

		end, err := parseShiftTime(shift.SharedShift.EndDateTime)
		if err != nil {
			return "", err
		}

		names := strings.Fields(msUser.DisplayName)
		if len(names) == 0 {
			continue
		}

		availability, err := subtractApprovedTimeOff(
			shift.UserID,
			start,
			end,
			timeOffRequests,
		)
		if err != nil {
			return "", fmt.Errorf("apply time off to Service Desk shift: %w", err)
		}
		scheduleText += formatServiceDeskAvailability(
			names[0],
			start,
			end,
			availability,
			pacific,
		)
	}

	if !isZoomCoverageDay(scheduleDay) {
		return scheduleText, nil
	}

	scheduleText += "\n" + buildZoomHeader() + "\n"
	employees, err := getAvailability(shifts, timeOffRequests)
	if err != nil {
		return "", fmt.Errorf("build Zoom availability: %w", err)
	}
	previousZoomSlots, err := loadPreviousWorkdayZoomSlots(zoomScheduleHistoryPath, scheduleDay)
	if err != nil {
		return "", err
	}

	zoomSlots := generateZoomSlots(scheduleDay)
	assignedSlots := assignZoomSlots(zoomSlots, employees, previousZoomSlots)
	for _, slot := range assignedSlots {
		zoomLine, err := formatZoomSlot(token, slot)
		if err != nil {
			return "", err
		}
		scheduleText += zoomLine + "\n"
	}

	if err := saveZoomScheduleHistory(zoomScheduleHistoryPath, scheduleDay, assignedSlots); err != nil {
		return "", err
	}
	return scheduleText, nil
}

func formatServiceDeskAvailability(
	firstName string,
	shiftStart time.Time,
	shiftEnd time.Time,
	availability []EmployeeAvailability,
	location *time.Location,
) string {
	formatRange := func(start time.Time, end time.Time) string {
		formattedStart := strings.TrimSuffix(start.In(location).Format("3:04"), ":00")
		formattedEnd := strings.TrimSuffix(end.In(location).Format("3:04"), ":00")
		return formattedStart + "-" + formattedEnd
	}

	originalRange := formatRange(shiftStart, shiftEnd)
	if len(availability) == 1 &&
		availability[0].Start.Equal(shiftStart) &&
		availability[0].End.Equal(shiftEnd) {
		return fmt.Sprintf("%s %s\n", firstName, originalRange)
	}

	if len(availability) == 0 {
		return fmt.Sprintf("%s %s --> Open Shift\n", firstName, originalRange)
	}

	availableRanges := make([]string, 0, len(availability))
	for _, window := range availability {
		availableRanges = append(availableRanges, formatRange(window.Start, window.End))
	}

	scheduleLine := fmt.Sprintf(
		"%s %s --> %s\n",
		firstName,
		originalRange,
		strings.Join(availableRanges, ", "),
	)

	// A partial time-off request leaves gaps inside the original shift. Those
	// gaps are open shifts that can receive their own Slack Claim button.
	cursor := shiftStart
	for _, window := range availability {
		if cursor.Before(window.Start) {
			scheduleLine += fmt.Sprintf(
				"%s %s --> Open Shift\n",
				firstName,
				formatRange(cursor, window.Start),
			)
		}
		cursor = window.End
	}
	if cursor.Before(shiftEnd) {
		scheduleLine += fmt.Sprintf(
			"%s %s --> Open Shift\n",
			firstName,
			formatRange(cursor, shiftEnd),
		)
	}

	return scheduleLine
}
func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using existing environment variables")
	}

	log.Printf("Test schedule flag: %q", os.Getenv("UBERSHIFT_TEST_SCHEDULE"))
	token, err := getMicrosoftToken()
	if err != nil {
		log.Fatal("Microsoft login failed:", err)
	}

	log.Println("Microsoft authentication successful")
	openShifts, err := getOpenShifts(token)
	if err != nil {
		log.Println("Failed to retrieve open shifts:", err)
	} else {
		log.Printf("Found %d Service Desk open shifts", len(openShifts))
	}
	if err := getSchedulingGroups(token); err != nil {
		log.Println("Failed to get scheduling groups: ", err)
	}

	if err := getSchedulingGroups(token); err != nil {
		log.Println("Failed to get scheduling groups: ", err)
	}

	botToken := os.Getenv("SLACK_BOT_TOKEN")
	appToken := os.Getenv("SLACK_APP_TOKEN")

	if botToken == "" {
		log.Fatal("SLACK_BOT_TOKEN is not set")
	}

	if appToken == "" {
		log.Fatal("SLACK_APP_TOKEN is not set")
	}

	api := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)
	if os.Getenv("UBERSHIFT_POST_ONCE") == "true" {
		if err := postDailySchedule(api, token); err != nil {
			log.Fatal("One-time schedule post failed:", err)
		}
		log.Println("One-time schedule post complete")
		return
	}

	store := NewClaimStore()

	startHTTPServer(api)

	client := socketmode.New(api)
	if os.Getenv("UBERSHIFT_TEST_SCHEDULE") == "true" {
		log.Println("Testing daily schedule posting")

		if err := postDailySchedule(api, token); err != nil {
			log.Println("Schedule test failed:", err)
		}
	}
	if os.Getenv("UBERSHIFT_TEST_UPDATE") == "true" {
		log.Println("Testing daily schedule update")

		err := updateDailySchedule(
			api,
			"*Monday's Schedule*\nSophia | 8-12\n*Open Shift | 8-10*\nElle | 10-3",
		)
		if err != nil {
			log.Println("Schedule update failed:", err)
		}
	}
	go startDailyScheduler(api, token)
	go startScheduleMonitor(api, token)

	go func() {
		for evt := range client.Events {
			switch evt.Type {

			case socketmode.EventTypeConnecting:
				log.Println("Connecting to Slack...")

			case socketmode.EventTypeConnected:
				log.Println("Connected to Slack")

			case socketmode.EventTypeEventsAPI:
				client.Ack(*evt.Request)

				eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok || eventsAPIEvent.Type != slackevents.CallbackEvent {
					continue
				}

				if ev, ok := eventsAPIEvent.InnerEvent.Data.(*slackevents.AppMentionEvent); ok {
					text := strings.ToLower(ev.Text)
					if strings.Contains(text, "ping") {
						api.PostMessage(
							ev.Channel,
							slack.MsgOptionText("pong", false),
						)
					}
					if strings.Contains(text, "post") {
						shiftText := "*Shift available*\n10:00 AM - 1:00 PM\nTest Shift"
						requestID := "SHFT_0a7c358f-8449-4f04-8051-ef9fcd079604"

						blocks := buildShiftBlocks(shiftText, requestID)

						api.PostMessage(
							ev.Channel,
							slack.MsgOptionText("Shift available", false),
							slack.MsgOptionBlocks(blocks...),
						)
					}
					if strings.Contains(strings.ToLower(ev.Text), "ping") {
						api.PostMessage(
							ev.Channel,
							slack.MsgOptionText("pong", false),
						)
					}
				}

			case socketmode.EventTypeInteractive:
				log.Println("Interactive event received")
				callback, ok := evt.Data.(slack.InteractionCallback)
				if !ok {
					client.Ack(*evt.Request)
					continue
				}

				client.Ack(*evt.Request)

				for _, action := range callback.ActionCallback.BlockActions {
					if action.ActionID == claimDailyOpenShiftActionID {
						success, claimerID := dailyOpenShiftClaims.Claim(action.Value, callback.User.ID)
						if !success {
							log.Printf("Daily open shift already claimed by %s", claimerID)
							continue
						}
						if len(callback.Message.Blocks.BlockSet) == 0 {
							continue
						}
						section, ok := callback.Message.Blocks.BlockSet[0].(*slack.SectionBlock)
						if !ok || section.Text == nil {
							continue
						}
						renderedText, blocks := dailyScheduleBlocks(section.Text.Text)
						_, _, _, err := api.UpdateMessage(
							callback.Channel.ID,
							callback.Message.Timestamp,
							slack.MsgOptionText(renderedText, false),
							slack.MsgOptionBlocks(blocks...),
						)
						if err != nil {
							log.Println("Update daily open shift claim failed:", err)
						}
						continue
					}
					if action.ActionID != "claim_shift" {
						continue
					}

					requestID := action.Value
					userID := callback.User.ID
					log.Println("Slack User ID:", userID)

					msUser, err := resolvesSlackUserToMicrosoft(
						api,
						token,
						userID,
						getSlackUserEmail,
						getMicrosoftUser,
					)
					if err != nil {
						log.Println("Identity lookup failed:", err)
						continue
					}

					err = assignShift(token, os.Getenv("MS_TEAM_ID"), requestID, msUser.ID)
					if err != nil {
						log.Println("Assign shift failed:", err)
						continue
					}

					log.Println("Microsoft user:", msUser.DisplayName)
					log.Println("Microsoft ID:", msUser.ID)
					success, claimerID := store.Claim(
						requestID,
						userID,
					)

					shiftText := "*Shift available*"

					if len(callback.Message.Blocks.BlockSet) > 0 {
						if sb, ok := callback.Message.Blocks.BlockSet[0].(*slack.SectionBlock); ok {
							shiftText = sb.Text.Text
						}
					}

					blocks := claimedBlocks(
						shiftText,
						success,
						claimerID,
						userID,
					)

					api.UpdateMessage(
						callback.Channel.ID,
						callback.Message.Timestamp,
						slack.MsgOptionBlocks(blocks...),
					)
				}
			}
		}
	}()

	if err := client.RunContext(context.Background()); err != nil {
		log.Fatal(err)
	}
}
