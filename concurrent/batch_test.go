package concurrent

import (
	"context"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBatchRunOKNoConcurrency(t *testing.T) {
	inC := make(chan Supplier)

	go func() {
		for i := 0; i < 10; i++ {
			value := i
			inC <- SupplierFunc(func() (interface{}, error) {
				return value, nil
			})
		}
		close(inC)
	}()

	resC := BatchWithOpts(context.TODO(), inC, BatchOpts{
		Concurrency: 1,
	})

	counter := 0
	for res := range resC {
		assert.Equal(t, counter, res.Value())
		assert.Nil(t, res.Err())
		counter++
	}
}

func TestBatchRunErrTimeout(t *testing.T) {
	inC := make(chan Supplier)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	resC := BatchWithOpts(ctx, inC, BatchOpts{
		Concurrency: 1,
	})

	counter := 0
	for range resC {
		counter++
	}

	assert.Equal(t, 0, counter)
}

func TestBatchRunErrCancel(t *testing.T) {
	inC := make(chan Supplier, 64)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		for i := 0; i < 10; i++ {
			value := i
			inC <- SupplierFunc(func() (interface{}, error) {
				return value, nil
			})

			if i == 1 {
				cancel()
			}
		}
		close(inC)
	}()

	resC := BatchWithOpts(ctx, inC, BatchOpts{
		Concurrency: 1,
	})

	counter := 0
	for range resC {
		counter++
	}

	assert.True(t, counter < 10)
}

func TestBatchRunOKWithConcurrency(t *testing.T) {
	inC := make(chan Supplier)

	go func() {
		for i := 0; i < 10; i++ {
			value := i
			inC <- SupplierFunc(func() (interface{}, error) {
				return value, nil
			})
		}
		close(inC)
	}()

	resC := BatchWithOpts(context.TODO(), inC, BatchOpts{
		Concurrency: 8,
	})

	var results []int
	for res := range resC {
		assert.Nil(t, res.Err())
		results = append(results, res.Value().(int))
	}

	sort.Sort(sort.IntSlice(results))
	assert.Equal(t, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, results)
}

func TestBatchSliceOK(t *testing.T) {
	in := make([]Supplier, 0, 10)

	for i := 0; i < 10; i++ {
		value := i
		in = append(in, SupplierFunc(func() (interface{}, error) {
			time.Sleep(time.Duration(10-i) * time.Millisecond)
			return value, nil
		}))
	}

	res := BatchSliceWithOpts(context.TODO(), in, BatchOpts{Concurrency: 8})

	assert.Equal(t, len(in), len(res))
	for i := 0; i < len(res); i++ {
		assert.Equal(t, i, res[i].Value())
		assert.Nil(t, res[i].Err())
	}
}

// waitForGoroutines waits until the number of goroutines goes back
// to at most n, and returns the last observed count
func waitForGoroutines(n int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for {
		current := runtime.NumGoroutine()
		if current <= n || time.Now().After(deadline) {
			return current
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestBatchRunCancelWithUnreadConsumerDoesNotLeak(t *testing.T) {
	baseline := runtime.NumGoroutine()

	inC := make(chan Supplier, 100)
	for i := 0; i < 100; i++ {
		inC <- SupplierFunc(func() (interface{}, error) {
			return 0, nil
		})
	}
	close(inC)

	ctx, cancel := context.WithCancel(context.Background())
	runner := NewBatchRunnerWithOpts(BatchOpts{Concurrency: 4})
	outC := runner.Run(ctx, inC)

	// let the workers block trying to send results nobody reads
	time.Sleep(20 * time.Millisecond)
	cancel()

	current := waitForGoroutines(baseline, 5*time.Second)
	assert.True(t, current <= baseline,
		"goroutines leaked: baseline %d, current %d", baseline, current)

	// the output channel must be closed. A few results might still be
	// buffered in flight, but the channel must not stay open
	for range outC {
	}
}

func TestBatchRunCancelWithBlockedInputDoesNotLeak(t *testing.T) {
	baseline := runtime.NumGoroutine()

	// never closed and never written to after the initial values
	inC := make(chan Supplier)
	ctx, cancel := context.WithCancel(context.Background())
	outC := BatchWithOpts(ctx, inC, BatchOpts{Concurrency: 2})

	cancel()
	for range outC {
	}

	current := waitForGoroutines(baseline, 5*time.Second)
	assert.True(t, current <= baseline,
		"goroutines leaked: baseline %d, current %d", baseline, current)
}

func TestBatchRunnerRunningUntilBatchCompletes(t *testing.T) {
	runner := NewBatchRunnerWithOpts(BatchOpts{Concurrency: 1})
	inC := make(chan Supplier)
	outC := runner.Run(context.Background(), inC)

	// the first batch has not completed, so a second Run must panic
	assert.Panics(t, func() {
		runner.Run(context.Background(), inC)
	})

	close(inC)
	for range outC {
	}

	// the batch has completed, the runner can be reused. The flag is
	// reset right before outC is closed, so retry briefly
	inC2 := make(chan Supplier, 1)
	inC2 <- SupplierFunc(func() (interface{}, error) { return 1, nil })
	close(inC2)

	var outC2 <-chan BatchResult
	deadline := time.Now().Add(5 * time.Second)
	for outC2 == nil && time.Now().Before(deadline) {
		func() {
			defer func() { _ = recover() }()
			outC2 = runner.Run(context.Background(), inC2)
		}()
	}

	if assert.NotNil(t, outC2) {
		var values []interface{}
		for res := range outC2 {
			values = append(values, res.Value())
		}
		assert.Equal(t, []interface{}{1}, values)
	}
}

func TestBatchSliceCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make([]Supplier, 0, 10)
	for i := 0; i < 10; i++ {
		value := i
		in = append(in, SupplierFunc(func() (interface{}, error) {
			if value == 0 {
				cancel()
			}
			return value + 1, nil
		}))
	}

	res := BatchSliceWithOpts(ctx, in, BatchOpts{Concurrency: 1})
	assert.Equal(t, len(in), len(res))
	assert.Equal(t, context.Canceled, ctx.Err())

	// entries for operations that did not run are the zero value
	missing := 0
	for i := range res {
		if res[i].Value() == nil {
			missing++
			assert.Nil(t, res[i].Err())
		} else {
			assert.Equal(t, i+1, res[i].Value())
		}
	}
	assert.True(t, missing > 0)
}

func runBatchBenchmark(b *testing.B, s Supplier) {
	inC := make(chan Supplier, 64)
	runner := NewBatchRunnerWithOpts(BatchOpts{
		Concurrency: 8,
	})
	ctx := context.TODO()
	outC := runner.Run(ctx, inC)

	go func(inC chan<- Supplier) {
		for i := 0; i < b.N; i++ {
			inC <- s
		}
		close(inC)
	}(inC)

	counter := 0
	for range outC {
		counter++
	}

	assert.Equal(b, b.N, counter)
}

func BenchmarkBatchRunner(b *testing.B) {
	s := SupplierFunc(func() (interface{}, error) {
		return 0, nil
	})

	runBatchBenchmark(b, s)
}

func BenchmarkBatchRunnerWithConstantWait(b *testing.B) {
	s := SupplierFunc(func() (interface{}, error) {
		<-time.After(1 * time.Microsecond)
		return 0, nil
	})

	runBatchBenchmark(b, s)
}
