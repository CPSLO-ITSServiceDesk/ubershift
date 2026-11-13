package main

import (
	"fmt"
	"log"
	"time"

	"github.com/slack-go/slack"
)

func startScheduleMonitor(api *slack.Client, token string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Only update an existing daily schedule
		saved, err := loadDailyScheduleMessage()
		if err != nil {
			log.Println("Cannot load saved schedule:", err)
			continue
		}

		pacific, err := time.LoadLocation("America/Los_Angeles")
		if err != nil {
			log.Println("Timezone error:", err)
			continue
		}

		today := time.Now().In(pacific).Format("2006-01-02")

		log.Println("Saved schedule date:", saved.Date)
		log.Println("Today's date:", today)

		if saved.Date != today {
			continue
		}

		log.Println("Checking Teams schedule for changes")

		if err := refreshDailySchedule(api, token); err != nil {
			log.Println("Schedule refresh failed:", err)
		}
	}
}

func refreshDailySchedule(api *slack.Client, token string) error {
	// generate the latest schedule from MS Teams
	scheduleText, err := buildDailyScheduleText(token)
	if err != nil {
		return err
	}

	// load the original Slack message
	saved, err := loadDailyScheduleMessage()
	if err != nil {
		return err
	}

	// retrieve the current message from Slack
	messages, _, _, err := api.GetConversationReplies(&slack.GetConversationRepliesParameters{
		ChannelID: saved.ChannelID,
		Timestamp: saved.Timestamp,
		Limit:     1,
	})
	if err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("original schedule message not found")
	}

	// only update Slack if the schedule changed
	renderedText, blocks := dailyScheduleBlocks(scheduleText)
	if messages[0].Text == renderedText {
		log.Println("No schedule changes detected")
		if saved.SourceText != scheduleText {
			saved.SourceText = scheduleText
			return saveDailyScheduleMessage(saved)
		}
		return nil
	}

	log.Println("Schedule change detected, updating Slack")
	if err := updateDailyScheduleWithBlocks(api, renderedText, blocks); err != nil {
		return err
	}
	saved.SourceText = scheduleText
	return saveDailyScheduleMessage(saved)
}
