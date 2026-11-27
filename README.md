# octopus

[![CI](https://github.com/eaugeas/octopus/actions/workflows/ci.yml/badge.svg)](https://github.com/eaugeas/octopus/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/eaugeas/octopus.svg)](https://pkg.go.dev/github.com/eaugeas/octopus)

octopus is a small Go library of building blocks for backend services: bounded
concurrency primitives (worker pools, batches, retries with capped exponential
backoff and a master/worker supervisor), ordered containers (red-black and
unbalanced binary search trees, integer interval sets), a minimal HTTP RPC layer
that binds typed handlers to routes with JSON decoding and trace IDs,
structured logging on top of logrus, and flag/env/file configuration on top of
cobra and viper. Every concurrency primitive is context-aware and tested under
the race detector.

```sh
go get github.com/eaugeas/octopus
```

## Packages

| Package | Description |
| --- | --- |
| [`concurrent`](concurrent) | `PoolRunner` (fixed set of goroutines that run submitted work), `BatchRunner` / `BatchSlice` (run a set of operations with bounded concurrency and collect ordered results), `Retry` (capped exponential backoff with optional jitter) and `Master` / `Worker` (keyed, long-lived workers with request, broadcast and execute-on-any semantics, inactivity expiry and error propagation). |
| [`container/tree`](container/tree) | Binary search trees with a pluggable comparator: a self-balancing red-black tree and an unbalanced tree, with ordered walks, `Higher` / `Lower` queries and duplicates support. |
| [`container/interval`](container/interval) | Immutable integer intervals and an `IntSet` that merges overlapping intervals, backed by a red-black tree. |
| [`rpcs`](rpcs) | HTTP RPC layer: `HttpBinder` binds `Handler`s to method and path and builds an immutable `HttpRouter` (`http.Handler`), with JSON body decoding and size limits, typed HTTP errors, request pre-processors, a CORS pre-processor and trace ID propagation. |
| [`logs`](logs) | `Logger` interface with structured `Loggable` fields and a logrus implementation that adds the trace ID stored in the context to every entry. |
| [`config`](config) | Declarative configuration: `Binder`s register flags, which are resolved from command line flags, `<APP>_*` environment variables or a TOML / YAML file. |
| [`readwrite`](readwrite) | `io` helpers: a reader that limits (and optionally fails on exceeding) the number of bytes read, and a fixed-buffer writer. |
| [`errors`](errors) | `Error`, a serializable error with a code and description that can be returned to clients and logged. |

## Usage

The snippets below are trimmed versions of the runnable examples in
[`concurrent/example_test.go`](concurrent/example_test.go) and
[`rpcs/example_test.go`](rpcs/example_test.go), which are compiled and checked by
`go test` and rendered on [pkg.go.dev](https://pkg.go.dev/github.com/eaugeas/octopus).

### Worker pool

```go
pool := concurrent.NewPoolRunnerWithOpts(ctx, concurrent.PoolOpts{Concurrency: 4})
defer pool.Stop() // idempotent, safe to call concurrently with Run

results := make(chan concurrent.PoolResult, 1)
err := pool.Run(concurrent.PoolInput{
	Supplier: concurrent.SupplierFunc(func() (interface{}, error) {
		return fetch(ctx)
	}),
	OutC: results, // optional; nil discards the result
})
if err != nil {
	// ctx.Err() if the pool context is done, concurrent.ErrPoolStopped after Stop
}
res := <-results
fmt.Println(res.Value(), res.Err())
```

### Batch

```go
ops := []concurrent.Supplier{opA, opB, opC}
results := concurrent.BatchSliceWithOpts(ctx, ops, concurrent.BatchOpts{Concurrency: 2})
for i, res := range results { // results[i] is the result of ops[i]
	fmt.Println(i, res.Value(), res.Err())
}
```

If `ctx` is cancelled the batch stops early, and the entries of operations that
did not complete are left as zero values; check `ctx.Err()` after the call.

### Retry

```go
v, err := concurrent.RetryWithOpts(ctx, op, concurrent.RetryOpts{
	Attempts:        5,
	BaseTimeout:     10 * time.Millisecond, // wait after the first failure
	BaseExp:         2,                     // each wait doubles...
	MaxRetryTimeout: time.Second,           // ...up to this cap
	Random:          true,                  // full jitter
})
// err is concurrent.ErrMaxAttemptsReached, ctx.Err(), or the cause of a
// concurrent.ErrCannotRecover returned by op (which stops retrying)
```

### Master / worker

A `Master` owns a set of long-lived workers, one per key, each running in its
own goroutine and handling its requests sequentially, so per-key state needs no
locking.

```go
master := concurrent.NewMaster(concurrent.MasterOpts{
	MasterHandler: concurrent.MasterHandlerFunc(func(ctx context.Context, ev concurrent.MasterEvent) error {
		switch ev := ev.(type) {
		case concurrent.CreateWorkerEvent:
			ev.Opts.WorkerHandler = newSessionHandler(ev.Key) // handles RequestWorkerEvent / ErrorWorkerEvent
			ev.Opts.MaxInactivity = 10 * time.Minute         // <= 0: never expire
			ev.Opts.ErrC = errC                              // optional: errors that may end the worker
		case concurrent.DestroyWorkerEvent:
			// destroyed by Destroy, Stop, inactivity or an error; ev.Cause is
			// the error returned by the WorkerHandler, if any
		}
		return nil
	}),
})
if err := master.Start(ctx); err != nil { ... }
defer master.Stop() // shuts down all the workers and waits for them

err := master.Create(ctx, "alice", nil)
v, err := master.Request(ctx, "alice", req)      // one worker
responses, err := master.Broadcast(ctx, req)     // every worker
v, err = master.Execute(ctx, req)                // any available worker
```

Every call honours its context and never blocks on a master or worker that has
gone away: once the event loop has exited (after `Stop`, or when the context
passed to `Start` is done) calls return `concurrent.ErrMasterStopped`, and a
request queued for a worker that exits before handling it fails with
`concurrent.ErrWorkerExited`. A worker that serves no request for
`MaxInactivity` destroys itself and is removed, so `Exists` returns false
afterwards. `Stop` is safe to call after the context was cancelled, and a
stopped master can be started again.

### HTTP RPC binder

```go
binder := rpcs.NewHttpBinder(rpcs.HttpBinderProperties{
	Encoder: rpcs.JsonEncoder{},
	Logger:  logger,
	HandlerFactory: rpcs.HttpHandlerFactoryFunc(
		func(factory rpcs.EntityFactory, handler rpcs.Handler) rpcs.HttpMiddleware {
			return rpcs.NewHttpJsonHandler(rpcs.HttpJsonHandlerProperties{
				Limit: 1 << 10, Handler: handler, Logger: logger, Factory: factory,
			})
		}),
})

binder.Bind(http.MethodPost, "/greet",
	rpcs.HandlerFunc(func(ctx context.Context, req interface{}) (interface{}, error) {
		body := req.(*greetRequest)
		if body.Name == "" {
			return nil, rpcs.HttpBadRequest(ctx, errors.New("name is required"))
		}
		return greetResponse{Greeting: "hello " + body.Name}, nil
	}),
	rpcs.EntityFactoryFunc(func() interface{} { return &greetRequest{} }))

log.Fatal(http.ListenAndServe(":8080", binder.Build()))
```

```sh
$ curl -s -XPOST localhost:8080/greet -H 'Content-Type: application/json' -d '{"name":"octopus"}'
{"greeting":"hello octopus"}
```

`HttpJsonHandler` accepts `application/json` with parameters such as
`; charset=utf-8` and rejects any other media type with a 400. Responses are
encoded into a buffer before anything is written, so a value that fails to
encode yields a clean 500, and a panic in a handler yields a 500 unless the
response had already been started.

To add CORS to every route of a binder, adapt the CORS pre-processor with
`AsPreProcessor`. Preflight `OPTIONS` requests are answered by the
pre-processor and never reach the handlers:

```go
binder.AddPreProcessor(rpcs.NewHttpCorsPreProcessor(rpcs.HttpCorsPreProcessorProps{
	Enabled:        true,
	AllowedOrigins: []string{"https://app.example.com"},
	AllowedMethods: []string{http.MethodPost},
	AllowedHeaders: []string{"Content-Type"},
}).AsPreProcessor())
```

## Development

Requires Go 1.18 or newer.

```sh
make test               # go test -race ./...
make ci                 # what CI runs: gofmt check, go vet, go test -race -cover
make bench              # benchmarks
make coverage-html      # coverage report in the browser
```

CI checks formatting with `gofmt -l`, and runs `go vet ./...` and
`go test -race -cover ./...` on every push and pull request
([`.github/workflows/ci.yml`](.github/workflows/ci.yml)).

## License

See [LICENSE](LICENSE).
