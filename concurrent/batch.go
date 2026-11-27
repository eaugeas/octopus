package concurrent

import (
	"context"
	"sync"
	"sync/atomic"
)

// batchInput to a batch operation
type batchInput struct {
	Supplier Supplier
	Index    int64
}

// BatchResult of a batch operation
type BatchResult struct {
	Result
	index int64
}

// Index is the position of the result within the batch
// of operations
func (r BatchResult) Index() int64 {
	return r.index
}

// BatchOpts are the options to configure how a batch of
// operations will be executed
type BatchOpts struct {
	// Concurrency specificies the maximum number of goroutines
	// that will be used to run all the operations in the batch
	Concurrency int
}

// BatchRunner executes a batch of operations until all of them
// complete, and returns the results in the order in which the
// suppliers were provided. This is useful to execute a set of operations
// as a block, and there's a need to wait for the whole results of the block
// before moving forward.
type BatchRunner struct {
	opts    BatchOpts
	running int32
}

// NewBatchRunner creates a new instance of a BatchRunner using
// the default values as configuration
func NewBatchRunner() *BatchRunner {
	return NewBatchRunnerWithOpts(BatchOpts{
		Concurrency: defaultConcurrency,
	})
}

// NewBatchRunnerWithOpts creates a new instance of a BatchRunner
// with the specified options
func NewBatchRunnerWithOpts(opts BatchOpts) *BatchRunner {
	if opts.Concurrency <= 0 {
		opts.Concurrency = defaultConcurrency
	}

	return &BatchRunner{opts: opts}
}

// Run runs all the operations provided by the input channel
// and returns the results. The results may be returned in a
// different order compared to the order in which the inputs
// are received. The returned channel is closed once all the
// inputs have been processed, or once ctx is done. If ctx is
// done, operations that have not started yet are not executed and
// results that have not been consumed are dropped, so consumers must
// not rely on receiving one result per input in that case.
//
// A BatchRunner can process a single batch at a time. Calling Run
// again before the channel returned by the previous call is closed
// panics.
func (r *BatchRunner) Run(
	ctx context.Context,
	inC <-chan Supplier,
) <-chan BatchResult {
	outC := make(chan BatchResult)
	if ok := atomic.CompareAndSwapInt32(&r.running, 0, 1); !ok {
		panic("attempt to call Run when BatchRunner is already processing a batch")
	}

	wg := &sync.WaitGroup{}
	wg.Add(r.opts.Concurrency)
	argC := make(chan batchInput, 64)
	for i := 0; i < r.opts.Concurrency; i++ {
		go r.run(ctx, argC, outC, wg)
	}

	go func() {
		// the runner is only released once the batch has completed,
		// that is, once all the workers have exited
		defer func() {
			if ok := atomic.CompareAndSwapInt32(&r.running, 1, 0); !ok {
				panic("attempt to stop BatchRunner that is not running")
			}
		}()

		r.sendInputs(ctx, inC, argC)
		close(argC)
		wg.Wait()
		close(outC)
	}()

	return outC
}

func (r *BatchRunner) sendInputs(
	ctx context.Context,
	inC <-chan Supplier,
	argC chan<- batchInput,
) {
	var counter int64
	for {
		select {
		case <-ctx.Done():
			return
		case in, ok := <-inC:
			if !ok {
				return
			}

			select {
			case <-ctx.Done():
				return
			case argC <- batchInput{
				Supplier: in,
				Index:    counter,
			}:
			}
		}
		counter++
	}
}

func (r *BatchRunner) run(
	ctx context.Context,
	inC chan batchInput,
	outC chan<- BatchResult,
	wg *sync.WaitGroup,
) {
	defer func() {
		wg.Done()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case in, ok := <-inC:
			if !ok {
				return
			}

			value, err := in.Supplier.Supply()
			select {
			case <-ctx.Done():
				return
			case outC <- BatchResult{
				Result: Result{
					value: value,
					err:   err,
				},
				index: in.Index,
			}:
			}
		}
	}
}

// Batch operations provided by the input channel
// and return the results of those operations on the
// output channel
func Batch(
	ctx context.Context,
	inC <-chan Supplier,
) <-chan BatchResult {
	return BatchWithOpts(ctx, inC, BatchOpts{
		Concurrency: defaultConcurrency,
	})
}

// BatchWithOpts operations provided by the input channel
// and return the results of those operations on the
// output channel. It allows to specify the configuration
// options on how the batch will be run
func BatchWithOpts(
	ctx context.Context,
	inC <-chan Supplier,
	opts BatchOpts,
) <-chan BatchResult {
	runner := NewBatchRunnerWithOpts(opts)
	return runner.Run(ctx, inC)
}

// BatchSlice runs as a batch a slice of operations
// and returns an ordered batch with the results.
//
// The returned slice always has the same length as in, and
// result i corresponds to in[i]. If ctx is done before all the
// operations complete, BatchSlice returns early and the entries for
// operations that were not executed (or whose result was dropped)
// are left as the zero BatchResult: Value() and Err() are nil and
// Index() is 0. Callers that need to tell those entries apart from
// successful results should check ctx.Err() after BatchSlice returns.
func BatchSlice(
	ctx context.Context,
	in []Supplier,
) []BatchResult {
	return BatchSliceWithOpts(ctx, in, BatchOpts{
		Concurrency: defaultConcurrency,
	})
}

// BatchSliceWithOpts runs all the operations in the slice
// as a batch and equally returns a slice with the
// results. See BatchSlice for the behaviour on cancellation
func BatchSliceWithOpts(
	ctx context.Context,
	in []Supplier,
	opts BatchOpts,
) []BatchResult {
	inC := make(chan Supplier, 64)

	go func() {
		defer close(inC)
		for i := 0; i < len(in); i++ {
			select {
			case <-ctx.Done():
				return
			case inC <- in[i]:
			}
		}
	}()

	results := make([]BatchResult, len(in))
	outC := BatchWithOpts(ctx, inC, opts)
	for res := range outC {
		results[res.Index()] = res
	}

	return results
}
