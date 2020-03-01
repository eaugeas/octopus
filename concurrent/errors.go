package concurrent

import "fmt"

// ErrNoOccurrence is returned failing to wait for an
// event that never occurred
type ErrNoOccurrence struct{}

func (e ErrNoOccurrence) Error() string {
	return "the expected event never occurred"
}

// ErrCannotRecover is an error that can be passed by clients to
// retry mechanisms so that the attempted action is not retried
type ErrCannotRecover struct {
	Cause error
}

// Error implementation of error for ErrCannotRecover
func (e ErrCannotRecover) Error() string {
	return e.Cause.Error()
}

// ErrMaxAttemptsReached is an error that is returned after attempting
// an action multiple times with failures
type ErrMaxAttemptsReached struct {
	Causes []error
}

// Error implementation of error for ErrCannotRecover
func (e ErrMaxAttemptsReached) Error() string {
	return fmt.Sprintf("maximum number of attempts %d reached", len(e.Causes))
}
