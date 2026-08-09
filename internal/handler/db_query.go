package handler

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"log"
	"strings"
	"time"
)

const dbQueryMaxAttempts = 3

func isRetryableQueryErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, driver.ErrBadConn) {
		return true
	}
	msg := strings.ToLower(err.Error())
	// Do not retry query timeouts — a slow FULLTEXT/LIKE scan would run 3x.
	if strings.Contains(msg, "i/o timeout") {
		return false
	}
	return strings.Contains(msg, "invalid connection") ||
		strings.Contains(msg, "bad connection") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe")
}

func queryRowsWithRetry(db *sql.DB, query string, args ...interface{}) (*sql.Rows, error) {
	var lastErr error
	for attempt := 1; attempt <= dbQueryMaxAttempts; attempt++ {
		rows, err := db.Query(query, args...)
		if err == nil {
			return rows, nil
		}
		lastErr = err
		if attempt < dbQueryMaxAttempts && isRetryableQueryErr(err) {
			log.Printf("db query retry attempt=%d err=%v", attempt, err)
			time.Sleep(time.Duration(attempt) * 50 * time.Millisecond)
			continue
		}
		return nil, err
	}
	return nil, lastErr
}
