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

	// minRetryTimeout is the lower bound for the delay between two
	// attempts, so that a zero BaseTimeout does not cause a busy loop
	minRetryTimeout time.Duration = time.Millisecond
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
	// If it is not set (zero or negative) a default of 10s is used.
	MaxRetryTimeout time.Duration
}

// RetryWithOpts is an implementation of an exponential back off
// retry operation for a supplier. It keeps retrying the operation
// until the maximum number of attempts has been reached, in which
// case it returns the associated error, or until it succeeds.
//
// The first retry waits BaseTimeout, and every following retry waits
// BaseExp times longer than the previous one, never exceeding
// MaxRetryTimeout. When Random is set, each wait is a random duration
// in (0, delay] where delay is the capped exponential value (full jitter).
// If the context is done before the operation succeeds, ctx.Err() is
// returned.
func RetryWithOpts(
	ctx context.Context,
	supplier Supplier,
	opts RetryOpts,
) (interface{}, error) {
	var errs []error
	maxTimeout := opts.MaxRetryTimeout
	if maxTimeout <= 0 {
		maxTimeout = defaultMaxRetryTimeout
	}
	var delay time.Duration
	attempts := 0
	maxAttempts := int(opts.Attempts)
	timer := time.NewTimer(0 * time.Second)
	defer timer.Stop()

	if opts.UnlimitedAttempts {
		maxAttempts = -1
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

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

		if attempts == 1 {
			delay = firstRetryDelay(opts.BaseTimeout, maxTimeout)
		} else {
			delay = nextRetryDelay(delay, int64(opts.BaseExp), maxTimeout)
		}

		wait := delay
		if opts.Random {
			wait = time.Duration(rand.Float64()*float64(delay)) + 1
		}
		timer.Reset(wait)
	}
}

// firstRetryDelay returns the delay to wait after the first failed
// attempt. It is at least minRetryTimeout and at most maxTimeout
func firstRetryDelay(base, maxTimeout time.Duration) time.Duration {
	if base < minRetryTimeout {
		base = minRetryTimeout
	}

	if base > maxTimeout {
		return maxTimeout
	}

	return base
}

// nextRetryDelay computes the next exponential backoff delay from
// the current one as min(current*exp, maxTimeout). Exponents lower
// than 1 are treated as 1 so that the delay never decreases, and the
// multiplication is guarded against overflow.
func nextRetryDelay(current time.Duration, exp int64, maxTimeout time.Duration) time.Duration {
	if exp < 1 {
		exp = 1
	}

	if current < minRetryTimeout {
		current = minRetryTimeout
	}

	if current >= maxTimeout || current > maxTimeout/time.Duration(exp) {
		return maxTimeout
	}

	return current * time.Duration(exp)
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
