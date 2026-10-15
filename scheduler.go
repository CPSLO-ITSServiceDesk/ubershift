package main

import (
	"fmt"
	"time"

	"github.com/slack-go/slack"
)

func eightAM() (time.Time, error) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.Time{}, err
	}
	now := time.Now().In(loc)
	pacific_time := time.Date(
		now.Year(), now.Month(), now.Day(),
		8, 0, 0, 0,
		loc,
	)
	return pacific_time, nil
}

func startDailyScheduler(api *slack.Client, token string) {
	for {
		next_run, err := eightAM()
		if err != nil {
			return
		}
		if time.Now().After(next_run) {
			next_run = next_run.AddDate(0, 0, 1)
		}

		wait_duration := time.Until(next_run)

		timer := time.NewTimer(wait_duration) // starts a countdown
		defer timer.Stop()                    // timer.C
		<-timer.C                             // pauses execution until signal arrives
		fmt.Println("It's 8 AM Pacific! Time to post today's schedule.")
		err = postDailySchedule(api, token)
		if err != nil {
			fmt.Println("Failed to post daily schedule:", err)
		}
		fmt.Println("Next schedule run:", next_run)
		fmt.Println("Waiting:", wait_duration)
	}
}
