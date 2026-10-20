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
	scheduleText, err := buildDailyScheduleText(token)
	if err != nil {
		return err
	}

	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return err
	}

	saved, err := loadDailyScheduleMessage()
	if err == nil && saved.Date == time.Now().In(pacific).Format("2006-01-02") {
		return updateDailySchedule(api, scheduleText)
	}

	postedChannel, timestamp, err := api.PostMessage(
		"C0AP09MEL9Z",
		slack.MsgOptionText(scheduleText, false),
	)
	if err != nil {
		return err
	}

	err = saveDailyScheduleMessage(DailyScheduleMessage{
		Date:      time.Now().In(pacific).Format("2006-01-02"),
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
	shifts, err := getShifts(token)
	if err != nil {
		return "", err
	}

	sortShiftsByStart(shifts)

	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return "", err
	}

	day := time.Now().In(pacific).Weekday().String()
	scheduleText := fmt.Sprintf("*%s's Schedule*\n\n", day)

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

		start, err := formatShiftTime(shift.SharedShift.StartDateTime)
		if err != nil {
			return "", err
		}

		end, err := formatShiftTime(shift.SharedShift.EndDateTime)
		if err != nil {
			return "", err
		}

		names := strings.Fields(msUser.DisplayName)
		if len(names) == 0 {
			continue
		}

		scheduleText += fmt.Sprintf("%s %s-%s\n", names[0], start, end)
	}

	return scheduleText, nil
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

	timeOffOpenShifts, err := getTimeOffRequests(token)
	if err != nil {
		log.Println("Failed to get open shifts from time off requests:", err)
	} else {
		log.Printf("Found %d Service Desk open shifts", len(timeOffOpenShifts))
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
