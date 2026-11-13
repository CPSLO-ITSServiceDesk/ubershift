package autoshift

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type OpenShiftSegment struct {
	Start       time.Time
	End         time.Time
	SlackUserID string
}

func splitOpenShiftByClaims(
	openStart time.Time,
	openEnd time.Time,
	claims []ShiftPortionClaim,
) []OpenShiftSegment {
	var segments []OpenShiftSegment
	cursor := openStart

	for _, claim := range claims {
		if !claim.End.After(cursor) || !claim.Start.Before(openEnd) {
			continue
		}

		claimStart := claim.Start
		if claimStart.Before(cursor) {
			claimStart = cursor
		}

		claimEnd := claim.End
		if claimEnd.After(openEnd) {
			claimEnd = openEnd
		}

		if cursor.Before(claimStart) {
			segments = append(segments, OpenShiftSegment{
				Start: cursor,
				End:   claimStart,
			})
		}

		segments = append(segments, OpenShiftSegment{
			Start:       claimStart,
			End:         claimEnd,
			SlackUserID: claim.SlackUserID,
		})

		cursor = claimEnd
	}

	if cursor.Before(openEnd) {
		segments = append(segments, OpenShiftSegment{
			Start: cursor,
			End:   openEnd,
		})
	}

	return segments
}

func formatOpenShiftSegments(
	firstName string,
	segments []OpenShiftSegment,
	location *time.Location,
) string {
	var text strings.Builder

	for _, segment := range segments {
		start := strings.TrimSuffix(segment.Start.In(location).Format("3:04"), ":00")
		end := strings.TrimSuffix(segment.End.In(location).Format("3:04"), ":00")
		timeRange := start + "-" + end

		if segment.SlackUserID == "" {
			fmt.Fprintf(&text, "%s %s --> Open Shift\n", firstName, timeRange)
			continue
		}

		fmt.Fprintf(
			&text,
			"%s %s --> <@%s> %s\n",
			firstName,
			timeRange,
			segment.SlackUserID,
			timeRange,
		)
	}

	return text.String()
}

func formatOpenShiftSegment(
	firstName string,
	segment OpenShiftSegment,
	location *time.Location,
) string {
	return formatOpenShiftSegments(firstName, []OpenShiftSegment{segment}, location)
}

func strikeOutOpenShiftLine(line string) string {
	parts := strings.SplitN(line, " -->", 2)
	if len(parts) != 2 {
		return line
	}
	return "~" + strings.ReplaceAll(parts[0], "~", "") + "~ -->" + parts[1]
}

func parseOpenShiftLine(
	line string,
	day time.Time,
) (string, time.Time, time.Time, bool) {
	parts := strings.SplitN(line, " --> Open Shift", 2)
	if len(parts) != 2 {
		return "", time.Time{}, time.Time{}, false
	}

	fields := strings.Fields(strings.ReplaceAll(parts[0], "~", ""))
	if len(fields) != 2 {
		return "", time.Time{}, time.Time{}, false
	}

	times := strings.SplitN(fields[1], "-", 2)
	if len(times) != 2 {
		return "", time.Time{}, time.Time{}, false
	}

	start, ok := parseOpenShiftTime(day, times[0])
	if !ok {
		return "", time.Time{}, time.Time{}, false
	}

	end, ok := parseOpenShiftTime(day, times[1])
	if !ok || !start.Before(end) {
		return "", time.Time{}, time.Time{}, false
	}

	return fields[0], start, end, true
}

func parseOpenShiftTime(day time.Time, value string) (time.Time, bool) {
	parts := strings.SplitN(value, ":", 2)

	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 1 || hour > 12 {
		return time.Time{}, false
	}

	minute := 0
	if len(parts) == 2 {
		minute, err = strconv.Atoi(parts[1])
		if err != nil || minute < 0 || minute > 59 {
			return time.Time{}, false
		}
	}

	// Service Desk displayed hours 1-7 represent afternoon hours.
	if hour >= 1 && hour <= 7 {
		hour += 12
	}

	return time.Date(
		day.Year(), day.Month(), day.Day(),
		hour, minute, 0, 0,
		day.Location(),
	), true
}
