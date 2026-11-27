package rpcs_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/eaugeas/octopus/logs"
	"github.com/eaugeas/octopus/rpcs"
	"github.com/sirupsen/logrus"
)

type greetRequest struct {
	Name string `json:"name"`
}

type greetResponse struct {
	Greeting string `json:"greeting"`
}

// Bind an rpc handler to a method and path, build the router and
// serve a JSON request with it
func ExampleHttpBinder() {
	logger := logs.NewLogrus(logs.LogrusLoggerProperties{
		Level:  logrus.InfoLevel,
		Output: ioutil.Discard,
	})

	binder := rpcs.NewHttpBinder(rpcs.HttpBinderProperties{
		Encoder: rpcs.JsonEncoder{},
		Logger:  logger,
		// decode JSON bodies into the entity created by the factory
		// before passing them to the handler
		HandlerFactory: rpcs.HttpHandlerFactoryFunc(
			func(factory rpcs.EntityFactory, handler rpcs.Handler) rpcs.HttpMiddleware {
				return rpcs.NewHttpJsonHandler(rpcs.HttpJsonHandlerProperties{
					Limit:   1 << 10,
					Handler: handler,
					Logger:  logger,
					Factory: factory,
				})
			}),
	})

	binder.Bind(http.MethodPost, "/greet",
		rpcs.HandlerFunc(func(ctx context.Context, req interface{}) (interface{}, error) {
			body := req.(*greetRequest)
			if len(body.Name) == 0 {
				return nil, rpcs.HttpBadRequest(ctx, fmt.Errorf("name is required"))
			}
			return greetResponse{Greeting: "hello " + body.Name}, nil
		}),
		rpcs.EntityFactoryFunc(func() interface{} { return &greetRequest{} }))

	// the router is a regular http.Handler and can be passed to
	// http.ListenAndServe
	router := binder.Build()

	for _, payload := range []string{`{"name":"octopus"}`, `{}`} {
		req := httptest.NewRequest(http.MethodPost, "/greet", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, req)
		fmt.Print(recorder.Code, " ", recorder.Body.String())
	}
	// Output:
	// 200 {"greeting":"hello octopus"}
	// 400 {"errorCode":-1,"description":"Bad Request"}
}

// Add CORS support to all the routes of a binder. Preflight requests are
// answered without reaching the handlers
func ExampleHttpCorsPreProcessor_AsPreProcessor() {
	logger := logs.NewLogrus(logs.LogrusLoggerProperties{
		Level:  logrus.InfoLevel,
		Output: ioutil.Discard,
	})

	binder := rpcs.NewHttpBinder(rpcs.HttpBinderProperties{
		Encoder: rpcs.JsonEncoder{},
		Logger:  logger,
		HandlerFactory: rpcs.HttpHandlerFactoryFunc(
			func(factory rpcs.EntityFactory, handler rpcs.Handler) rpcs.HttpMiddleware {
				return rpcs.NewHttpJsonHandler(rpcs.HttpJsonHandlerProperties{
					Handler: handler,
					Logger:  logger,
					Factory: factory,
				})
			}),
	})

	binder.AddPreProcessor(rpcs.NewHttpCorsPreProcessor(rpcs.HttpCorsPreProcessorProps{
		Enabled:        true,
		AllowedOrigins: []string{"https://app.example.com"},
		AllowedMethods: []string{http.MethodPost},
		AllowedHeaders: []string{"Content-Type"},
	}).AsPreProcessor())

	binder.Bind(http.MethodPost, "/greet",
		rpcs.HandlerFunc(func(ctx context.Context, req interface{}) (interface{}, error) {
			return greetResponse{Greeting: "hello " + req.(*greetRequest).Name}, nil
		}),
		rpcs.EntityFactoryFunc(func() interface{} { return &greetRequest{} }))

	router := binder.Build()

	// the browser sends a preflight request before the actual request
	preflight := httptest.NewRequest(http.MethodOptions, "/greet", nil)
	preflight.Header.Set("Origin", "https://app.example.com")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodPost)
	preflight.Header.Set("Access-Control-Request-Headers", "Content-Type")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, preflight)
	fmt.Println(recorder.Code, recorder.Header().Get("Access-Control-Allow-Origin"),
		recorder.Header().Get("Access-Control-Allow-Methods"))

	req := httptest.NewRequest(http.MethodPost, "/greet", strings.NewReader(`{"name":"octopus"}`))
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	fmt.Print(recorder.Code, " ", recorder.Header().Get("Access-Control-Allow-Origin"), " ", recorder.Body.String())
	// Output:
	// 204 https://app.example.com POST
	// 200 https://app.example.com {"greeting":"hello octopus"}
}
