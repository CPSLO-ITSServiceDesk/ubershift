package main

import (
	"os"
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

	end := start.Add(9 * time.Hour)

	for current := start; current.Before(end); current = current.Add(time.Hour) {
		slots = append(slots, ZoomShift{
			Start: current,
			End:   current.Add(time.Hour),
		})
	}
	return slots
}

func assignZoomSlots(
	slots []ZoomShift,
	employees []EmployeeAvailability,
	yesterday []ZoomShift,
) []ZoomShift {

	assignedHours := make(map[string]int)

	for i := range slots {
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
				assignedHours[employee.UserID] < assignedHours[selected] {
				selected = employee.UserID
			}
		}

		// If everyone worked this time yesterday,
		// select the employee with the fewest assigned hours.
		if selected == "" {
			selected = available[0].UserID

			for _, employee := range available {
				if assignedHours[employee.UserID] < assignedHours[selected] {
					selected = employee.UserID
				}
			}
		}

		slots[i].UserID = selected
		assignedHours[selected]++
	}

	return slots
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
