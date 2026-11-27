package concurrent

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

const (
	stopped  = 0
	started  = 1
	stopping = 2
)

func errorFromPanic(r interface{}) error {
	stacktrace := debug.Stack()

	switch x := r.(type) {
	case string:
		return fmt.Errorf("panic error %s at %s", x, string(stacktrace))
	case error:
		return fmt.Errorf("panic error %s at %s", x.Error(), string(stacktrace))
	default:
		return fmt.Errorf("unknown panic %+v at %s", r, string(stacktrace))
	}
}

// MasterEvent is the interface implemented by all events triggered
// by the master and handled for a MasterHandler
type MasterEvent interface {
	WorkerKey() string
}

type createRequest struct {
	Context context.Context
	Key     string
	Out     chan Response
	Value   interface{}
}

func (r createRequest) GetContext() context.Context {
	return r.Context
}

func (r createRequest) WorkerKey() string {
	return r.Key
}

type destroyRequest struct {
	Context context.Context
	Key     string
	Out     chan Response
}

func (r destroyRequest) GetContext() context.Context {
	return r.Context
}

func (r destroyRequest) WorkerKey() string {
	return r.Key
}

type existsRequest struct {
	Context context.Context
	Key     string
	Out     chan Response
}

func (r existsRequest) GetContext() context.Context {
	return r.Context
}

func (r existsRequest) WorkerKey() string {
	return r.Key
}

type request interface {
	GetContext() context.Context
}

// masterRun holds the channels used by the master during a single
// Start-Stop span. A new instance is created on every Start so that
// goroutines holding on to a previous run never observe the channels
// of a newer one
type masterRun struct {
	// ctx is the context that the master uses for the duration
	// of its Start-Stop span
	ctx context.Context

	// shutdownCh is closed by Stop to signal a shutdown to the
	// event loop
	shutdownCh chan struct{}

	// done is closed by the event loop when it exits, for any reason,
	// after all the workers have exited
	done chan struct{}

	// doneCh is the channel used by workers to notify to the
	// Master that their lifetime has ended
	doneCh chan workerDestroyed

	// sharedCh is a shared channel between the master and
	// its workers. This channel can be used by the master to
	// send a request to any worker. When a request is send through
	// this channel, any worker that is available can pick it up
	sharedCh chan executeRequest

	// inCh is the channel used by the master to pass on requests
	// from external goroutines to the event loop
	inCh chan request
}

func newMasterRun(ctx context.Context) *masterRun {
	return &masterRun{
		ctx:        ctx,
		shutdownCh: make(chan struct{}),
		done:       make(chan struct{}),
		doneCh:     make(chan workerDestroyed, 64),
		sharedCh:   make(chan executeRequest, 64),
		inCh:       make(chan request),
	}
}

// Master manages a set of workers and distributes workers
// amongst them. It also keeps track of the workers lifetimes.
//
// All the state of the master, other than the run state, is owned
// by its event loop, which is the only goroutine that reads or
// modifies it
type Master struct {
	// createWorkerOnRequest creates a worker if a request is received by
	// a worker and the worker was not created beforehand
	createWorkerOnRequest bool

	// workerCount to keep track of the number of workers and
	// ensure a graceful shutdown of the master and its workers
	workerCount sync.WaitGroup

	// communication channels that the master has with the
	// workers. The master uses the channels as write only and
	// the workers use the channels as read only. The workers
	// communicate back with the master with a channel created
	// on a per request basis
	workers map[string]*Worker

	// shutdownWorkers are the workers that are shutting down
	// and we are waiting for a doneCh event. Workers are tracked
	// by identity so that a worker can be created again with the
	// same key while the previous one is still shutting down
	shutdownWorkers map[*Worker]struct{}

	// mu protects the transitions of state and run
	mu sync.RWMutex

	// state keeps track of whether the master is running. It
	// is modified while holding mu and can be read atomically
	state uint32

	// run holds the channels of the current Start-Stop span
	run *masterRun

	// handler is the user defined handler for events that
	// need to be handled by the master
	handler MasterHandler

	// Error is set in case of exiting with an error
	Error error
}

// MasterHandler is the user defined handler to handle events
// for the master
type MasterHandler interface {
	Handle(ctx context.Context, ev MasterEvent) error
}

// MasterHandlerFunc is the implementation of MasterHandler for functions
type MasterHandlerFunc func(ctx context.Context, ev MasterEvent) error

// Handle implementation of MasterHandler for MasterHandlerFunc
func (f MasterHandlerFunc) Handle(ctx context.Context, ev MasterEvent) error {
	return f(ctx, ev)
}

// MasterOpts are the properties used by the master to define
// its behaviour and that of its workers
type MasterOpts struct {
	// MasterHandler is the handler the master will use to provide access
	// to the master events
	MasterHandler MasterHandler

	// CreateWorkerOnRequest creates a worker if a request is received by
	// a worker and the worker was not created beforehand. Should only be
	// used if a worker does not need a specific request passed on to the
	// CreateWorkerEvent handler
	CreateWorkerOnRequest bool
}

// NewMaster creates a new master
func NewMaster(opts MasterOpts) *Master {
	return &Master{
		createWorkerOnRequest: opts.CreateWorkerOnRequest,
		handler:               opts.MasterHandler,
		workers:               make(map[string]*Worker),
		shutdownWorkers:       make(map[*Worker]struct{}),
		state:                 stopped,
	}
}

// IsStopped returns true if the master is not running
func (m *Master) IsStopped() bool {
	return atomic.LoadUint32(&m.state) == stopped
}

// Start the master. The master runs until Stop is called or ctx
// is done. In the latter case the master stops serving requests,
// which fail with ErrMasterStopped, but Stop still needs to be called
// before the master can be started again
func (m *Master) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if atomic.LoadUint32(&m.state) != stopped {
		return errors.New("master is not stopped")
	}

	// the previous event loop, if any, has exited, so the event loop
	// state can be safely reset before the new one starts
	run := newMasterRun(ctx)
	m.run = run
	m.workers = make(map[string]*Worker)
	m.shutdownWorkers = make(map[*Worker]struct{})
	atomic.StoreUint32(&m.state, started)

	go m.startLoop(run)
	return nil
}

// Stop the master and shutdown all the workers that are still running.
// This method blocks until all the workers have exited
func (m *Master) Stop() error {
	m.mu.Lock()
	if atomic.LoadUint32(&m.state) != started {
		m.mu.Unlock()
		return errors.New("master is not started")
	}
	atomic.StoreUint32(&m.state, stopping)
	run := m.run
	m.mu.Unlock()

	// the transition from started to stopping happens only once per
	// run, so shutdownCh is closed only once
	close(run.shutdownCh)
	<-run.done

	m.mu.Lock()
	atomic.StoreUint32(&m.state, stopped)
	m.mu.Unlock()

	return nil
}

// currentRun returns the run of the master if the master is started
func (m *Master) currentRun() (*masterRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if atomic.LoadUint32(&m.state) != started {
		return nil, errors.New("master is not started")
	}

	return m.run, nil
}

// Create a new worker
func (m *Master) Create(ctx context.Context, key string, value interface{}) error {
	run, err := m.currentRun()
	if err != nil {
		return err
	}

	out := make(chan Response, 1)
	if err := run.send(ctx, createRequest{Context: ctx, Key: key, Out: out, Value: value}); err != nil {
		return err
	}
	res, _, err := run.wait(ctx, out)
	if err != nil {
		return err
	}
	return res.Error
}

// Destroy an existing worker
func (m *Master) Destroy(ctx context.Context, key string) error {
	run, err := m.currentRun()
	if err != nil {
		return err
	}

	out := make(chan Response, 1)
	if err := run.send(ctx, destroyRequest{Context: ctx, Key: key, Out: out}); err != nil {
		return err
	}
	res, _, err := run.wait(ctx, out)
	if err != nil {
		return err
	}
	if res.Error != nil {
		return res.Error
	}

	// wait for the worker to destroy
	return run.waitShutdown(ctx, res.Value.(<-chan error))
}

// Exists returns true if the worker exists, false otherwise
func (m *Master) Exists(ctx context.Context, key string) (bool, error) {
	run, err := m.currentRun()
	if err != nil {
		return false, err
	}

	out := make(chan Response, 1)
	if err := run.send(ctx, existsRequest{Context: ctx, Key: key, Out: out}); err != nil {
		return false, err
	}
	res, _, err := run.wait(ctx, out)
	if err != nil {
		return false, err
	}
	return res.Value.(bool), nil
}

// Request sends a request to a specific worker and returns back
// the response
func (m *Master) Request(ctx context.Context, key string, req interface{}) (interface{}, error) {
	run, err := m.currentRun()
	if err != nil {
		return nil, err
	}

	out := make(chan Response, 1)
	count := int32(1)
	if err := run.send(ctx, workerRequest{
		Context: ctx,
		Key:     key,
		Value:   req,
		Out:     out,
		Count:   &count,
	}); err != nil {
		return nil, err
	}
	res, _, err := run.wait(ctx, out)
	if err != nil {
		return nil, err
	}
	return res.Value, res.Error
}

// Broadcast sends the same request to all workers and waits until
// a response from each is received
func (m *Master) Broadcast(ctx context.Context, req interface{}) ([]Response, error) {
	run, err := m.currentRun()
	if err != nil {
		return nil, err
	}

	out := make(chan Response, 1)
	if err := run.send(ctx, broadcastRequest{Context: ctx, Value: req, Out: out}); err != nil {
		return nil, err
	}

	// the master replies with the channel on which the workers
	// send their responses
	res, _, err := run.wait(ctx, out)
	if err != nil {
		return nil, err
	}
	results := res.Value.(<-chan Response)

	var responses []Response
	for {
		res, ok, err := run.wait(ctx, results)
		if err != nil {
			return nil, err
		}
		if !ok {
			return responses, nil
		}
		responses = append(responses, res)
	}
}

// Execute sends a request that will be caught by any worker which
// is available and execute it
func (m *Master) Execute(ctx context.Context, req interface{}) (interface{}, error) {
	run, err := m.currentRun()
	if err != nil {
		return nil, err
	}

	out := make(chan Response, 1)
	if err := run.send(ctx, executeRequest{Context: ctx, Value: req, Out: out}); err != nil {
		return nil, err
	}
	res, _, err := run.wait(ctx, out)
	if err != nil {
		return nil, err
	}
	return res.Value, res.Error
}

// send passes a request to the event loop. It returns an error instead
// of blocking forever if the context of the request is done or the
// event loop has exited before accepting the request
func (r *masterRun) send(ctx context.Context, req request) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.done:
		return ErrMasterStopped
	case r.inCh <- req:
		return nil
	}
}

// wait blocks until a response is received on out, the context
// is done or the event loop has exited. ok is false if out has
// been closed. Reply channels are buffered, so neither the master
// nor the workers block on replying to a caller that left
func (r *masterRun) wait(ctx context.Context, out <-chan Response) (res Response, ok bool, err error) {
	select {
	case res, ok = <-out:
		return res, ok, nil
	case <-ctx.Done():
		return Response{}, false, ctx.Err()
	case <-r.done:
		// all the replies are sent before the event loop exits, so
		// a reply that is already available takes precedence
		select {
		case res, ok = <-out:
			return res, ok, nil
		default:
			return Response{}, false, ErrMasterStopped
		}
	}
}

// waitShutdown waits until a worker that is shutting down has been
// removed by the event loop and returns the error reported by the
// MasterHandler on the DestroyWorkerEvent, if any
func (r *masterRun) waitShutdown(ctx context.Context, c <-chan error) error {
	var (
		err error
		ok  bool
	)

	select {
	case err, ok = <-c:
	case <-ctx.Done():
		return ctx.Err()
	case <-r.done:
		// the event loop removes all the workers before exiting
		select {
		case err, ok = <-c:
		default:
			return ErrMasterStopped
		}
	}

	if ok && err != nil {
		return err
	}
	return nil
}

// reply sends the response of a request to a worker on its buffered
// output channel and closes it once all the expected responses have
// been sent
func (r workerRequest) reply(res Response) {
	r.Out <- res
	if value := atomic.AddInt32(r.Count, -1); value == 0 {
		close(r.Out)
	}
}

// shutdown closes all the workers and frees the resources
// they are using. This method should only be called from
// the event loop when it exits
func (m *Master) shutdown(run *masterRun) {
	// shutdown all the workers.
	for key := range m.workers {
		m.shutdownWorker(key)
	}

	// remove all workers that have been dismissed as they
	// notify the master
	for len(m.shutdownWorkers) > 0 {
		m.removeWorker(run, <-run.doneCh)
	}

	m.workerCount.Wait()

	// requests that may still be pending on the shared channel
	// will not be picked up by any worker
	m.drainShared(run, ErrMasterStopped)

	if len(m.workers) > 0 || len(m.shutdownWorkers) > 0 {
		panic("failed to shutdown all workers gracefully")
	}
}

func (m *Master) shutdownWorker(key string) (<-chan error, bool) {
	w, ok := m.workers[key]
	if !ok {
		return nil, false
	}

	// remove the worker from the set of active workers and move it to the
	// set of workers which are being shutdown
	delete(m.workers, key)
	m.shutdownWorkers[w] = struct{}{}
	close(w.C)
	return w.ShutdownC, true
}

func (m *Master) startLoop(run *masterRun) {
	defer func() {
		if r := recover(); r != nil {
			panic(errorFromPanic(r))
		}

		m.shutdown(run)
		close(run.done)
	}()

	for {
		select {
		case <-run.ctx.Done():
			return
		case <-run.shutdownCh:
			return
		case ev := <-run.doneCh:
			m.removeWorker(run, ev)
		case req := <-run.inCh:
			m.handleRequest(run, req)
		}
	}
}

func (m *Master) handleRequest(run *masterRun, req request) {
	switch req := req.(type) {
	case workerRequest:
		m.handleWorkerRequest(run, req)
	case createRequest:
		m.handleCreateRequest(run, req)
	case destroyRequest:
		m.handleDestroyRequest(req)
	case existsRequest:
		m.handleExistsRequest(req)
	case executeRequest:
		m.handleExecuteRequest(run, req)
	case broadcastRequest:
		m.handleBroadcastRequest(req)
	default:
		panic("received unexpected request")
	}
}

// dispatch passes a request to a worker. It does not block if the
// worker has exited or the caller has left
func (m *Master) dispatch(w *Worker, req workerRequest) {
	select {
	case w.C <- req:
	case <-w.done:
		req.reply(Response{Key: w.key, Error: ErrWorkerExited})
	case <-req.Context.Done():
		req.reply(Response{Key: w.key, Error: req.Context.Err()})
	}
}

func (m *Master) handleWorkerRequest(run *masterRun, req workerRequest) {
	w, ok := m.workers[req.Key]
	if !ok && !m.createWorkerOnRequest {
		req.reply(Response{Value: nil, Error: errors.New("worker does not exist")})
		return

	} else if !ok && m.createWorkerOnRequest {
		if err := m.createWorker(run, req.Context, req.Key, nil); err != nil {
			req.reply(Response{Value: nil, Error: err})
			return
		}

		w, ok = m.workers[req.Key]
		if !ok {
			panic("worker had just been added to the list of active workers")
		}
	}

	m.dispatch(w, req)
}

func (m *Master) handleBroadcastRequest(req broadcastRequest) {
	if len(m.workers) == 0 {
		results := make(chan Response, 1)
		results <- Response{Value: nil, Error: errors.New("no workers available to handle the broadcast request")}
		close(results)
		req.Out <- Response{Value: (<-chan Response)(results)}
		return
	}

	// the channel can hold the responses of all workers so that
	// workers never block if the caller leaves
	results := make(chan Response, len(m.workers))
	req.Out <- Response{Value: (<-chan Response)(results)}

	count := int32(len(m.workers))
	for _, w := range m.workers {
		m.dispatch(w, workerRequest{
			Context: req.Context,
			Key:     w.key,
			Value:   req.Value,
			Out:     results,
			Count:   &count,
		})
	}
}

func (m *Master) handleExecuteRequest(run *masterRun, req executeRequest) {
	for {
		if len(m.workers) == 0 {
			req.Out <- Response{Value: nil, Error: errors.New("no workers available to handle the execute request")}
			return
		}

		select {
		case run.sharedCh <- req:
			return
		case <-req.Context.Done():
			req.Out <- Response{Value: nil, Error: req.Context.Err()}
			return
		case ev := <-run.doneCh:
			// the shared channel is full. Keep removing workers that exit
			// meanwhile so that the master does not wait on workers that
			// will never pick up the request
			m.removeWorker(run, ev)
		}
	}
}

func (m *Master) createWorker(run *masterRun, ctx context.Context, key string, value interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errorFromPanic(r)
		}
	}()

	_, ok := m.workers[key]
	if ok {
		err = errors.New("worker already exists")
		return
	}

	var opts CreateWorkerOpts
	err = m.handler.Handle(ctx, CreateWorkerEvent{
		Value: value,
		Key:   key,
		Opts:  &opts,
	})
	if err != nil {
		return
	}

	ch := make(chan workerRequest, 64)
	m.workers[key] = NewWorker(run.ctx, WorkerOpts{
		MaxInactivity: opts.MaxInactivity,
		Key:           key,
		DoneC:         run.doneCh,
		WorkerHandler: opts.WorkerHandler,
		UserData:      opts.UserData,
		ErrC:          opts.ErrC,
		C:             ch,
		SharedC:       run.sharedCh,
	})

	m.workerCount.Add(1)
	return nil
}

func (m *Master) handleCreateRequest(run *masterRun, req createRequest) {
	req.Out <- Response{Key: req.Key, Error: m.createWorker(run, req.Context, req.Key, req.Value)}
}

func (m *Master) handleDestroyRequest(req destroyRequest) {
	c, ok := m.shutdownWorker(req.Key)
	if !ok {
		req.Out <- Response{Key: req.Key, Error: errors.New("worker does not exist"), Value: nil}
		return
	}

	req.Out <- Response{Key: req.Key, Error: nil, Value: c}
}

func (m *Master) handleExistsRequest(req existsRequest) {
	_, ok := m.workers[req.Key]
	req.Out <- Response{Key: req.Key, Value: ok}
}

// drainWorker replies with an error to the requests left on the
// channel of a worker that has exited. It must only be called once
// the worker is no longer reachable by the master, so that no more
// requests are sent to it
func (m *Master) drainWorker(w *Worker) {
	for {
		select {
		case req, ok := <-w.C:
			if !ok {
				return
			}
			req.reply(Response{Key: w.key, Error: ErrWorkerExited})
		default:
			return
		}
	}
}

// drainShared replies with an error to the execute requests left on
// the shared channel when there are no workers to pick them up
func (m *Master) drainShared(run *masterRun, err error) {
	for {
		select {
		case req := <-run.sharedCh:
			req.Out <- Response{Value: nil, Error: err}
		default:
			return
		}
	}
}

// removeWorker removes a worker that has exited, either because the
// master shut it down or because it exited on its own (inactivity,
// error, or context cancellation)
func (m *Master) removeWorker(run *masterRun, ev workerDestroyed) {
	w := ev.Worker
	if w == nil {
		return
	}

	if _, ok := m.shutdownWorkers[w]; ok {
		delete(m.shutdownWorkers, w)
	} else if current, ok := m.workers[ev.Key]; ok && current == w {
		delete(m.workers, ev.Key)
	} else {
		return
	}

	m.workerCount.Done()

	// the worker is not reachable anymore, so any request still
	// queued for it can be answered
	m.drainWorker(w)
	if len(m.workers) == 0 {
		m.drainShared(run, errors.New("no workers available to handle the execute request"))
	}

	if err := m.notifyDestroyed(w, ev); err != nil {
		w.ShutdownC <- err
	}
	close(w.ShutdownC)
}

func (m *Master) notifyDestroyed(w *Worker, ev workerDestroyed) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errorFromPanic(r)
		}
	}()

	return m.handler.Handle(context.Background(), DestroyWorkerEvent{
		Worker: w,
		Key:    ev.Key,
		Cause:  ev.Cause,
	})
}

// Worker handles requests issued by the master in a separate
// goroutine and gives back results. Its lifetime is managed
// by the Master
type Worker struct {
	// maxInactivity is the maximum time the worker is allowed to exist
	// without serving any request. When this time expires the worker
	// should destroy itself. A value <= 0 means that the worker never
	// expires
	maxInactivity time.Duration

	// key is the string that uniquely identifies a worker
	key string

	// handler is the user defined handler for events that
	// a worker needs to handle
	handler WorkerHandler

	// SharedC is the shared channel between the master and the worker
	// for requests met by the worker who is available
	SharedC <-chan executeRequest

	// C is the channel the worker only reads from
	C chan workerRequest

	// ShutdownC is a channel used by the worker to signal that it
	// has been completely shutdown and removed
	ShutdownC chan error

	// ErrC is an error channel the worker can listen to and report errors
	// though events in case they happen
	ErrC <-chan error

	// doneC is a write once channel the worker uses to notify the master
	// that the worker has exited
	doneC chan<- workerDestroyed

	// done is closed when the worker loop exits
	done chan struct{}

	// UserData is data that the user can attach to the worker in case any
	// external context is required
	UserData interface{}
}

// WorkerEvent is the interface defined for events that the worker emits
type WorkerEvent interface {
	GetWorker() *Worker
}

// ErrorWorkerEvent is emitted by the worker when an event on the
// error channel is received
type ErrorWorkerEvent struct {
	Worker *Worker
	Error  error
}

// GetWorker implementation of WorkerEvent for ErrorWorkerEvent
func (e ErrorWorkerEvent) GetWorker() *Worker {
	return e.Worker
}

// RequestWorkerEvent is emitted by the worker when a request
// is received by the worker
type RequestWorkerEvent struct {
	Worker *Worker
	Value  interface{}
}

// GetWorker implementation of WorkerEvent for ErrorWorkerEvent
func (e RequestWorkerEvent) GetWorker() *Worker {
	return e.Worker
}

// WorkerHandler is the user defined handler to handle events
// targeting a worker
type WorkerHandler interface {
	Handle(ctx context.Context, req WorkerEvent) (interface{}, error)
}

// WorkerHandlerFunc is the implementation of MasterHandler for functions
type WorkerHandlerFunc func(ctx context.Context, ev WorkerEvent) (interface{}, error)

// Handle implementation of WorkerHandler for WorkerHandlerFunc
func (f WorkerHandlerFunc) Handle(ctx context.Context, ev WorkerEvent) (interface{}, error) {
	return f(ctx, ev)
}

// CreateWorkerOpts is the place where a user defined MasterHandler can put
// the defined properties for a Worker on a CreateWorkerEvent
type CreateWorkerOpts struct {
	// WorkerHandler is the handler used by the worker to handle
	// incoming requests
	WorkerHandler WorkerHandler

	// ErrC is an error channel the worker can listen to and report errors
	// though events in case they happen. If the WorkerHandler returns an
	// error for an ErrorWorkerEvent the worker exits, and the error is
	// reported as the Cause of the DestroyWorkerEvent
	ErrC <-chan error

	// UserData is data that the user can attach to the worker in case any
	// external context is required
	UserData interface{}

	// MaxInactivity is the maximum time the worker is allowed to exist
	// without serving any request. When this time expires the worker
	// destroys itself. A value <= 0 means that the worker never expires
	MaxInactivity time.Duration
}

// CreateWorkerEvent is triggered by a master when a new worker
// is created and available to be sent events to
type CreateWorkerEvent struct {
	Key   string
	Value interface{}
	Opts  *CreateWorkerOpts
}

// WorkerKey implementation of MasterEvent for CreateWorkerEvent
func (e CreateWorkerEvent) WorkerKey() string {
	return e.Key
}

// DestroyWorkerEvent is triggered by a master when an existing worker
// is destroyed
type DestroyWorkerEvent struct {
	Worker *Worker
	Key    string

	// Cause is set if the worker exited because of an error, either
	// returned by its WorkerHandler for an ErrorWorkerEvent or caused
	// by a panic in the worker loop
	Cause error
}

// WorkerKey implementation of MasterEvent for DestroyWorkerEvent
func (e DestroyWorkerEvent) WorkerKey() string {
	return e.Key
}

// WorkerOpts are the properties used to construct a worker instance
type WorkerOpts struct {
	// Key that uniquely identifies the worker
	Key string

	// DoneC is a write once channel the worker uses to notify the master
	// that the worker has exited
	DoneC chan<- workerDestroyed

	// WorkerHandler is the handler used by the worker to handle
	// incoming requests
	WorkerHandler WorkerHandler

	// C is the channel the worker gets requests from
	C chan workerRequest

	// ErrC is an error channel the worker can listen to and report errors
	// though events in case they happen
	ErrC <-chan error

	// SharedC is the shared channel between the master and the worker
	// for requests met by the worker who is available
	SharedC <-chan executeRequest

	// UserData is data that the user can attach to the worker in case any
	// external context is required
	UserData interface{}

	// MaxInactivity is the maximum time the worker is allowed to exist
	// without serving any request. When this time expires the worker
	// destroys itself. A value <= 0 means that the worker never expires
	MaxInactivity time.Duration
}

// workerDestroyed is the event sent by a worker to the
// master to signal the end of the worker. If the worker
// was shutdown because of an error Cause may be set
type workerDestroyed struct {
	Context context.Context

	// Key uniquely identifies a worker
	Key string

	// Worker is the worker that exited
	Worker *Worker

	// Cause may be set by the worker if the conditions in
	// which it terminated were abnormal
	Cause error
}

// Response is the result of a request handled by a worker
type Response struct {
	Value interface{}
	Key   string
	Error error
}

type workerRequest struct {
	Context context.Context
	Key     string
	Value   interface{}
	Out     chan Response
	Count   *int32
}

type broadcastRequest struct {
	Context context.Context
	Value   interface{}
	Out     chan Response
}

type executeRequest struct {
	Context context.Context
	Value   interface{}
	Out     chan Response
}

func (r workerRequest) GetContext() context.Context {
	return r.Context
}

func (r broadcastRequest) GetContext() context.Context {
	return r.Context
}

func (r executeRequest) GetContext() context.Context {
	return r.Context
}

// NewWorker creates a new worker instance
func NewWorker(ctx context.Context, opts WorkerOpts) *Worker {
	w := &Worker{
		maxInactivity: opts.MaxInactivity,
		key:           opts.Key,
		handler:       opts.WorkerHandler,
		SharedC:       opts.SharedC,
		C:             opts.C,

		// ShutdownC may be closed with an error if there are no listeners
		// for it. In that case we should not block
		ShutdownC: make(chan error, 2),
		ErrC:      opts.ErrC,
		doneC:     opts.DoneC,
		done:      make(chan struct{}),
		UserData:  opts.UserData,
	}

	go w.startLoop(ctx)
	return w
}

// resetTimer restarts the inactivity timer. The timer must not
// have been received from after its last reset
func resetTimer(timer *time.Timer, d time.Duration) {
	if !timer.Stop() {
		// drain the channel in case the timer fired and the
		// value has not been received yet
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(d)
}

func (w *Worker) startLoop(ctx context.Context) {
	var (
		err     error
		timer   *time.Timer
		expired <-chan time.Time
	)

	// a nil expired channel blocks forever, so a worker without
	// maxInactivity never expires
	if w.maxInactivity > 0 {
		timer = time.NewTimer(w.maxInactivity)
		expired = timer.C
	}

	defer func() {
		if timer != nil {
			timer.Stop()
		}

		if r := recover(); r != nil {
			err = errorFromPanic(r)
		}

		close(w.done)
		w.doneC <- workerDestroyed{Context: ctx, Key: w.key, Worker: w, Cause: err}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-expired:
			return
		case cause, ok := <-w.ErrC:
			if !ok {
				return
			}

			// a handler error ends the worker and is reported
			// as the cause of its destruction
			err = w.handleError(cause)
			if err != nil {
				return
			}

		case req, ok := <-w.SharedC:
			if !ok {
				return
			}

			w.handleExecute(req)
			if timer != nil {
				resetTimer(timer, w.maxInactivity)
			}
		case req, ok := <-w.C:
			if !ok {
				return
			}

			w.handleRequest(req)
			if timer != nil {
				resetTimer(timer, w.maxInactivity)
			}
		}
	}
}

func (w *Worker) handleError(req error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errorFromPanic(r)
		}
	}()

	_, err = w.handler.Handle(context.Background(), ErrorWorkerEvent{
		Worker: w,
		Error:  req,
	})

	return err
}

func (w *Worker) handleExecute(req executeRequest) {
	count := int32(1)
	w.handleRequest(workerRequest{
		Context: req.Context,
		Key:     w.key,
		Value:   req.Value,
		Out:     req.Out,
		Count:   &count,
	})
}

func (w *Worker) processRequest(req workerRequest) Response {
	if req.Key != w.key {
		panic("received request intended for another worker")
	}

	v, err := w.handler.Handle(req.Context, RequestWorkerEvent{
		Worker: w,
		Value:  req.Value,
	})

	return Response{Value: v, Key: w.key, Error: err}
}

func (w *Worker) handleRequest(req workerRequest) {
	defer func() {
		if r := recover(); r != nil {
			req.reply(Response{Value: nil, Key: w.key, Error: errorFromPanic(r)})
		}
	}()

	req.reply(w.processRequest(req))
}
