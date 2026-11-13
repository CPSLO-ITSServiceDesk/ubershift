package autoshift

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type ShiftPortionClaim struct {
	OpenShiftID string
	Start       time.Time
	End         time.Time
	SlackUserID string
}

func openClaimDB() (*sql.DB, error) {
	return openClaimDBAt("autoshift.db")
}

func openClaimDBAt(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS 
		open_shift_claims (
			open_shift_id TEXT PRIMARY KEY,
			slack_user_id TEXT NOT NULL,
			claimed_at TEXT NOT NULL
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create claims table: %w", err)
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS shift_portion_claims (
			id INTEGER PRIMARY KEY,
			open_shift_id TEXT NOT NULL,
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL,
			slack_user_id TEXT NOT NULL,
			claimed_at TEXT NOT NULL,
			UNIQUE(open_shift_id, start_time, end_time)
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create portion claims table: %w", err)
	}
	return db, nil
}

func saveClaims(db *sql.DB, openShiftID string, slackUserID string) (bool, error) {
	result, err := db.Exec(`
		INSERT INTO open_shift_claims (
			open_shift_id, 
			slack_user_id, 
			claimed_at
		)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(open_shift_id) DO NOTHING
	`, openShiftID, slackUserID)
	if err != nil {
		return false, fmt.Errorf("save claim: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check saved claim %w", err)
	}

	return rows == 1, nil
}

func loadClaims(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query(`
		SELECT open_shift_id, slack_user_id
		FROM open_shift_claims
	`)
	if err != nil {
		return nil, fmt.Errorf("load claims: %w", err)
	}
	defer rows.Close()

	claims := make(map[string]string)
	for rows.Next() {
		var openShiftID string
		var slackUserID string

		if err := rows.Scan(&openShiftID, &slackUserID); err != nil {
			return nil, fmt.Errorf("read claim: %w", err)
		}

		claims[openShiftID] = slackUserID
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("finish loading claims: %w", err)
	}

	return claims, nil
}

func savePortionClaim(db *sql.DB, claim ShiftPortionClaim) (bool, error) {
	if !claim.Start.Before(claim.End) {
		return false, fmt.Errorf("claim start must be before end")
	}

	start := claim.Start.UTC().Format(time.RFC3339)
	end := claim.End.UTC().Format(time.RFC3339)

	var overlappingClaims int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM shift_portion_claims
		WHERE open_shift_id = ?
			AND start_time < ?
			AND end_time > ?
	`, claim.OpenShiftID, end, start).Scan(&overlappingClaims)
	if err != nil {
		return false, fmt.Errorf("check claim overlap: %w", err)
	}

	if overlappingClaims > 0 {
		return false, nil
	}

	_, err = db.Exec(`
		INSERT INTO shift_portion_claims (
			open_shift_id,
			start_time,
			end_time,
			slack_user_id,
			claimed_at
		)
		VALUES (?, ?, ?, ?, datetime('now'))
	`,
		claim.OpenShiftID,
		start,
		end,
		claim.SlackUserID,
	)
	if err != nil {
		return false, fmt.Errorf("save portion claim: %w", err)
	}

	return true, nil
}

func loadPortionClaims(db *sql.DB, openShiftID string) ([]ShiftPortionClaim, error) {
	rows, err := db.Query(`
		SELECT start_time, end_time, slack_user_id
		FROM shift_portion_claims
		WHERE open_shift_id = ?
		ORDER BY start_time
	`, openShiftID)
	if err != nil {
		return nil, fmt.Errorf("load portion claims: %w", err)
	}
	defer rows.Close()

	var claims []ShiftPortionClaim
	for rows.Next() {
		var startText string
		var endText string
		var claim ShiftPortionClaim

		claim.OpenShiftID = openShiftID
		if err := rows.Scan(&startText, &endText, &claim.SlackUserID); err != nil {
			return nil, fmt.Errorf("read portion claim: %w", err)
		}

		claim.Start, err = time.Parse(time.RFC3339, startText)
		if err != nil {
			return nil, fmt.Errorf("parse portion claim start: %w", err)
		}

		claim.End, err = time.Parse(time.RFC3339, endText)
		if err != nil {
			return nil, fmt.Errorf("parse portion claim end: %w", err)
		}

		claims = append(claims, claim)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("finish loading portion claims: %w", err)
	}

	return claims, nil
}
