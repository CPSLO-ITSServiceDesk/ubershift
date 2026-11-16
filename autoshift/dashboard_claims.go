package autoshift

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

// DashboardScheduleMessage identifies one Slack message whose source schedule
// is stored outside Vercel's temporary filesystem.
type DashboardScheduleMessage struct {
	ChannelID    string    `json:"channel_id"`
	MessageTS    string    `json:"message_ts"`
	ScheduleDate time.Time `json:"schedule_date"`
	SourceText   string    `json:"source_text"`
}

// BuildDashboardScheduleBlocks renders claim buttons using durable claims. It
// does not change Teams assignments.
func BuildDashboardScheduleBlocks(
	scheduleText string,
	scheduleDay time.Time,
	claims []ShiftPortionClaim,
) (string, []slack.Block) {
	claimsByOpenShift := make(map[string][]ShiftPortionClaim)
	for _, claim := range claims {
		claimsByOpenShift[claim.OpenShiftID] = append(claimsByOpenShift[claim.OpenShiftID], claim)
	}

	lines := strings.Split(strings.TrimSuffix(scheduleText, "\n"), "\n")
	renderedLines := make([]string, 0, len(lines))
	var openShifts []claimableOpenShift
	pacific := scheduleDay.Location()

	for _, line := range lines {
		if !strings.Contains(line, " --> Open Shift") {
			renderedLines = append(renderedLines, line)
			continue
		}

		requestID := dailyOpenShiftIDForDay(line, scheduleDay)
		firstName, start, end, parsed := parseOpenShiftLine(line, scheduleDay)
		if !parsed {
			renderedLines = append(renderedLines, line)
			continue
		}

		for _, segment := range splitOpenShiftByClaims(start, end, claimsByOpenShift[requestID]) {
			segmentLine := strings.TrimSuffix(formatOpenShiftSegment(firstName, segment, pacific), "\n")
			if strings.Contains(line, "~") {
				segmentLine = strikeOutOpenShiftLine(segmentLine)
			}
			renderedLines = append(renderedLines, segmentLine)
			if segment.SlackUserID == "" {
				openShifts = append(openShifts, claimableOpenShift{
					line: segmentLine,
					request: portionClaimRequest{
						OpenShiftID: requestID,
						FirstName:   firstName,
						Start:       segment.Start.UTC().Format(time.RFC3339),
						End:         segment.End.UTC().Format(time.RFC3339),
					},
					buttonID: requestID + ":" + segment.Start.UTC().Format(time.RFC3339),
				})
			}
		}
	}

	renderedText := strings.Join(renderedLines, "\n") + "\n"
	blocks := []slack.Block{slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", renderedText, false, false), nil, nil,
	)}
	for _, openShift := range openShifts {
		value, err := json.Marshal(openShift.request)
		if err != nil {
			continue
		}
		blocks = append(blocks, slack.NewActionBlock(
			openShift.buttonID,
			slack.NewButtonBlockElement(
				claimDailyOpenShiftActionID,
				string(value),
				slack.NewTextBlockObject("plain_text", openShiftButtonLabel(openShift.line), false, false),
			).WithStyle(slack.StylePrimary),
		))
	}
	return renderedText, blocks
}

// DashboardClaimModal creates the Slack modal for claiming an entire open
// shift or a selected portion of it.
func DashboardClaimModal(actionValue, channelID, messageTS string) (slack.ModalViewRequest, error) {
	var request portionClaimRequest
	if err := json.Unmarshal([]byte(actionValue), &request); err != nil {
		return slack.ModalViewRequest{}, fmt.Errorf("decode claim action: %w", err)
	}
	request.ChannelID = channelID
	request.MessageTS = messageTS
	return portionClaimModal(request)
}

// DashboardClaimFromView validates a submitted claim modal and returns its
// requested time range along with the originating Slack schedule message.
func DashboardClaimFromView(callback slack.InteractionCallback) (ShiftPortionClaim, DashboardScheduleMessage, error) {
	var request portionClaimRequest
	if err := json.Unmarshal([]byte(callback.View.PrivateMetadata), &request); err != nil {
		return ShiftPortionClaim{}, DashboardScheduleMessage{}, fmt.Errorf("claim metadata is invalid")
	}
	if request.ChannelID == "" || request.MessageTS == "" {
		return ShiftPortionClaim{}, DashboardScheduleMessage{}, fmt.Errorf("claim message is missing")
	}
	allowedStart, err := time.Parse(time.RFC3339, request.Start)
	if err != nil {
		return ShiftPortionClaim{}, DashboardScheduleMessage{}, fmt.Errorf("claim start is invalid")
	}
	allowedEnd, err := time.Parse(time.RFC3339, request.End)
	if err != nil || !allowedStart.Before(allowedEnd) {
		return ShiftPortionClaim{}, DashboardScheduleMessage{}, fmt.Errorf("claim end is invalid")
	}
	start, err := portionClaimTimeFromView(callback, portionClaimStartBlockID, portionClaimStartActionID)
	if err != nil {
		return ShiftPortionClaim{}, DashboardScheduleMessage{}, fmt.Errorf("choose a start time")
	}
	end, err := portionClaimTimeFromView(callback, portionClaimEndBlockID, portionClaimEndActionID)
	if err != nil || !start.Before(end) {
		return ShiftPortionClaim{}, DashboardScheduleMessage{}, fmt.Errorf("end time must be after start time")
	}
	if start.Before(allowedStart) || end.After(allowedEnd) {
		return ShiftPortionClaim{}, DashboardScheduleMessage{}, fmt.Errorf("choose a time inside the available Open Shift")
	}
	if callback.User.ID == "" {
		return ShiftPortionClaim{}, DashboardScheduleMessage{}, fmt.Errorf("Slack user is missing")
	}
	return ShiftPortionClaim{
		OpenShiftID: request.OpenShiftID,
		Start:       start,
		End:         end,
		SlackUserID: callback.User.ID,
	}, DashboardScheduleMessage{ChannelID: request.ChannelID, MessageTS: request.MessageTS}, nil
}
