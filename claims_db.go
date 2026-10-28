package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

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