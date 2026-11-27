package concurrent

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPoolRunOKNoConcurrency(t *testing.T) {
	resC := make(chan PoolResult, 10)
	pool := NewPoolRunnerWithOpts(context.TODO(), PoolOpts{
		Concurrency: 1,
	})
	defer pool.Stop()

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

	for counter := 0; counter < 10; counter++ {
		res := <-resC
		assert.Equal(t, counter, res.Value())
		assert.Nil(t, res.Err())
	}

	// no more results are sent once the pool has stopped
	pool.Stop()
	close(resC)
}

func TestPoolRunOKWithConcurrency(t *testing.T) {
	resC := make(chan PoolResult, 10)
	pool := NewPoolRunnerWithOpts(context.TODO(), PoolOpts{
		Concurrency: 8,
	})
	defer pool.Stop()

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
	for counter := 0; counter < 10; counter++ {
		res := <-resC
		values = append(values, res.Value().(int))
		assert.Nil(t, res.Err())
	}

	// no more results are sent once the pool has stopped
	pool.Stop()
	close(resC)

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
	defer pool.Stop()

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

func TestPoolStopIdempotent(t *testing.T) {
	pool := NewPoolRunnerWithOpts(context.Background(), PoolOpts{
		Concurrency: 4,
	})

	pool.Stop()
	pool.Stop()

	err := pool.Run(PoolInput{
		Supplier: SupplierFunc(func() (interface{}, error) {
			return 0, nil
		}),
	})
	assert.Equal(t, ErrPoolStopped, err)
}

func TestPoolStopExecutesAcceptedInputs(t *testing.T) {
	var executed int32
	pool := NewPoolRunnerWithOpts(context.Background(), PoolOpts{
		Concurrency: 2,
	})

	for i := 0; i < 10; i++ {
		err := pool.Run(PoolInput{
			Supplier: SupplierFunc(func() (interface{}, error) {
				atomic.AddInt32(&executed, 1)
				return nil, nil
			}),
		})
		assert.Nil(t, err)
	}

	pool.Stop()
	assert.Equal(t, int32(10), atomic.LoadInt32(&executed))
}

func TestPoolRunConcurrentWithStop(t *testing.T) {
	for iteration := 0; iteration < 20; iteration++ {
		pool := NewPoolRunnerWithOpts(context.Background(), PoolOpts{
			Concurrency: 2,
		})

		var accepted, executed int32
		wg := sync.WaitGroup{}
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					err := pool.Run(PoolInput{
						Supplier: SupplierFunc(func() (interface{}, error) {
							atomic.AddInt32(&executed, 1)
							return nil, nil
						}),
					})
					if err == ErrPoolStopped {
						return
					}
					assert.Nil(t, err)
					atomic.AddInt32(&accepted, 1)
				}
			}()
		}

		stopWg := sync.WaitGroup{}
		for i := 0; i < 3; i++ {
			stopWg.Add(1)
			go func() {
				defer stopWg.Done()
				pool.Stop()
			}()
		}

		wg.Wait()
		stopWg.Wait()

		// every input accepted by Run must have been executed
		assert.Equal(t, atomic.LoadInt32(&accepted), atomic.LoadInt32(&executed))
	}
}

func TestPoolRunBlockedReturnsOnStop(t *testing.T) {
	release := make(chan struct{})
	pool := NewPoolRunnerWithOpts(context.Background(), PoolOpts{
		Concurrency: 1,
	})

	blocking := PoolInput{
		Supplier: SupplierFunc(func() (interface{}, error) {
			<-release
			return nil, nil
		}),
	}

	// one input is being executed, one sits in the buffer, so the
	// next Run blocks until Stop is called
	assert.Nil(t, pool.Run(blocking))
	assert.Nil(t, pool.Run(blocking))

	errC := make(chan error, 1)
	go func() {
		errC <- pool.Run(blocking)
	}()

	stopped := make(chan struct{})
	go func() {
		pool.Stop()
		close(stopped)
	}()

	select {
	case err := <-errC:
		// the blocked Run could also have been accepted right before
		// Stop closed the pool
		if err != nil {
			assert.Equal(t, ErrPoolStopped, err)
		}
	case <-time.After(5 * time.Second):
		assert.Fail(t, "Run did not return after Stop")
	}

	close(release)
	<-stopped
}

func TestPoolWorkerDoesNotBlockOnUnreadOutputAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pool := NewPoolRunnerWithOpts(ctx, PoolOpts{
		Concurrency: 2,
	})

	// unbuffered and never read
	outC := make(chan PoolResult)
	for i := 0; i < 2; i++ {
		assert.Nil(t, pool.Run(PoolInput{
			Supplier: SupplierFunc(func() (interface{}, error) {
				return 0, nil
			}),
			OutC: outC,
		}))
	}

	cancel()

	stopped := make(chan struct{})
	go func() {
		pool.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		assert.Fail(t, "workers blocked sending to an unread output channel")
	}

	assert.Equal(t, ErrPoolStopped, pool.Run(PoolInput{
		Supplier: SupplierFunc(func() (interface{}, error) { return 0, nil }),
	}))
}

func TestPoolRunReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pool := NewPoolRunnerWithOpts(ctx, PoolOpts{
		Concurrency: 1,
	})
	cancel()

	assert.Equal(t, context.Canceled, pool.Run(PoolInput{
		Supplier: SupplierFunc(func() (interface{}, error) { return 0, nil }),
	}))
	pool.Stop()
}

func runPoolBenchmark(b *testing.B, s Supplier) {
	outC := make(chan PoolResult, 64)
	pool := NewPoolRunnerWithOpts(context.TODO(), PoolOpts{
		Concurrency: 8,
	})
	defer pool.Stop()

	// b.N is read once, the producer may outlive the benchmark iteration
	n := b.N
	produced := make(chan struct{})
	go func(outC chan<- PoolResult) {
		defer close(produced)
		for i := 0; i < n; i++ {
			pool.Run(PoolInput{
				Supplier: s,
				OutC:     outC,
			})
		}
	}(outC)

	counter := 0
	for counter < n {
		<-outC
		counter++
	}

	// no more results are sent once the pool has stopped
	<-produced
	pool.Stop()
	close(outC)
	assert.Equal(b, n, counter)
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
