package autoshift

import (
	"fmt"
	"os"
	"time"

	"github.com/slack-go/slack"
)

func schedulePostTime() (time.Time, error) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.Time{}, err
	}
	return schedulePostTimeAt(time.Now().In(loc), loc), nil
}

func nextScheduleDay() (time.Time, error) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.Time{}, err
	}
	return scheduleDayAt(time.Now().In(loc), loc), nil
}

// scheduleDayForPost uses an explicitly requested weekday when one is set;
// otherwise it uses today, or Monday during the weekend.
func scheduleDayForPost() (time.Time, error) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.Time{}, err
	}
	return scheduleDayForPostAt(
		time.Now().In(loc),
		os.Getenv("UBERSHIFT_SCHEDULE_DATE"),
		loc,
	)
}

func scheduleDayForPostAt(now time.Time, requestedDate string, loc *time.Location) (time.Time, error) {
	if requestedDate == "" {
		return scheduleDayAt(now, loc), nil
	}

	day, err := time.ParseInLocation("2006-01-02", requestedDate, loc)
	if err != nil || day.Format("2006-01-02") != requestedDate {
		return time.Time{}, fmt.Errorf("UBERSHIFT_SCHEDULE_DATE must use YYYY-MM-DD, got %q", requestedDate)
	}
	if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
		return time.Time{}, fmt.Errorf("UBERSHIFT_SCHEDULE_DATE must be Monday through Friday, got %s", day.Weekday())
	}
	return day, nil
}

func scheduleDayAt(now time.Time, loc *time.Location) time.Time {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	for day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
		day = day.AddDate(0, 0, 1)
	}
	return day
}

func schedulePostTimeAt(now time.Time, loc *time.Location) time.Time {
	nextRun := scheduleDayAt(now, loc).Add(7*time.Hour + 30*time.Minute)
	if now.After(nextRun) {
		nextRun = nextRun.AddDate(0, 0, 1)
	}

	for nextRun.Weekday() == time.Saturday || nextRun.Weekday() == time.Sunday {
		nextRun = nextRun.AddDate(0, 0, 1)
	}
	return nextRun
}

func startDailyScheduler(api *slack.Client, token string) {
	for {
		next_run, err := schedulePostTime()
		if err != nil {
			return
		}
		wait_duration := time.Until(next_run)

		timer := time.NewTimer(wait_duration) // starts a countdown
		defer timer.Stop()                    // timer.C
		<-timer.C                             // pauses execution until signal arrives
		fmt.Println("It's 7:30 AM Pacific! Time to post today's schedule.")
		err = postDailySchedule(api, token)
		if err != nil {
			fmt.Println("Failed to post daily schedule:", err)
		}
		fmt.Println("Next schedule run:", next_run)
		fmt.Println("Waiting:", wait_duration)
	}
}
