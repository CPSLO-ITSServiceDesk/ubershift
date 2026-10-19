package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/slack-go/slack"
)

type DailyScheduleMessage struct {
	Date      string `json:"date"`
	ChannelID string `json:"channel_id"`
	Timestamp string `json:"timestamp"`
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
	message, err := loadDailyScheduleMessage()
	if err != nil {
		return err
	}

	_, _, _, err = api.UpdateMessage(
		message.ChannelID,
		message.Timestamp,
		slack.MsgOptionText(scheduleText, false),
	)
	if err != nil {
		return err
	}

	log.Println("Daily schedule updated successfully")
	return nil
}
