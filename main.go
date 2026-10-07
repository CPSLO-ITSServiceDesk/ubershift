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
			slack.MsgOptionText("Shift available", false),
			slack.MsgOptionBlocks(blocks...),
		)
		if err != nil {
			log.Println("post message error:", err)
			http.Error(w, "failed to post shift", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	go http.ListenAndServe(":8080", nil)
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using existing environment variables")
	}
	token, err := getMicrosoftToken()
	if err != nil {
		log.Fatal("Microsoft login failed:", err)
	}

	log.Println("Microsoft authentication successful")

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

	shifts, err := getShifts(token)
	if err != nil {
		log.Fatal("Failed to get shifts", err)
	}

	if len(shifts) > 0 {
		firstShift := shifts[0]
		if firstShift.SharedShift != nil {

			shiftText := fmt.Sprintf(
				"*Shift available*\n%s - %s",
				firstShift.SharedShift.StartDateTime,
				firstShift.SharedShift.EndDateTime,
			)
			blocks := buildShiftBlocks(
				shiftText,
				firstShift.ID,
			)
			_, _, err = api.PostMessage(
				"C0AP09MEL9Z",
				slack.MsgOptionText("Shift available", false),
				slack.MsgOptionBlocks(blocks...),
			)
			if err != nil {
				log.Println("Failed to post real shift:", err)
			}
		}
		log.Println("First real shift ID:", firstShift.ID)
	}

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
