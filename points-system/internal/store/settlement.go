package store

import "time"

type rowScanner interface {
	Scan(...any) error
}

type AttemptResult struct {
	HTTPStatus int
	Outcome    string
	ErrorCode  string
	Error      string
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	delay := time.Duration(1<<uint(attempt-1)) * time.Minute
	if delay > 6*time.Hour {
		return 6 * time.Hour
	}
	return delay
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
