package concurrent_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/eaugeas/octopus/concurrent"
)

// Run a fixed number of goroutines and submit work to them
func ExamplePoolRunner() {
	pool := concurrent.NewPoolRunnerWithOpts(context.Background(), concurrent.PoolOpts{
		Concurrency: 4,
	})
	defer pool.Stop()

	results := make(chan concurrent.PoolResult, 3)
	for i := 1; i <= 3; i++ {
		n := i
		if err := pool.Run(concurrent.PoolInput{
			Supplier: concurrent.SupplierFunc(func() (interface{}, error) {
				return n * n, nil
			}),
			OutC: results,
		}); err != nil {
			fmt.Println("failed to schedule:", err)
			return
		}
	}

	var squares []int
	for i := 0; i < 3; i++ {
		res := <-results
		squares = append(squares, res.Value().(int))
	}

	sort.Ints(squares)
	fmt.Println(squares)
	// Output: [1 4 9]
}

// Run a slice of operations concurrently and get the results back
// in the same order as the operations
func ExampleBatchSliceWithOpts() {
	words := []string{"octopus", "squid", "nautilus"}
	ops := make([]concurrent.Supplier, 0, len(words))
	for _, w := range words {
		word := w
		ops = append(ops, concurrent.SupplierFunc(func() (interface{}, error) {
			return len(word), nil
		}))
	}

	results := concurrent.BatchSliceWithOpts(context.Background(), ops, concurrent.BatchOpts{
		Concurrency: 2,
	})

	for i, res := range results {
		fmt.Println(words[i], res.Value(), res.Err())
	}
	// Output:
	// octopus 7 <nil>
	// squid 5 <nil>
	// nautilus 8 <nil>
}

// Retry an operation with capped exponential backoff
func ExampleRetryWithOpts() {
	attempts := 0
	v, err := concurrent.RetryWithOpts(context.Background(), concurrent.SupplierFunc(func() (interface{}, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("temporarily unavailable")
		}
		return "connected", nil
	}), concurrent.RetryOpts{
		Attempts:        5,
		BaseTimeout:     time.Millisecond,
		BaseExp:         2,
		MaxRetryTimeout: 10 * time.Millisecond,
		Random:          true,
	})

	fmt.Println(v, err, attempts)
	// Output: connected <nil> 3
}

// Keep one long-lived worker per key, send requests to a specific
// worker or to all of them, and let idle workers expire
func ExampleMaster() {
	ctx := context.Background()
	master := concurrent.NewMaster(concurrent.MasterOpts{
		MasterHandler: concurrent.MasterHandlerFunc(func(ctx context.Context, ev concurrent.MasterEvent) error {
			switch ev := ev.(type) {
			case concurrent.CreateWorkerEvent:
				// each worker keeps its own running total. The state is
				// only accessed from the worker goroutine
				total := 0
				ev.Opts.MaxInactivity = time.Minute
				ev.Opts.WorkerHandler = concurrent.WorkerHandlerFunc(
					func(ctx context.Context, ev concurrent.WorkerEvent) (interface{}, error) {
						if req, ok := ev.(concurrent.RequestWorkerEvent); ok {
							total += req.Value.(int)
							return total, nil
						}
						return nil, nil
					})
			case concurrent.DestroyWorkerEvent:
				// ev.Cause is set if the worker exited because of an error
				fmt.Println("destroyed", ev.Key, ev.Cause)
			}
			return nil
		}),
	})

	if err := master.Start(ctx); err != nil {
		fmt.Println("failed to start:", err)
		return
	}

	for _, key := range []string{"alice", "bob"} {
		if err := master.Create(ctx, key, nil); err != nil {
			fmt.Println("failed to create:", err)
			return
		}
	}

	_, _ = master.Request(ctx, "alice", 5)
	total, err := master.Request(ctx, "alice", 2)
	fmt.Println("alice", total, err)

	responses, err := master.Broadcast(ctx, 1)
	if err != nil {
		fmt.Println("failed to broadcast:", err)
		return
	}
	sort.Slice(responses, func(i, j int) bool { return responses[i].Key < responses[j].Key })
	for _, res := range responses {
		fmt.Println("broadcast", res.Key, res.Value, res.Error)
	}

	fmt.Println(master.Destroy(ctx, "bob"))
	fmt.Println(master.Stop())
	// Output:
	// alice 7 <nil>
	// broadcast alice 8 <nil>
	// broadcast bob 1 <nil>
	// destroyed bob <nil>
	// <nil>
	// destroyed alice <nil>
	// <nil>
}
