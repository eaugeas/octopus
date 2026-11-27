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

	// stopC is closed when Stop is called so that any Run call blocked
	// waiting for capacity returns ErrPoolStopped
	stopC    chan struct{}
	stopOnce sync.Once

	// mu guards closed and the closing of inC, so that Run never sends
	// on a closed channel. Run holds a read lock while sending, Stop
	// holds the write lock while closing inC
	mu     sync.RWMutex
	closed bool
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
		opts:  opts,
		ctx:   ctx,
		inC:   make(chan PoolInput, opts.Concurrency),
		stopC: make(chan struct{}),
	}

	runner.wg.Add(int(opts.Concurrency))
	for i := 0; i < int(opts.Concurrency); i++ {
		go runner.run(ctx, runner.inC)
	}

	return runner
}

// Run schedules the input to be executed by one of the goroutines
// of the pool. It blocks until the pool accepts the input, the
// context of the pool is done, in which case the context error is
// returned, or the pool is stopped, in which case ErrPoolStopped
// is returned. It is safe to call Run concurrently with Stop.
func (r *PoolRunner) Run(input PoolInput) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return ErrPoolStopped
	}

	if err := r.ctx.Err(); err != nil {
		return err
	}

	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case <-r.stopC:
		return ErrPoolStopped
	case r.inC <- input:
		return nil
	}
}

func (r *PoolRunner) run(ctx context.Context, inC <-chan PoolInput) {
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
				select {
				case <-ctx.Done():
					return
				case in.OutC <- PoolResult{
					Result{
						value: v,
						err:   err,
					}}:
				}
			}
		}
	}
}

// Stop orderly stops all the goroutines in the PoolRunner
// and returns once all the goroutines have exited. Inputs that
// were already accepted by Run are still executed before the
// goroutines exit, and their results delivered to PoolInput.OutC,
// so a caller of Stop must keep consuming those channels (or cancel
// the context of the pool) for Stop to return. Stop is idempotent
// and safe to call concurrently with Run; after Stop is called,
// Run returns ErrPoolStopped.
func (r *PoolRunner) Stop() {
	r.stopOnce.Do(func() {
		// wake up any Run call blocked waiting for capacity
		close(r.stopC)

		r.mu.Lock()
		r.closed = true
		close(r.inC)
		r.mu.Unlock()
	})

	r.wg.Wait()
}
