package concurrent

import (
	"errors"
	"fmt"
)

// ErrPoolStopped is returned by PoolRunner.Run when the pool has
// been stopped and cannot accept more inputs
var ErrPoolStopped = errors.New("pool runner is stopped")

// ErrMasterStopped is returned by the methods of a Master when its
// event loop has exited, either because Stop was called or because
// the context passed to Start is done
var ErrMasterStopped = errors.New("master is stopped")

// ErrWorkerExited is returned for a request sent to a worker that
// exited before handling it
var ErrWorkerExited = errors.New("worker has exited")

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
