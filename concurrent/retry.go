package concurrent

import (
	"context"
	"math/rand"
	"time"
)

const (
	defaultBaseTimeout     time.Duration = 10 * time.Millisecond
	defaultBaseExp         uint8         = 4
	defaultMaxRetryTimeout time.Duration = 10 * time.Second
	defaultAttempts        uint8         = 5
)

// RandomOpts is the default configuration for a Retry
// with random settings
var RandomOpts = RetryOpts{
	BaseTimeout:     defaultBaseTimeout,
	BaseExp:         defaultBaseExp,
	MaxRetryTimeout: defaultMaxRetryTimeout,
	Attempts:        defaultAttempts,
	Random:          true,
}

// RetryOpts is the configuration parameters for the Retry
// concurrent utility. Look at RetryWithOpts for more information
type RetryOpts struct {
	// Random sets the retry to wait a random time based on the
	// exponential back off
	Random bool

	// UnlimitedAttempts when set to true, Attempts will be ignored
	// and the action will be retried until it succeeds or the context
	// stops
	UnlimitedAttempts bool

	// Attempts is the maximum number of attempts allowed by a
	// Retry operation
	Attempts uint8

	// BaseExp is the base exponent for the calculation of the next
	// time an attempt must be triggered using exponential backoff
	BaseExp uint8

	// BaseTimeout is the initial timeout used after the first
	// attempt fails
	BaseTimeout time.Duration

	// MaxRetryTimeout sets an upper bound into the time that
	// the retry will wait until attempting an operation again.
	MaxRetryTimeout time.Duration
}

// RetryWithOpts is an implementation of an exponential back off
// retry operation for a supplier. It keeps retrying the operation
// until the maximum number of attempts has been reached, in which
// case it returns the associated error, or until it succeeds.
func RetryWithOpts(
	ctx context.Context,
	supplier Supplier,
	opts RetryOpts,
) (interface{}, error) {
	var errs []error
	timeout := opts.BaseTimeout.Nanoseconds()
	exp := int64(opts.BaseExp)
	maxTimeout := opts.MaxRetryTimeout.Nanoseconds()
	attempts := 0
	maxAttempts := int(opts.Attempts)
	timer := time.NewTimer(0 * time.Second)

	if opts.UnlimitedAttempts {
		maxAttempts = -1
	}

	for {
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, context.Canceled

		case <-timer.C:
			v, err := supplier.Supply()
			if err == nil {
				return v, nil
			}

			if err, ok := err.(ErrCannotRecover); ok {
				return nil, err.Cause
			}

			errs = append(errs, err)
		}

		attempts++
		if attempts >= maxAttempts && maxAttempts >= 0 {
			return nil, ErrMaxAttemptsReached{Causes: errs}
		}

		// make sure that the timeout is set to at least 1ns
		timeout = ((timeout * exp) % maxTimeout) + time.Millisecond.Nanoseconds()
		if opts.Random {
			timeout = int64(rand.Float64()*float64(timeout)) + 1
		}
		timer.Reset(time.Duration(timeout))
	}
}

// Retry is the same operation as RetryWithOpts but in this
// case the default values for RetryOpts are used
func Retry(ctx context.Context, supplier Supplier) (interface{}, error) {
	return RetryWithOpts(ctx, supplier, RetryOpts{
		BaseTimeout:     defaultBaseTimeout,
		BaseExp:         defaultBaseExp,
		MaxRetryTimeout: defaultMaxRetryTimeout,
		Attempts:        defaultAttempts,
	})
}

// RetryRandom is the same operation as RetryWithOpts but in this
// case the default values for RetryOpts are used with random
// exponential backoff
func RetryRandom(ctx context.Context, supplier Supplier) (interface{}, error) {
	return RetryWithOpts(ctx, supplier, RandomOpts)
}
