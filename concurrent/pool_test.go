package concurrent

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPoolRunOKNoConcurrency(t *testing.T) {
	resC := make(chan PoolResult, 10)
	pool := NewPoolRunnerWithOpts(context.TODO(), PoolOpts{
		Concurrency: 1,
	})

	go func() {
		for i := 0; i < 10; i++ {
			value := i
			pool.Run(PoolInput{
				Supplier: SupplierFunc(func() (interface{}, error) {
					return value, nil
				}),
				OutC: resC,
			})
		}
	}()

	counter := 0
	for res := range resC {
		assert.Equal(t, counter, res.Value())
		assert.Nil(t, res.Err())
		counter++

		if counter == 10 {
			close(resC)
		}
	}
}

func TestPoolRunOKWithConcurrency(t *testing.T) {
	resC := make(chan PoolResult, 10)
	pool := NewPoolRunnerWithOpts(context.TODO(), PoolOpts{
		Concurrency: 8,
	})

	go func() {
		for i := 0; i < 10; i++ {
			value := i
			pool.Run(PoolInput{
				Supplier: SupplierFunc(func() (interface{}, error) {
					return value, nil
				}),
				OutC: resC,
			})
		}
	}()

	var values []int
	counter := 0
	for res := range resC {
		values = append(values, res.Value().(int))
		assert.Nil(t, res.Err())
		counter++

		if counter == 10 {
			close(resC)
		}
	}

	sort.Sort(sort.IntSlice(values))
	assert.Equal(t, []int{
		0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
	}, values)
}

func TestPoolRunErrTimeout(t *testing.T) {
	resC := make(chan PoolResult, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	pool := NewPoolRunnerWithOpts(ctx, PoolOpts{
		Concurrency: 1,
	})

	<-time.After(5 * time.Millisecond)

	pool.Run(PoolInput{
		Supplier: SupplierFunc(func() (interface{}, error) {
			return 0, nil
		}),
		OutC: resC,
	})

	select {
	case <-time.After(10 * time.Millisecond):
		assert.True(t, true)
	case <-resC:
		assert.True(t, false, "should not have received an event")
	}
}

func TestPoolRunErrCancel(t *testing.T) {
	resC := make(chan PoolResult, 10)
	ctx, cancel := context.WithCancel(context.Background())
	pool := NewPoolRunnerWithOpts(ctx, PoolOpts{
		Concurrency: 1,
	})

	for i := 0; i < 10; i++ {
		value := i
		pool.Run(PoolInput{
			Supplier: SupplierFunc(func() (interface{}, error) {
				return value, nil
			}),
			OutC: resC,
		})

		if i == 1 {
			cancel()
		}
	}

	pool.Stop()
	close(resC)
	counter := 0

	assert.True(t, counter < 10)
}

func runPoolBenchmark(b *testing.B, s Supplier) {
	outC := make(chan PoolResult, 64)
	pool := NewPoolRunnerWithOpts(context.TODO(), PoolOpts{
		Concurrency: 8,
	})

	go func(outC chan<- PoolResult) {
		for i := 0; i < b.N; i++ {
			pool.Run(PoolInput{
				Supplier: s,
				OutC:     outC,
			})
		}
	}(outC)

	counter := 0
	for range outC {
		counter++
		if counter == b.N {
			break
		}
	}

	close(outC)
	assert.Equal(b, b.N, counter)
}

func BenchmarkPoolRunner(b *testing.B) {
	s := SupplierFunc(func() (interface{}, error) {
		return nil, nil
	})
	runPoolBenchmark(b, s)
}

func BenchmarkPoolRunnerWithConstantWait(b *testing.B) {
	s := SupplierFunc(func() (interface{}, error) {
		<-time.After(1 * time.Microsecond)
		return nil, nil
	})
	runPoolBenchmark(b, s)
}
