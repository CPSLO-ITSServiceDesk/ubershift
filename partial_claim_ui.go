package autoshift

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

const (
	portionClaimModalCallbackID = "claim_open_shift_portion"
	portionClaimStartBlockID    = "portion_claim_start"
	portionClaimEndBlockID      = "portion_claim_end"
	portionClaimStartActionID   = "portion_claim_start_time"
	portionClaimEndActionID     = "portion_claim_end_time"
)

type portionClaimRequest struct {
	OpenShiftID string `json:"open_shift_id"`
	FirstName   string `json:"first_name"`
	Start       string `json:"start"`
	End         string `json:"end"`
}

type claimableOpenShift struct {
	line     string
	request  portionClaimRequest
	buttonID string
}

func dailyScheduleBlocksWithPortionClaims(scheduleText string, claimsDB *sql.DB) (string, []slack.Block) {
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		pacific = time.Local
	}
	day := time.Now().In(pacific)
	lines := strings.Split(strings.TrimSuffix(scheduleText, "\n"), "\n")
	renderedLines := make([]string, 0, len(lines))
	var openShifts []claimableOpenShift

	for _, line := range lines {
		if !strings.Contains(line, " --> Open Shift") {
			renderedLines = append(renderedLines, line)
			continue
		}

		requestID := dailyOpenShiftID(line)
		firstName, start, end, parsed := parseOpenShiftLine(line, day)
		if claimsDB != nil && parsed {
			claims, err := loadPortionClaims(claimsDB, requestID)
			if err != nil {
				log.Printf("Load portion claims failed: %v", err)
			} else if len(claims) > 0 {
				for _, segment := range splitOpenShiftByClaims(start, end, claims) {
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
				continue
			}
		}

		if claimerID, ok := dailyOpenShiftClaims.Claimer(requestID); ok {
			renderedLines = append(renderedLines, claimedDailyOpenShiftLine(line, claimerID))
			continue
		}
		renderedLines = append(renderedLines, line)
		if parsed {
			openShifts = append(openShifts, claimableOpenShift{
				line: line,
				request: portionClaimRequest{
					OpenShiftID: requestID,
					FirstName:   firstName,
					Start:       start.UTC().Format(time.RFC3339),
					End:         end.UTC().Format(time.RFC3339),
				},
				buttonID: requestID,
			})
		}
	}

	renderedText := strings.Join(renderedLines, "\n") + "\n"
	blocks := []slack.Block{slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", renderedText, false, false), nil, nil,
	)}
	for _, openShift := range openShifts {
		value, err := json.Marshal(openShift.request)
		if err != nil {
			log.Printf("Encode portion claim request failed: %v", err)
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

func portionClaimModal(request portionClaimRequest) (slack.ModalViewRequest, error) {
	start, err := time.Parse(time.RFC3339, request.Start)
	if err != nil {
		return slack.ModalViewRequest{}, err
	}
	end, err := time.Parse(time.RFC3339, request.End)
	if err != nil || !start.Before(end) {
		return slack.ModalViewRequest{}, fmt.Errorf("invalid claim range")
	}
	metadata, err := json.Marshal(request)
	if err != nil {
		return slack.ModalViewRequest{}, err
	}
	startOptions := portionClaimTimeOptions(start, end, false)
	endOptions := portionClaimTimeOptions(start, end, true)

	startSelect := slack.NewOptionsSelectBlockElement(slack.OptTypeStatic,
		slack.NewTextBlockObject("plain_text", "Select start", false, false),
		portionClaimStartActionID, startOptions...).WithInitialOption(startOptions[0])
	endSelect := slack.NewOptionsSelectBlockElement(slack.OptTypeStatic,
		slack.NewTextBlockObject("plain_text", "Select end", false, false),
		portionClaimEndActionID, endOptions...).WithInitialOption(endOptions[len(endOptions)-1])

	return slack.ModalViewRequest{
		Type:            slack.VTModal,
		CallbackID:      portionClaimModalCallbackID,
		PrivateMetadata: string(metadata),
		Title:           slack.NewTextBlockObject("plain_text", "Claim Shift", false, false),
		Close:           slack.NewTextBlockObject("plain_text", "Cancel", false, false),
		Submit:          slack.NewTextBlockObject("plain_text", "Claim", false, false),
		Blocks: slack.Blocks{BlockSet: []slack.Block{
			slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("Claim all or part of *%s*'s open shift. The full available range is selected by default.", request.FirstName), false, false), nil, nil),
			slack.NewInputBlock(portionClaimStartBlockID, slack.NewTextBlockObject("plain_text", "Start time", false, false), nil, startSelect),
			slack.NewInputBlock(portionClaimEndBlockID, slack.NewTextBlockObject("plain_text", "End time", false, false), nil, endSelect),
		}},
	}, nil
}

func portionClaimTimeOptions(start, end time.Time, isEnd bool) []*slack.OptionBlockObject {
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		pacific = start.Location()
	}
	first := start
	if isEnd {
		first = start.Add(time.Hour)
	}
	var options []*slack.OptionBlockObject
	for value := first; !value.After(end); value = value.Add(time.Hour) {
		options = append(options, slack.NewOptionBlockObject(value.UTC().Format(time.RFC3339),
			slack.NewTextBlockObject("plain_text", value.In(pacific).Format("3 PM"), false, false), nil))
	}
	return options
}

func portionClaimTimeFromView(callback slack.InteractionCallback, blockID, actionID string) (time.Time, error) {
	if callback.View.State == nil {
		return time.Time{}, fmt.Errorf("claim form state is missing")
	}
	action, ok := callback.View.State.Values[blockID][actionID]
	if !ok || action.SelectedOption.Value == "" {
		return time.Time{}, fmt.Errorf("claim time is missing")
	}
	return time.Parse(time.RFC3339, action.SelectedOption.Value)
}

func saveSubmittedPortionClaim(db *sql.DB, callback slack.InteractionCallback) (bool, *slack.ViewSubmissionResponse, error) {
	var request portionClaimRequest
	if err := json.Unmarshal([]byte(callback.View.PrivateMetadata), &request); err != nil {
		return false, slack.NewErrorsViewSubmissionResponse(map[string]string{
			portionClaimStartBlockID: "This claim is no longer valid. Please open it again.",
		}), nil
	}
	allowedStart, err := time.Parse(time.RFC3339, request.Start)
	if err != nil {
		return false, nil, err
	}
	allowedEnd, err := time.Parse(time.RFC3339, request.End)
	if err != nil {
		return false, nil, err
	}
	start, err := portionClaimTimeFromView(callback, portionClaimStartBlockID, portionClaimStartActionID)
	if err != nil {
		return false, slack.NewErrorsViewSubmissionResponse(map[string]string{
			portionClaimStartBlockID: "Choose a start time.",
		}), nil
	}
	end, err := portionClaimTimeFromView(callback, portionClaimEndBlockID, portionClaimEndActionID)
	if err != nil {
		return false, slack.NewErrorsViewSubmissionResponse(map[string]string{
			portionClaimEndBlockID: "Choose an end time.",
		}), nil
	}
	if !start.Before(end) {
		return false, slack.NewErrorsViewSubmissionResponse(map[string]string{
			portionClaimEndBlockID: "End time must be after the start time.",
		}), nil
	}
	if start.Before(allowedStart) || end.After(allowedEnd) {
		return false, slack.NewErrorsViewSubmissionResponse(map[string]string{
			portionClaimEndBlockID: "Choose a time inside the available Open Shift.",
		}), nil
	}

	saved, err := savePortionClaim(db, ShiftPortionClaim{
		OpenShiftID: request.OpenShiftID,
		Start:       start,
		End:         end,
		SlackUserID: callback.User.ID,
	})
	if err != nil {
		return false, nil, err
	}
	if !saved {
		return false, slack.NewErrorsViewSubmissionResponse(map[string]string{
			portionClaimEndBlockID: "That time overlaps a claim that was just submitted. Choose another range.",
		}), nil
	}
	return true, slack.NewClearViewSubmissionResponse(), nil
}
