package concurrent

import (
	"context"
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
