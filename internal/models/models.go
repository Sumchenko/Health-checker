package models

import "time"

type Target struct {
	ID       int
	URL      string
	Interval time.Duration
}

type Result struct {
	TargetID     int
	URL          string
	StatusCode   int
	ResponseTime time.Duration
	Timestamp    time.Time
	Err          error
}
