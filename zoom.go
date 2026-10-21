package main

import (
	"fmt"
	"os"
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

func getAvailability(shifts []Shift) []EmployeeAvailability {
	var employees []EmployeeAvailability

	for _, shift := range shifts {
		if shift.SharedShift == nil {
			continue
		}

		if shift.SchedulingGroupID != os.Getenv("MS_SERVICE_DESK_GROUP_ID") {
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

		employees = append(employees, EmployeeAvailability{
			UserID: shift.UserID,
			Start:  start,
			End:    end,
		})
	}

	return employees
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
			hourlySlots := splitZoomSlotIntoHours(slots[i])
			updatedSlots := make([]ZoomShift, 0, len(slots)-1+len(hourlySlots))
			updatedSlots = append(updatedSlots, slots[:i]...)
			updatedSlots = append(updatedSlots, hourlySlots...)
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
	if slot.End.Sub(slot.Start) <= time.Hour {
		return false
	}

	fullBlockCandidates := findAvailabilityEmployees(employees, slot.Start, slot.End)
	hourlySlots := splitZoomSlotIntoHours(slot)

	if len(fullBlockCandidates) == 0 {
		for _, hourlySlot := range hourlySlots {
			if len(findAvailabilityEmployees(employees, hourlySlot.Start, hourlySlot.End)) > 0 {
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

	for _, hourlySlot := range hourlySlots {
		hasLowerLoadedCandidate := false
		for _, employee := range findAvailabilityEmployees(employees, hourlySlot.Start, hourlySlot.End) {
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

func splitZoomSlotIntoHours(slot ZoomShift) []ZoomShift {
	var hourlySlots []ZoomShift

	for start := slot.Start; start.Before(slot.End); start = start.Add(time.Hour) {
		end := start.Add(time.Hour)
		if end.After(slot.End) {
			end = slot.End
		}

		hourlySlots = append(hourlySlots, ZoomShift{
			Start: start,
			End:   end,
		})
	}

	return hourlySlots
}

func workedYesterday(
	userID string,
	start time.Time,
	end time.Time,
	yesterday []ZoomShift,
) bool {
	for _, shift := range yesterday {
		if shift.UserID != userID {
			continue
		}

		// compare clock times rather than dates.
		if start.Hour() < shift.End.Hour() &&
			end.Hour() > shift.Start.Hour() {
			return true
		}
	}
	return false
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
