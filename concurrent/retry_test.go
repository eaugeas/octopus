package concurrent

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryError(t *testing.T) {
	ctx := context.Background()
	runs := 0

	res, err := RetryWithOpts(ctx, SupplierFunc(func() (interface{}, error) {
		runs++
		return runs, errors.New("error")
	}), RetryOpts{
		Attempts:        10,
		BaseExp:         2,
		BaseTimeout:     1 * time.Millisecond,
		MaxRetryTimeout: 10 * time.Millisecond,
	})

	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Equal(t, runs, 10)
}

func TestRetry(t *testing.T) {
	ctx := context.Background()
	runs := 0

	res, err := RetryWithOpts(ctx, SupplierFunc(func() (interface{}, error) {
		runs++
		return 0, nil
	}), RetryOpts{
		Attempts:        10,
		BaseExp:         2,
		BaseTimeout:     1 * time.Millisecond,
		MaxRetryTimeout: 10 * time.Millisecond,
	})

	assert.Nil(t, err)
	assert.Equal(t, res, 0)
	assert.Equal(t, runs, 1)
}

func TestRetryWithSomeErrors(t *testing.T) {
	ctx := context.Background()
	runs := 0

	res, err := RetryWithOpts(ctx, SupplierFunc(func() (interface{}, error) {
		runs++
		if runs < 9 {
			return nil, errors.New("error")
		}

		return 0, nil
	}), RetryOpts{
		Attempts:        10,
		BaseExp:         2,
		BaseTimeout:     1 * time.Millisecond,
		MaxRetryTimeout: 10 * time.Millisecond,
	})

	assert.Nil(t, err)
	assert.Equal(t, res, 0)
	assert.Equal(t, runs, 9)
}

func TestNextRetryDelayMonotonicAndCapped(t *testing.T) {
	cases := []struct {
		base       time.Duration
		exp        int64
		maxTimeout time.Duration
	}{
		{base: 1 * time.Millisecond, exp: 2, maxTimeout: 10 * time.Millisecond},
		{base: 10 * time.Millisecond, exp: 4, maxTimeout: 10 * time.Second},
		{base: 3 * time.Millisecond, exp: 7, maxTimeout: 1 * time.Second},
		{base: 0, exp: 0, maxTimeout: 5 * time.Millisecond},
		{base: 1 * time.Hour, exp: 255, maxTimeout: time.Duration(math.MaxInt64)},
	}

	for _, c := range cases {
		delay := firstRetryDelay(c.base, c.maxTimeout)
		assert.True(t, delay > 0)
		assert.True(t, delay <= c.maxTimeout)

		reachedCap := false
		for i := 0; i < 100; i++ {
			next := nextRetryDelay(delay, c.exp, c.maxTimeout)
			assert.True(t, next >= delay, "delay decreased from %v to %v", delay, next)
			assert.True(t, next <= c.maxTimeout, "delay %v exceeds cap %v", next, c.maxTimeout)
			if next == c.maxTimeout {
				reachedCap = true
			}
			delay = next
		}

		if c.exp > 1 {
			assert.True(t, reachedCap, "delay never reached the cap for %+v", c)
		}
	}
}

func TestNextRetryDelayValues(t *testing.T) {
	assert.Equal(t, 1*time.Millisecond, firstRetryDelay(0, 10*time.Millisecond))
	assert.Equal(t, 5*time.Millisecond, firstRetryDelay(20*time.Millisecond, 5*time.Millisecond))
	assert.Equal(t, 4*time.Millisecond, nextRetryDelay(2*time.Millisecond, 2, 10*time.Millisecond))
	assert.Equal(t, 10*time.Millisecond, nextRetryDelay(8*time.Millisecond, 2, 10*time.Millisecond))
	assert.Equal(t, 10*time.Millisecond, nextRetryDelay(10*time.Millisecond, 2, 10*time.Millisecond))
	assert.Equal(t, 3*time.Millisecond, nextRetryDelay(3*time.Millisecond, 0, 10*time.Millisecond))
	assert.Equal(t, time.Duration(math.MaxInt64),
		nextRetryDelay(time.Duration(math.MaxInt64/2+1), 2, time.Duration(math.MaxInt64)))
}

func TestRetryDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	res, err := RetryWithOpts(ctx, SupplierFunc(func() (interface{}, error) {
		return nil, errors.New("error")
	}), RetryOpts{
		UnlimitedAttempts: true,
		BaseExp:           2,
		BaseTimeout:       1 * time.Millisecond,
		MaxRetryTimeout:   5 * time.Millisecond,
	})

	assert.Nil(t, res)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "unexpected error %v", err)
}

func TestRetryCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := RetryWithOpts(ctx, SupplierFunc(func() (interface{}, error) {
		return nil, errors.New("error")
	}), RetryOpts{
		UnlimitedAttempts: true,
		BaseTimeout:       1 * time.Millisecond,
		MaxRetryTimeout:   5 * time.Millisecond,
	})

	assert.True(t, errors.Is(err, context.Canceled), "unexpected error %v", err)
}

func TestRetryZeroMaxTimeoutDoesNotPanic(t *testing.T) {
	runs := 0
	_, err := RetryWithOpts(context.Background(), SupplierFunc(func() (interface{}, error) {
		runs++
		return nil, errors.New("error")
	}), RetryOpts{
		Attempts: 3,
		BaseExp:  2,
	})

	assert.IsType(t, ErrMaxAttemptsReached{}, err)
	assert.Equal(t, 3, runs)
}

func TestRetryCannotRecover(t *testing.T) {
	cause := errors.New("fatal")
	runs := 0
	_, err := RetryWithOpts(context.Background(), SupplierFunc(func() (interface{}, error) {
		runs++
		return nil, ErrCannotRecover{Cause: cause}
	}), RandomOpts)

	assert.Equal(t, cause, err)
	assert.Equal(t, 1, runs)
}

func TestRetryRandomRespectsCap(t *testing.T) {
	runs := 0
	start := time.Now()
	_, err := RetryWithOpts(context.Background(), SupplierFunc(func() (interface{}, error) {
		runs++
		return nil, errors.New("error")
	}), RetryOpts{
		Random:          true,
		Attempts:        6,
		BaseExp:         10,
		BaseTimeout:     1 * time.Millisecond,
		MaxRetryTimeout: 2 * time.Millisecond,
	})

	assert.IsType(t, ErrMaxAttemptsReached{}, err)
	assert.Equal(t, 6, runs)
	// 5 waits of at most 2ms each, with generous slack for slow CI machines
	assert.True(t, time.Since(start) < 1*time.Second)
}
