package graft

import "errors"

var (
	ErrLogCorrupt = errors.New("Encountered corrupt log file")
	ErrNoStateLog = errors.New("Log file does not have any state")
)
