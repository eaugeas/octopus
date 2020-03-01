package concurrent

import (
	"context"
	"errors"
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
