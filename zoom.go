package autoshift

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

type ZoomShift struct {
	UserID string
	Start  time.Time
	End    time.Time
}

type EmployeeAvailability struct {
	UserID string
	Start  time.Time
	End    time.Time
}

// returns true only if the student assistant's Service Desk
// shift covers the entire proposed Zoom assignment
func isAvailable(shiftStart, shiftEnd, zoomStart, zoomEnd time.Time) bool {
	return !shiftStart.After(zoomStart) && !shiftEnd.Before(zoomEnd)
}

func getAvailability(
	shifts []Shift,
	timeOffRequests []TimeOffRequest,
) ([]EmployeeAvailability, error) {
	var employees []EmployeeAvailability
	excludedUserIDs := zoomExcludedUserIDs()

	for _, shift := range shifts {
		if shift.SharedShift == nil {
			continue
		}

		if shift.SchedulingGroupID != os.Getenv("MS_SERVICE_DESK_GROUP_ID") {
			continue
		}

		if excludedUserIDs[shift.UserID] {
			continue
		}

		start, err := parseShiftTime(shift.SharedShift.StartDateTime)
		if err != nil {
			continue
		}

		end, err := parseShiftTime(shift.SharedShift.EndDateTime)
		if err != nil {
			continue
		}

		availability, err := subtractApprovedTimeOff(
			shift.UserID,
			start,
			end,
			timeOffRequests,
		)
		if err != nil {
			return nil, err
		}
		employees = append(employees, availability...)
	}

	return employees, nil
}

// subtractApprovedTimeOff splits a Service Desk shift into the time ranges
// during which the employee is still available for Zoom coverage.
func subtractApprovedTimeOff(
	userID string,
	shiftStart time.Time,
	shiftEnd time.Time,
	timeOffRequests []TimeOffRequest,
) ([]EmployeeAvailability, error) {
	available := []EmployeeAvailability{{
		UserID: userID,
		Start:  shiftStart,
		End:    shiftEnd,
	}}

	for _, request := range timeOffRequests {
		if request.SenderUserID != userID ||
			!strings.EqualFold(request.State, "approved") {
			continue
		}

		timeOffStart, err := parseShiftTime(request.StartDateTime)
		if err != nil {
			return nil, fmt.Errorf("parse approved time-off start: %w", err)
		}
		timeOffEnd, err := parseShiftTime(request.EndDateTime)
		if err != nil {
			return nil, fmt.Errorf("parse approved time-off end: %w", err)
		}

		var remaining []EmployeeAvailability
		for _, window := range available {
			// No overlap: preserve the complete availability window.
			if !timeOffStart.Before(window.End) || !timeOffEnd.After(window.Start) {
				remaining = append(remaining, window)
				continue
			}

			if window.Start.Before(timeOffStart) {
				remaining = append(remaining, EmployeeAvailability{
					UserID: userID,
					Start:  window.Start,
					End:    timeOffStart,
				})
			}
			if window.End.After(timeOffEnd) {
				remaining = append(remaining, EmployeeAvailability{
					UserID: userID,
					Start:  timeOffEnd,
					End:    window.End,
				})
			}
		}
		available = remaining
	}

	// Teams can return time-off boundaries with seconds. Do not display or
	// assign a meaningless seconds-long availability fragment as "2-2".
	const minimumAvailability = time.Minute
	meaningful := available[:0]
	for _, window := range available {
		if window.End.Sub(window.Start) >= minimumAvailability {
			meaningful = append(meaningful, window)
		}
	}

	return meaningful, nil
}

// zoomExcludedUserIDs returns the Microsoft Object IDs that must never receive
// Zoom coverage, such as Service Desk leads.
//
// Set MS_ZOOM_EXCLUDED_USER_IDS in .env to a comma-separated list of IDs.
func zoomExcludedUserIDs() map[string]bool {
	excluded := make(map[string]bool)

	for _, userID := range strings.Split(os.Getenv("MS_ZOOM_EXCLUDED_USER_IDS"), ",") {
		userID = strings.TrimSpace(userID)
		if userID != "" {
			excluded[userID] = true
		}
	}

	return excluded
}
func findAvailabilityEmployees(
	employees []EmployeeAvailability,
	zoomStart time.Time,
	zoomEnd time.Time,
) []EmployeeAvailability {

	var available []EmployeeAvailability

	for _, employee := range employees {
		if isAvailable(
			employee.Start,
			employee.End,
			zoomStart,
			zoomEnd,
		) {
			available = append(available, employee)
		}
	}
	return available
}

func generateZoomSlots(day time.Time) []ZoomShift {
	var slots []ZoomShift

	start := time.Date(
		day.Year(), day.Month(), day.Day(),
		8, 0, 0, 0, day.Location(),
	)

	durations := []time.Duration{
		2 * time.Hour,
		2 * time.Hour,
		1 * time.Hour,
		2 * time.Hour,
		2 * time.Hour,
	}

	current := start
	for _, duration := range durations {
		slots = append(slots, ZoomShift{
			Start: current,
			End:   current.Add(duration),
		})
		current = current.Add(duration)
	}
	return slots
}

func assignZoomSlots(
	slots []ZoomShift,
	employees []EmployeeAvailability,
	yesterday []ZoomShift,
) []ZoomShift {
	assignedTime := make(map[string]time.Duration)

	for i := 0; i < len(slots); i++ {
		if shouldSplitZoomSlot(slots[i], employees, assignedTime) {
			splitSlots := splitZoomSlotAtAvailabilityBoundaries(slots[i], employees)
			updatedSlots := make([]ZoomShift, 0, len(slots)-1+len(splitSlots))
			updatedSlots = append(updatedSlots, slots[:i]...)
			updatedSlots = append(updatedSlots, splitSlots...)
			updatedSlots = append(updatedSlots, slots[i+1:]...)
			slots = updatedSlots
			i--
			continue
		}

		available := findAvailabilityEmployees(
			employees,
			slots[i].Start,
			slots[i].End,
		)

		if len(available) == 0 {
			slots[i].UserID = "Unassigned"
			continue
		}

		selected := ""

		// Prefer employees who did not work this time yesterday.
		for _, employee := range available {
			if workedYesterday(
				employee.UserID,
				slots[i].Start,
				slots[i].End,
				yesterday,
			) {
				continue
			}

			if selected == "" ||
				assignedTime[employee.UserID] < assignedTime[selected] {
				selected = employee.UserID
			}
		}

		// If everyone worked this time yesterday,
		// select the employee with the fewest assigned hours.
		if selected == "" {
			selected = available[0].UserID

			for _, employee := range available {
				if assignedTime[employee.UserID] < assignedTime[selected] {
					selected = employee.UserID
				}
			}
		}

		slots[i].UserID = selected
		assignedTime[selected] += slots[i].End.Sub(slots[i].Start)
	}

	return slots
}

func shouldSplitZoomSlot(
	slot ZoomShift,
	employees []EmployeeAvailability,
	assignedTime map[string]time.Duration,
) bool {
	fullBlockCandidates := findAvailabilityEmployees(employees, slot.Start, slot.End)
	splitSlots := splitZoomSlotAtAvailabilityBoundaries(slot, employees)
	if len(splitSlots) <= 1 {
		return false
	}

	if len(fullBlockCandidates) == 0 {
		for _, splitSlot := range splitSlots {
			if len(findAvailabilityEmployees(employees, splitSlot.Start, splitSlot.End)) > 0 {
				return true
			}
		}
		return false
	}

	lowestFullBlockTime := assignedTime[fullBlockCandidates[0].UserID]
	for _, employee := range fullBlockCandidates[1:] {
		if assignedTime[employee.UserID] < lowestFullBlockTime {
			lowestFullBlockTime = assignedTime[employee.UserID]
		}
	}

	if lowestFullBlockTime == 0 {
		return false
	}

	for _, splitSlot := range splitSlots {
		hasLowerLoadedCandidate := false
		for _, employee := range findAvailabilityEmployees(employees, splitSlot.Start, splitSlot.End) {
			if assignedTime[employee.UserID] < lowestFullBlockTime {
				hasLowerLoadedCandidate = true
				break
			}
		}
		if !hasLowerLoadedCandidate {
			return false
		}
	}

	return true
}

func splitZoomSlotAtAvailabilityBoundaries(
	slot ZoomShift,
	employees []EmployeeAvailability,
) []ZoomShift {
	boundaries := []time.Time{slot.Start, slot.End}

	for _, employee := range employees {
		if employee.Start.After(slot.Start) && employee.Start.Before(slot.End) {
			boundaries = append(boundaries, employee.Start)
		}
		if employee.End.After(slot.Start) && employee.End.Before(slot.End) {
			boundaries = append(boundaries, employee.End)
		}
	}

	sort.Slice(boundaries, func(i, j int) bool {
		return boundaries[i].Before(boundaries[j])
	})

	var splitSlots []ZoomShift
	for i := 0; i < len(boundaries)-1; i++ {
		if boundaries[i].Equal(boundaries[i+1]) {
			continue
		}
		splitSlots = append(splitSlots, ZoomShift{
			Start: boundaries[i],
			End:   boundaries[i+1],
		})
	}

	return splitSlots
}

func workedYesterday(
	userID string,
	start time.Time,
	end time.Time,
	yesterday []ZoomShift,
) bool {
	startMinute := start.Hour()*60 + start.Minute()
	endMinute := end.Hour()*60 + end.Minute()

	for _, shift := range yesterday {
		if shift.UserID != userID {
			continue
		}

		shiftStartMinute := shift.Start.Hour()*60 + shift.Start.Minute()
		shiftEndMinute := shift.End.Hour()*60 + shift.End.Minute()

		// Compare clock times rather than dates.
		if startMinute < shiftEndMinute && endMinute > shiftStartMinute {
			return true
		}
	}
	return false
}

func isZoomCoverageDay(day time.Time) bool {
	return day.Weekday() != time.Saturday && day.Weekday() != time.Sunday
}

func buildZoomHeader() string {
	zoom_url := os.Getenv("ZOOM_MEETING_URL")
	if zoom_url == "" {
		return "Zoom:"
	}

	return fmt.Sprintf("<%s|Zoom>:", zoom_url)
}

func formatZoomSlot(token string, slot ZoomShift) (string, error) {
	start := strings.TrimSuffix(slot.Start.Format("3:04"), ":00")
	end := strings.TrimSuffix(slot.End.Format("3:04"), ":00")
	if slot.UserID == "Unassigned" {
		return fmt.Sprintf("%s-%s Unassigned", start, end), nil
	}

	ms_user, err := getMicrosoftUser(token, slot.UserID)
	if err != nil {
		return "", err
	}

	names := strings.Fields(ms_user.DisplayName)
	if len(names) == 0 {
		return "", fmt.Errorf("Microsoft user has no display name")
	}

	first_name := names[0]
	return fmt.Sprintf("%s-%s @%s", start, end, first_name), nil
}
