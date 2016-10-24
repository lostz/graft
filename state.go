package graft

import (
	"fmt"
)

// StateChange captures "from" and "to" States
type StateChange struct {
	// From is the previous state.
	From State

	// To is the new state.
	To State
}

//State for a graft node
type State int8

// Allowable states for a Graft node.
const (
	FOLLOWER State = iota
	LEADER
	CANDIDATE
	CLOSED
)

const sonyflakeTimeUnit = 1e7 // nsec, i.e. 10 msec
// These constants are the bit lengths of Sonyflake ID parts.
const (
	BitLenTime      = 39                               // bit length of time
	BitLenSequence  = 8                                // bit length of sequence number
	BitLenMachineID = 63 - BitLenTime - BitLenSequence // bit length of machine id
)

// Convenience for printing, etc.
func (s State) String() string {
	switch s {
	case FOLLOWER:
		return "Follower"
	case LEADER:
		return "Leader"
	case CANDIDATE:
		return "Candidate"
	case CLOSED:
		return "Closed"
	default:
		return fmt.Sprintf("Unknown[%d]", s)
	}
}
