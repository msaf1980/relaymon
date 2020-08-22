package checker

// State check status
type State int8

const (
	// SuccessState success state
	SuccessState State = iota

	// WarnState warning state
	WarnState

	// ErrorState error state
	ErrorState

	// CollectingState collecting state
	CollectingState

	// NotFoundState not found state (wrong check name or missed check object)
	NotFoundState

	// UnknownState unknown state (internal collector error)
	UnknownState
)

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

// Checker interface
type Checker interface {
	Name() string
	Status() (State, []error)
}
