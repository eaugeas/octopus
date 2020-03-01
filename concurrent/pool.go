package concurrent

import (
	"context"
	"sync"
)

// poolInput to a batch operation
type PoolInput struct {
	OutC     chan<- PoolResult
	Supplier Supplier
}

// PoolResult of a batch operation
type PoolResult struct {
	Result
}

// PoolOpts is the optsuration for a PoolRunner
type PoolOpts struct {
	// Concurrency is the number of Suppliers that the pool can
	// run in parallel at most
	Concurrency int
}

// PoolRunner has a fixed number of goroutines that are used to run
// an arbitrary number of tasks. A PoolRunner is a convenient way to
// execute multiple operations in parallel having control on how
// many go routines are run in the system.
type PoolRunner struct {
	opts PoolOpts
	wg   sync.WaitGroup
	ctx  context.Context

	// inC is the channel used to send operations to the PoolRunner. If
	// PoolInput.C is set, the result of the supplier will be sent
	// to that channel, otherwise the result will be ignored
	inC chan PoolInput
}

// NewPoolRunner creates and starts a new PoolRunner with the
// default optsuration parameters
func NewPoolRunner(ctx context.Context) *PoolRunner {
	return NewPoolRunnerWithOpts(ctx, PoolOpts{
		Concurrency: defaultConcurrency,
	})
}

// NewPoolRunnerWithOpts creates a new PoolRunner with the specified
// optsuration
func NewPoolRunnerWithOpts(ctx context.Context, opts PoolOpts) *PoolRunner {
	if opts.Concurrency <= 0 {
		opts.Concurrency = defaultConcurrency
	}

	runner := &PoolRunner{
		opts: opts,
		ctx:  ctx,
		inC:  make(chan PoolInput, opts.Concurrency),
	}

	runner.wg.Add(int(opts.Concurrency))
	for i := 0; i < int(opts.Concurrency); i++ {
		go runner.run(ctx, runner.inC)
	}

	return runner
}

func (r *PoolRunner) Run(input PoolInput) error {
	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case r.inC <- input:
		return nil
	}
}

func (r *PoolRunner) run(ctx context.Context, inC <-chan PoolInput) {
	go func() {
		defer r.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case in, ok := <-inC:
				if !ok {
					return
				}

				v, err := in.Supplier.Supply()

				if in.OutC != nil {
					in.OutC <- PoolResult{
						Result{
							value: v,
							err:   err,
						}}
				}
			}
		}
	}()
}

// Stop orderly stops all the goroutines in the PoolRunner
// and returns once all the goroutines have exited
func (r *PoolRunner) Stop() {
	close(r.inC)
	r.wg.Wait()
}
