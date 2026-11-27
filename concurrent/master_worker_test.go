package concurrent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func ScopedMaster(t assert.TestingT, fn func(ctx context.Context, m *Master)) {
	ctx := context.Background()
	master := NewMaster(MasterOpts{
		MasterHandler: &MockMasterHandler{},
	})

	err := master.Start(ctx)
	assert.Nil(t, err)

	fn(ctx, master)

	err = master.Stop()
	assert.Nil(t, err)
}

type MockMasterHandler struct {
	created   int32
	destroyed int32
}

func (m *MockMasterHandler) Created() int {
	return int(atomic.LoadInt32(&m.created))
}

func (m *MockMasterHandler) Destroyed() int {
	return int(atomic.LoadInt32(&m.destroyed))
}

func (m *MockMasterHandler) Handle(ctx context.Context, req MasterEvent) error {
	switch req := req.(type) {
	case CreateWorkerEvent:
		req.Opts.ErrC = nil
		req.Opts.UserData = nil
		req.Opts.WorkerHandler = &MockWorkerHandler{}
		atomic.AddInt32(&m.created, 1)
	case DestroyWorkerEvent:
		atomic.AddInt32(&m.destroyed, 1)
	default:
		panic("received unknown master event")
	}

	return nil
}

type MockWorkerHandler struct {
	Value int
}

func (m *MockWorkerHandler) Handle(ctx context.Context, req WorkerEvent) (interface{}, error) {
	switch req := req.(type) {
	case RequestWorkerEvent:
		m.Value = req.Value.(int) + 1
		return m.Value, nil
	case ErrorWorkerEvent:
		return nil, req.Error
	default:
		panic("received unknown worker event")
	}
}

func TestNewMaster(t *testing.T) {
	master := NewMaster(MasterOpts{
		MasterHandler: &MockMasterHandler{},
	})

	err := master.Start(context.Background())
	assert.Nil(t, err)

	err = master.Stop()
	assert.Nil(t, err)
}

func TestNewMasterStartTwice(t *testing.T) {
	master := NewMaster(MasterOpts{
		MasterHandler: &MockMasterHandler{},
	})

	err := master.Start(context.Background())
	assert.Nil(t, err)
	defer func() {
		err := master.Stop()
		assert.Nil(t, err)
	}()

	err = master.Start(context.Background())
	assert.Error(t, err)
}

func TestNewMasterStopTwice(t *testing.T) {
	master := NewMaster(MasterOpts{
		MasterHandler: &MockMasterHandler{},
	})

	err := master.Start(context.Background())
	assert.Nil(t, err)

	err = master.Stop()
	assert.Nil(t, err)

	err = master.Stop()
	assert.Error(t, err)
}

func TestNewMasterStopWithoutStart(t *testing.T) {
	master := NewMaster(MasterOpts{
		MasterHandler: &MockMasterHandler{},
	})

	err := master.Stop()
	assert.Error(t, err)
}

func TestMasterWorkerExists(t *testing.T) {
	ctx := context.Background()
	handler := &MockMasterHandler{}
	master := NewMaster(MasterOpts{
		MasterHandler: handler,
	})

	err := master.Start(ctx)
	assert.Nil(t, err)

	ok, err := master.Exists(ctx, "1")
	assert.Nil(t, err)
	assert.False(t, ok)

	err = master.Create(ctx, "1", nil)
	assert.Nil(t, err)

	ok, err = master.Exists(ctx, "1")
	assert.Nil(t, err)
	assert.True(t, ok)

	err = master.Destroy(ctx, "1")
	assert.Nil(t, err)

	ok, err = master.Exists(ctx, "1")
	assert.Nil(t, err)
	assert.False(t, ok)

	err = master.Stop()
	assert.Nil(t, err)

	assert.Equal(t, 1, handler.Created())
	assert.Equal(t, 1, handler.Destroyed())
}

func TestMasterWorkerRequest(t *testing.T) {
	ctx := context.Background()
	handler := &MockMasterHandler{}
	master := NewMaster(MasterOpts{
		MasterHandler: handler,
	})

	err := master.Start(ctx)
	assert.Nil(t, err)

	err = master.Create(ctx, "1", nil)
	assert.Nil(t, err)

	v, err := master.Request(ctx, "1", 0)
	assert.Nil(t, err)
	assert.Equal(t, 1, v)

	err = master.Destroy(ctx, "1")
	assert.Nil(t, err)

	err = master.Stop()
	assert.Nil(t, err)

	assert.Equal(t, 1, handler.Created())
	assert.Equal(t, 1, handler.Destroyed())
}

func TestMasterStopShutdownWorkers(t *testing.T) {
	ctx := context.Background()
	handler := &MockMasterHandler{}
	master := NewMaster(MasterOpts{
		MasterHandler: handler,
	})

	err := master.Start(ctx)
	assert.Nil(t, err)

	err = master.Create(ctx, "1", nil)
	assert.Nil(t, err)

	err = master.Stop()
	assert.Nil(t, err)

	assert.Equal(t, 1, handler.Created())
	assert.Equal(t, 1, handler.Destroyed())
}

func TestMasterWorkerPanicOnCreate(t *testing.T) {
	ctx := context.Background()
	handler := MasterHandlerFunc(func(ctx context.Context, ev MasterEvent) error {
		if ev, ok := ev.(CreateWorkerEvent); ok {
			ev.Opts.WorkerHandler = WorkerHandlerFunc(func(ctx context.Context, ev WorkerEvent) (interface{}, error) {
				panic("error")
			})
		}

		return nil
	})
	master := NewMaster(MasterOpts{
		MasterHandler: handler,
	})

	err := master.Start(ctx)
	assert.Nil(t, err)

	err = master.Create(ctx, "1", nil)
	assert.Nil(t, err)

	err = master.Stop()
	assert.Nil(t, err)
}

func TestMasterHandlerErrorOnCreate(t *testing.T) {
	ctx := context.Background()
	handler := MasterHandlerFunc(func(ctx context.Context, ev MasterEvent) error {
		return errors.New("error")
	})
	master := NewMaster(MasterOpts{
		MasterHandler: handler,
	})

	err := master.Start(ctx)
	assert.Nil(t, err)

	err = master.Create(ctx, "1", nil)
	assert.Error(t, err)
}

func TestMasterHandlerErrorOnDestroy(t *testing.T) {
	ctx := context.Background()
	handler := MasterHandlerFunc(func(ctx context.Context, ev MasterEvent) error {
		switch req := ev.(type) {
		case CreateWorkerEvent:
			req.Opts.ErrC = nil
			req.Opts.UserData = nil
			req.Opts.WorkerHandler = &MockWorkerHandler{}
		case DestroyWorkerEvent:
			return errors.New("error")
		default:
			panic("received unknown master event")
		}

		return nil
	})
	master := NewMaster(MasterOpts{
		MasterHandler: handler,
	})

	err := master.Start(ctx)
	assert.Nil(t, err)

	err = master.Create(ctx, "1", nil)
	assert.Nil(t, err)

	err = master.Destroy(ctx, "1")
	assert.Error(t, err)

	err = master.Stop()
	assert.Nil(t, err)
}

func TestMasterHandlerPanicOnDestroy(t *testing.T) {
	ctx := context.Background()
	handler := MasterHandlerFunc(func(ctx context.Context, ev MasterEvent) error {
		switch req := ev.(type) {
		case CreateWorkerEvent:
			req.Opts.ErrC = nil
			req.Opts.UserData = nil
			req.Opts.WorkerHandler = &MockWorkerHandler{}
		case DestroyWorkerEvent:
			panic("error")
		default:
			panic("received unknown master event")
		}

		return nil
	})
	master := NewMaster(MasterOpts{
		MasterHandler: handler,
	})

	err := master.Start(ctx)
	assert.Nil(t, err)

	err = master.Create(ctx, "1", nil)
	assert.Nil(t, err)

	err = master.Destroy(ctx, "1")
	assert.Error(t, err)

	err = master.Stop()
	assert.Nil(t, err)
}

func TestMasterCreateNoStart(t *testing.T) {
	ctx := context.Background()
	master := NewMaster(MasterOpts{
		MasterHandler: &MockMasterHandler{},
	})

	err := master.Create(ctx, "1", nil)
	assert.Error(t, err)
}

func TestMasterDestroyNoStart(t *testing.T) {
	ctx := context.Background()
	master := NewMaster(MasterOpts{
		MasterHandler: &MockMasterHandler{},
	})

	err := master.Destroy(ctx, "1")
	assert.Error(t, err)
}
func TestMasterExistsNoStart(t *testing.T) {
	ctx := context.Background()
	master := NewMaster(MasterOpts{
		MasterHandler: &MockMasterHandler{},
	})

	_, err := master.Exists(ctx, "1")
	assert.Error(t, err)
}

func TestMasterRequestNoStart(t *testing.T) {
	ctx := context.Background()
	master := NewMaster(MasterOpts{
		MasterHandler: &MockMasterHandler{},
	})

	_, err := master.Request(ctx, "1", 0)
	assert.Error(t, err)
}

func TestMasterExecuteNoStart(t *testing.T) {
	ctx := context.Background()
	master := NewMaster(MasterOpts{
		MasterHandler: &MockMasterHandler{},
	})

	_, err := master.Execute(ctx, 0)
	assert.Error(t, err)
}

func TestMasterExecuteNoWorkers(t *testing.T) {
	ScopedMaster(t, func(ctx context.Context, master *Master) {
		_, err := master.Execute(ctx, 0)
		assert.Error(t, err)
	})
}

func TestMasterExecuteSingleWorker(t *testing.T) {
	ScopedMaster(t, func(ctx context.Context, master *Master) {
		err := master.Create(ctx, "1", nil)
		assert.Nil(t, err)

		v, err := master.Execute(ctx, 0)
		assert.Nil(t, err)
		assert.Equal(t, 1, v)

		err = master.Destroy(ctx, "1")
		assert.Nil(t, err)
	})
}

func TestMasterBroadcastNoWorkers(t *testing.T) {
	ScopedMaster(t, func(ctx context.Context, master *Master) {
		res, err := master.Broadcast(ctx, 0)
		assert.Nil(t, err)
		assert.Error(t, res[0].Error)
	})
}

func TestMasterBroadcastSingleWorker(t *testing.T) {
	ScopedMaster(t, func(ctx context.Context, master *Master) {
		err := master.Create(ctx, "1", nil)
		assert.Nil(t, err)

		res, err := master.Broadcast(ctx, 0)
		assert.Nil(t, err)
		assert.Nil(t, res[0].Error)
		assert.Equal(t, 1, res[0].Value)

		err = master.Destroy(ctx, "1")
		assert.Nil(t, err)
	})
}

func TestMasterBroadcastMultipleWorkers(t *testing.T) {
	ScopedMaster(t, func(ctx context.Context, master *Master) {
		for i := 0; i < 10; i++ {
			err := master.Create(ctx, fmt.Sprintf("%d", i), nil)
			assert.Nil(t, err)
		}

		res, err := master.Broadcast(ctx, 0)
		assert.Nil(t, err)
		for i := 0; i < 10; i++ {
			assert.Nil(t, res[i].Error)
			assert.Equal(t, 1, res[i].Value)
		}

		for i := 0; i < 10; i++ {
			err = master.Destroy(ctx, fmt.Sprintf("%d", i))
			assert.Nil(t, err)
		}
	})
}

func BenchmarkMasterExecuteMultipleWorkers(b *testing.B) {
	ScopedMaster(b, func(ctx context.Context, master *Master) {
		for i := 0; i < 16; i++ {
			id := fmt.Sprintf("%d", i)
			err := master.Create(ctx, id, nil)
			assert.Nil(b, err)
			defer func() {
				err := master.Destroy(ctx, id)
				assert.Nil(b, err)
			}()
		}

		for i := 0; i < b.N; i++ {
			_, err := master.Execute(ctx, i)
			if err != nil {
				b.FailNow()
			}
		}
	})
}

func BenchmarkMasterRequestMultipleWorkers(b *testing.B) {
	ScopedMaster(b, func(ctx context.Context, master *Master) {
		ids := make(map[int]string)
		workers := 16
		for i := 0; i < workers; i++ {
			id := fmt.Sprintf("%d", i)
			ids[i] = id
			err := master.Create(ctx, id, nil)
			assert.Nil(b, err)
			defer func() {
				err := master.Destroy(ctx, id)
				assert.Nil(b, err)
			}()
		}

		for i := 0; i < b.N; i++ {
			id := ids[i%workers]
			_, err := master.Request(ctx, id, i)
			if err != nil {
				b.FailNow()
			}
		}
	})
}

func TestMasterExistsDoesNotBlockWhenLoopExited(t *testing.T) {
	startCtx, cancelStart := context.WithCancel(context.Background())
	master := NewMaster(MasterOpts{
		MasterHandler: &MockMasterHandler{},
	})

	assert.Nil(t, master.Start(startCtx))

	// stopping the context of the master makes the event loop exit
	cancelStart()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		// the event loop may still accept requests for a short time
		// after cancelStart, so keep sending until one fails
		for {
			if _, err := master.Exists(ctx, "1"); err != nil {
				done <- err
				return
			}
		}
	}()

	select {
	case err := <-done:
		// the event loop closes its done channel when it exits, so
		// callers learn that the master stopped
		assert.Equal(t, ErrMasterStopped, err)
	case <-time.After(10 * time.Second):
		assert.Fail(t, "Exists blocked after the master event loop exited")
	}

	assert.Nil(t, master.Stop())
}

// recordingMasterHandler creates workers with the provided options and
// records the destroy events
type recordingMasterHandler struct {
	mu        sync.Mutex
	opts      func(key string, opts *CreateWorkerOpts)
	destroyed []DestroyWorkerEvent
}

func (h *recordingMasterHandler) Handle(ctx context.Context, ev MasterEvent) error {
	switch ev := ev.(type) {
	case CreateWorkerEvent:
		ev.Opts.WorkerHandler = &MockWorkerHandler{}
		if h.opts != nil {
			h.opts(ev.Key, ev.Opts)
		}
	case DestroyWorkerEvent:
		h.mu.Lock()
		h.destroyed = append(h.destroyed, ev)
		h.mu.Unlock()
	}
	return nil
}

func (h *recordingMasterHandler) Destroyed() []DestroyWorkerEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]DestroyWorkerEvent(nil), h.destroyed...)
}

// within fails the test if fn does not return before the timeout
func within(t *testing.T, timeout time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("operation blocked")
	}
}

// eventually polls cond until it is true or the timeout expires
func eventually(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return cond()
}

func TestMasterWorkerExpiresWhenIdle(t *testing.T) {
	ctx := context.Background()
	handler := &recordingMasterHandler{opts: func(key string, opts *CreateWorkerOpts) {
		opts.MaxInactivity = 20 * time.Millisecond
	}}
	master := NewMaster(MasterOpts{MasterHandler: handler})
	assert.Nil(t, master.Start(ctx))

	assert.Nil(t, master.Create(ctx, "1", nil))
	v, err := master.Request(ctx, "1", 0)
	assert.Nil(t, err)
	assert.Equal(t, 1, v)

	assert.True(t, eventually(5*time.Second, func() bool {
		ok, err := master.Exists(ctx, "1")
		assert.Nil(t, err)
		return !ok
	}), "idle worker did not expire")

	destroyed := handler.Destroyed()
	if assert.Len(t, destroyed, 1) {
		assert.Equal(t, "1", destroyed[0].Key)
		assert.Nil(t, destroyed[0].Cause)
	}

	// the key can be used again after the worker expired
	assert.Nil(t, master.Create(ctx, "1", nil))
	assert.Nil(t, master.Stop())
	assert.Len(t, handler.Destroyed(), 2)
}

func TestMasterWorkerActivityResetsInactivity(t *testing.T) {
	ctx := context.Background()
	handler := &recordingMasterHandler{opts: func(key string, opts *CreateWorkerOpts) {
		opts.MaxInactivity = 200 * time.Millisecond
	}}
	master := NewMaster(MasterOpts{MasterHandler: handler})
	assert.Nil(t, master.Start(ctx))
	defer func() { assert.Nil(t, master.Stop()) }()

	assert.Nil(t, master.Create(ctx, "1", nil))

	// keep the worker busy for longer than its maximum inactivity
	deadline := time.Now().Add(600 * time.Millisecond)
	for time.Now().Before(deadline) {
		_, err := master.Request(ctx, "1", 0)
		assert.Nil(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	ok, err := master.Exists(ctx, "1")
	assert.Nil(t, err)
	assert.True(t, ok)
	assert.Len(t, handler.Destroyed(), 0)
}

func TestMasterWorkerWithoutInactivityDoesNotExpire(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		ctx := context.Background()
		handler := &recordingMasterHandler{opts: func(key string, opts *CreateWorkerOpts) {
			opts.MaxInactivity = d
		}}
		master := NewMaster(MasterOpts{MasterHandler: handler})
		assert.Nil(t, master.Start(ctx))

		assert.Nil(t, master.Create(ctx, "1", nil))
		time.Sleep(20 * time.Millisecond)

		ok, err := master.Exists(ctx, "1")
		assert.Nil(t, err)
		assert.True(t, ok)
		assert.Nil(t, master.Stop())
	}
}

func TestMasterWorkerErrorIsDestroyCause(t *testing.T) {
	ctx := context.Background()
	errC := make(chan error, 1)
	handler := &recordingMasterHandler{opts: func(key string, opts *CreateWorkerOpts) {
		opts.ErrC = errC
	}}
	master := NewMaster(MasterOpts{MasterHandler: handler})
	assert.Nil(t, master.Start(ctx))
	defer func() { assert.Nil(t, master.Stop()) }()

	assert.Nil(t, master.Create(ctx, "1", nil))

	// MockWorkerHandler returns the error of an ErrorWorkerEvent, which
	// makes the worker exit
	cause := errors.New("connection lost")
	errC <- cause

	assert.True(t, eventually(5*time.Second, func() bool {
		ok, err := master.Exists(ctx, "1")
		assert.Nil(t, err)
		return !ok
	}), "worker did not exit on error")

	destroyed := handler.Destroyed()
	if assert.Len(t, destroyed, 1) {
		assert.Equal(t, cause, destroyed[0].Cause)
	}

	_, err := master.Request(ctx, "1", 0)
	assert.Error(t, err)
}

func TestMasterRequestToExitingWorkerDoesNotBlock(t *testing.T) {
	ctx := context.Background()
	errC := make(chan error, 1)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once

	handler := MasterHandlerFunc(func(ctx context.Context, ev MasterEvent) error {
		if ev, ok := ev.(CreateWorkerEvent); ok {
			ev.Opts.ErrC = errC
			ev.Opts.WorkerHandler = WorkerHandlerFunc(func(ctx context.Context, ev WorkerEvent) (interface{}, error) {
				switch ev := ev.(type) {
				case RequestWorkerEvent:
					once.Do(func() {
						close(entered)
						<-release
					})
					return ev.Value, nil
				case ErrorWorkerEvent:
					return nil, ev.Error
				}
				return nil, nil
			})
		}
		return nil
	})
	master := NewMaster(MasterOpts{MasterHandler: handler})
	assert.Nil(t, master.Start(ctx))
	defer func() { assert.Nil(t, master.Stop()) }()

	assert.Nil(t, master.Create(ctx, "1", nil))

	first := make(chan error, 1)
	go func() {
		_, err := master.Request(ctx, "1", 1)
		first <- err
	}()
	<-entered

	// queue a request while the worker is busy, then make the worker
	// exit. The worker either handles the queued request or exits
	// before it, in which case the master answers it
	second := make(chan error, 1)
	go func() {
		_, err := master.Request(ctx, "1", 2)
		second <- err
	}()
	errC <- errors.New("exit")
	close(release)

	within(t, 5*time.Second, func() {
		assert.Nil(t, <-first)
		// the second request may also reach the master after the
		// worker has been removed
		if err := <-second; err != nil && err != ErrWorkerExited {
			assert.Equal(t, "worker does not exist", err.Error())
		}
	})

	assert.True(t, eventually(5*time.Second, func() bool {
		ok, err := master.Exists(ctx, "1")
		assert.Nil(t, err)
		return !ok
	}))

	within(t, 5*time.Second, func() {
		_, err := master.Request(ctx, "1", 3)
		assert.Error(t, err)
	})
}

func TestMasterDispatchToExitedWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	handler := &recordingMasterHandler{}
	master := NewMaster(MasterOpts{MasterHandler: handler})
	run := newMasterRun(context.Background())

	// a worker that exits straight away. Its notification is
	// held in doneCh until the master removes it
	w := NewWorker(ctx, WorkerOpts{
		Key:           "1",
		DoneC:         run.doneCh,
		WorkerHandler: &MockWorkerHandler{},
		C:             make(chan workerRequest),
		SharedC:       run.sharedCh,
	})
	master.workers["1"] = w
	master.workerCount.Add(1)
	cancel()
	<-w.done

	// the worker cannot accept the request on its unbuffered channel
	out := make(chan Response, 1)
	count := int32(1)
	within(t, 5*time.Second, func() {
		master.dispatch(w, workerRequest{Context: context.Background(), Key: "1", Out: out, Count: &count})
	})
	assert.Equal(t, ErrWorkerExited, (<-out).Error)

	// a request that was queued for the worker before it exited is
	// answered when the master removes the worker
	w.C = make(chan workerRequest, 1)
	out = make(chan Response, 1)
	count = int32(1)
	w.C <- workerRequest{Context: context.Background(), Key: "1", Out: out, Count: &count}
	master.removeWorker(run, <-run.doneCh)

	assert.Equal(t, ErrWorkerExited, (<-out).Error)
	assert.Len(t, master.workers, 0)
	assert.Len(t, handler.Destroyed(), 1)
	within(t, 5*time.Second, master.workerCount.Wait)
}

func TestMasterCallsConcurrentWithStop(t *testing.T) {
	for i := 0; i < 20; i++ {
		ctx := context.Background()
		master := NewMaster(MasterOpts{MasterHandler: &MockMasterHandler{}})
		assert.Nil(t, master.Start(ctx))

		var wg sync.WaitGroup
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				key := fmt.Sprintf("%d", g%4)
				for {
					// errors are expected while the master stops, the
					// calls must only not panic or block
					_ = master.Create(ctx, key, nil)
					_, _ = master.Request(ctx, key, g)
					_, _ = master.Execute(ctx, g)
					_, _ = master.Broadcast(ctx, g)
					_ = master.Destroy(ctx, key)
					if _, err := master.Exists(ctx, key); err != nil {
						return
					}
				}
			}(g)
		}

		time.Sleep(time.Millisecond)
		within(t, 10*time.Second, func() {
			assert.Nil(t, master.Stop())
			wg.Wait()
		})
	}
}

func TestMasterCallsAfterContextCancellation(t *testing.T) {
	startCtx, cancelStart := context.WithCancel(context.Background())
	handler := &MockMasterHandler{}
	master := NewMaster(MasterOpts{MasterHandler: handler})
	assert.Nil(t, master.Start(startCtx))

	ctx := context.Background()
	assert.Nil(t, master.Create(ctx, "1", nil))

	cancelStart()

	isStopped := func(err error) bool {
		return err == ErrMasterStopped || err == context.Canceled
	}

	within(t, 10*time.Second, func() {
		// the event loop may still accept a request right after the
		// cancellation, so wait until it reports that it stopped
		assert.True(t, eventually(5*time.Second, func() bool {
			_, err := master.Exists(ctx, "1")
			return isStopped(err)
		}))

		err := master.Create(ctx, "2", nil)
		assert.True(t, isStopped(err), "%v", err)
		err = master.Destroy(ctx, "1")
		assert.True(t, isStopped(err), "%v", err)
		_, err = master.Request(ctx, "1", 0)
		assert.True(t, isStopped(err), "%v", err)
		_, err = master.Broadcast(ctx, 0)
		assert.True(t, isStopped(err), "%v", err)
		_, err = master.Execute(ctx, 0)
		assert.True(t, isStopped(err), "%v", err)

		// the master needs to be stopped before it can be started again
		assert.Error(t, master.Start(ctx))
		assert.Nil(t, master.Stop())
	})

	assert.Equal(t, 1, handler.Created())
	assert.Equal(t, 1, handler.Destroyed())
	assert.True(t, master.IsStopped())
}

func TestMasterCallerContextCancelled(t *testing.T) {
	ScopedMaster(t, func(ctx context.Context, master *Master) {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()

		within(t, 5*time.Second, func() {
			err := master.Create(cancelled, "1", nil)
			assert.Equal(t, context.Canceled, err)
		})
	})
}

func TestMasterStartStopStart(t *testing.T) {
	ctx := context.Background()
	handler := &MockMasterHandler{}
	master := NewMaster(MasterOpts{MasterHandler: handler})

	for i := 0; i < 3; i++ {
		assert.Nil(t, master.Start(ctx))
		assert.False(t, master.IsStopped())

		assert.Nil(t, master.Create(ctx, "1", nil))
		v, err := master.Request(ctx, "1", i)
		assert.Nil(t, err)
		assert.Equal(t, i+1, v)

		assert.Nil(t, master.Stop())
		assert.True(t, master.IsStopped())

		_, err = master.Request(ctx, "1", i)
		assert.Error(t, err)
	}

	assert.Equal(t, 3, handler.Created())
	assert.Equal(t, 3, handler.Destroyed())
}

func TestMasterExecuteWhenLastWorkerExits(t *testing.T) {
	ctx := context.Background()
	handler := &recordingMasterHandler{opts: func(key string, opts *CreateWorkerOpts) {
		opts.MaxInactivity = 10 * time.Millisecond
	}}
	master := NewMaster(MasterOpts{MasterHandler: handler})
	assert.Nil(t, master.Start(ctx))
	defer func() { assert.Nil(t, master.Stop()) }()

	assert.Nil(t, master.Create(ctx, "1", nil))

	// the requests either run on the worker or fail once it expires,
	// they never block
	within(t, 10*time.Second, func() {
		for i := 0; i < 50; i++ {
			v, err := master.Execute(ctx, i)
			if err == nil {
				assert.Equal(t, i+1, v)
			}
			time.Sleep(time.Millisecond)
		}
	})
}
