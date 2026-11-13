package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/slack-go/slack"
)

type DailyScheduleMessage struct {
	Date       string `json:"date"`
	ChannelID  string `json:"channel_id"`
	Timestamp  string `json:"timestamp"`
	SourceText string `json:"source_text,omitempty"`
}

// stores the data, Slack channel ID, and message timestamp in a local JSON file
func saveDailyScheduleMessage(message DailyScheduleMessage) error {
	data, err := json.MarshalIndent(message, "", " ")
	if err != nil {
		return err
	}

	return os.WriteFile(".daily_schedule.json", data, 0600)
}

func loadDailyScheduleMessage() (DailyScheduleMessage, error) {
	var message DailyScheduleMessage

	data, err := os.ReadFile(".daily_schedule.json")
	if err != nil {
		return message, err
	}

	err = json.Unmarshal(data, &message)
	if err != nil {
		return message, err
	}

	return message, nil
}

func updateDailySchedule(api *slack.Client, scheduleText string) error {
	return updateDailyScheduleWithBlocks(api, scheduleText, nil)
}

func updateDailyScheduleWithBlocks(api *slack.Client, scheduleText string, blocks []slack.Block) error {
	message, err := loadDailyScheduleMessage()
	if err != nil {
		return err
	}

	options := []slack.MsgOption{slack.MsgOptionText(scheduleText, false)}
	if len(blocks) > 0 {
		options = append(options, slack.MsgOptionBlocks(blocks...))
	}
	_, _, _, err = api.UpdateMessage(message.ChannelID, message.Timestamp, options...)
	if err != nil {
		return err
	}

	log.Println("Daily schedule updated successfully")
	return nil
}

// updateDailyScheduleFromStoredSource redraws the Slack schedule from the
// last Teams-derived source text plus local claim data, without waiting on
// Microsoft Graph.
func updateDailyScheduleFromStoredSource(api *slack.Client) error {
	message, err := loadDailyScheduleMessage()
	if err != nil {
		return err
	}
	if message.SourceText == "" {
		return fmt.Errorf("daily schedule source text is unavailable")
	}
	renderedText, blocks := dailyScheduleBlocks(message.SourceText)
	return updateDailyScheduleWithBlocks(api, renderedText, blocks)
}
