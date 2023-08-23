package checker

import (
	"context"
	"regexp"
)

// State check status
type State int8

const (
	// CollectingState collecting state
	CollectingState State = iota

	// SuccessState success state
	SuccessState

	// WarnState warning state
	WarnState

	// ErrorState error state
	ErrorState

	// NotFoundState not found state (wrong check name or missed check object)
	NotFoundState

	// UnknownState unknown state (internal collector error)
	UnknownState
)

// ErrorChanged detect error change
func ErrorChanged(previous error, current error) bool {
	if previous == current {
		return false
	} else if previous != nil && current != nil {
		return previous.Error() != current.Error()
	}
	return true
}

// String get string for State
func (s *State) String() string {
	switch *s {
	case SuccessState:
		return "success"
	case WarnState:
		return "warn"
	case ErrorState:
		return "error"
	case NotFoundState:
		return "not found"
	default:
		return "unknown"
	}
}

// Strip for include as metric name part
func Strip(s string) string {
	reg := regexp.MustCompile(`[^a-zA-Z0-9_\-]+`)
	return reg.ReplaceAllString(s, "_")
}

// Metric describe checker metric
type Metric struct {
	Name  string
	Value string
}

// Checker interface
type Checker interface {
	Name() string
	// Status return check status and events
	Status(ctx context.Context, timestamp int64) (State, []string)
	// Return checker metrics
	Metrics() []Metric
}
