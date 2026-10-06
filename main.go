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

	if err := getShifts(token); err != nil {
		log.Fatal("Failed to get shifts", err)
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
					if strings.Contains(strings.ToLower(ev.Text), "ping") {
						api.PostMessage(
							ev.Channel,
							slack.MsgOptionText("pong", false),
						)
					}
				}

			case socketmode.EventTypeInteractive:
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
