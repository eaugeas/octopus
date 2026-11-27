package rpcs

import (
	"bytes"
	"context"
	stderr "errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eaugeas/octopus/logs"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var logger = logs.NewLogrus(logs.LogrusLoggerProperties{
	Level:  logrus.DebugLevel,
	Output: ioutil.Discard,
})

func mapEntityFactory() EntityFactory {
	return EntityFactoryFunc(func() interface{} { m := make(map[string]string); return &m })
}

func simpleHandlerFactory(factory EntityFactory, handler Handler) HttpMiddleware {
	return HttpMiddlewareRelay{handler: handler}
}

type HandlerEcho struct{}

func (m HandlerEcho) Handle(ctx context.Context, v interface{}) (interface{}, error) {
	return v, nil
}

type HttpMiddlewareRelay struct {
	handler Handler
}

func (m HttpMiddlewareRelay) ServeHTTP(req *http.Request) (interface{}, error) {
	return m.handler.Handle(req.Context(), nil)
}

type HttpMiddlewareOK struct {
	body interface{}
}

func (m HttpMiddlewareOK) ServeHTTP(req *http.Request) (interface{}, error) {
	return m.body, nil
}

type HttpMiddlewarePanic struct{}

func (m HttpMiddlewarePanic) ServeHTTP(req *http.Request) (interface{}, error) {
	panic("error")
}

type ErrEncoder struct{}

func (e ErrEncoder) Encode(w io.Writer, v interface{}) error {
	return stderr.New("failed to encode")
}

func setupRouter() *HttpRouter {
	enc := &JsonEncoder{}
	handlers := map[string]MethodHandlers{
		"/path": map[string]HttpMiddleware{
			"GET": HttpMiddlewareOK{body: map[string]string{"result": "ok"}},
			"PUT": HttpMiddlewareOK{body: nil},
		},
		"/panic": map[string]HttpMiddleware{
			"GET": HttpMiddlewarePanic{},
		},
	}

	mux := make(map[string]*HttpRoute)
	for path, handler := range handlers {
		mux[path] = NewHttpRoute(HttpRouteProps{
			Logger:   logger,
			Encoder:  enc,
			Handlers: handler,
		})
	}

	return &HttpRouter{
		encoder: enc,
		mux:     mux,
		logger:  logger,
	}
}

func TestHttpRouterServeHTTPNoRoute(t *testing.T) {
	router := setupRouter()

	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/unknown", nil)

	router.ServeHTTP(recorder, req)

	s, err := ioutil.ReadAll(recorder.Body)

	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Equal(t, "", string(s))
}

func TestHttpRouterServeHTTPNoMethod(t *testing.T) {
	router := setupRouter()

	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/path", nil)

	router.ServeHTTP(recorder, req)

	s, err := ioutil.ReadAll(recorder.Body)

	assert.Nil(t, err)
	assert.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
	assert.Equal(t, "", string(s))
}

func TestHttpRouterServeHTTPOKNoBody(t *testing.T) {
	router := setupRouter()

	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/path", nil)

	router.ServeHTTP(recorder, req)

	s, err := ioutil.ReadAll(recorder.Body)

	assert.Nil(t, err)
	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.Equal(t, "", string(s))
}

func TestHttpRouterServeHTTPOKWithBody(t *testing.T) {
	router := setupRouter()

	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/path", nil)

	router.ServeHTTP(recorder, req)

	s, err := ioutil.ReadAll(recorder.Body)

	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "{\"result\":\"ok\"}\n", string(s))
}

func TestHttpRouterServeHTTPPanic(t *testing.T) {
	router := setupRouter()

	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/panic", nil)

	router.ServeHTTP(recorder, req)

	s, err := ioutil.ReadAll(recorder.Body)

	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, "{\"errorCode\":-1,\"description\":\"Internal Server Error\"}\n", string(s))
}

func TestHttpBinderBuildRouterNoEncoder(t *testing.T) {
	assert.Panics(t, func() {
		NewHttpBinder(HttpBinderProperties{
			Logger:         logger,
			HandlerFactory: HttpHandlerFactoryFunc(simpleHandlerFactory),
		})
	})
}

func TestHttpBinderBuildRouterNoLogger(t *testing.T) {
	assert.Panics(t, func() {
		NewHttpBinder(HttpBinderProperties{
			Encoder:        JsonEncoder{},
			HandlerFactory: HttpHandlerFactoryFunc(simpleHandlerFactory),
		})
	})
}

func TestHttpBinderBuildRouterNoFactory(t *testing.T) {
	assert.Panics(t, func() {
		NewHttpBinder(HttpBinderProperties{
			Encoder: JsonEncoder{},
			Logger:  logger,
		})
	})
}

func TestHttpRouteReportSuccessEncoderErr(t *testing.T) {
	router := setupRouter()
	route := router.mux["/path"]
	route.encoder = ErrEncoder{}

	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/path", nil)
	status, err := route.reportSuccess(recorder, req, make(map[string]string))

	// the status returned is the one written to the response
	assert.Error(t, err)
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, "", recorder.Body.String())
}

func TestHttpBinderBuildRouter(t *testing.T) {
	binder := NewHttpBinder(HttpBinderProperties{
		Encoder:        JsonEncoder{},
		Logger:         logger,
		HandlerFactory: HttpHandlerFactoryFunc(simpleHandlerFactory),
	})

	binder.Bind("GET", "/path", HandlerEcho{}, nil)
	router := binder.Build()

	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/path", nil)

	router.ServeHTTP(recorder, req)

	s, err := ioutil.ReadAll(recorder.Body)

	assert.Nil(t, err)
	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.Equal(t, "", string(s))
}

func TestHttpJsonHandlerContentLengthMissing(t *testing.T) {
	handler := NewHttpJsonHandler(HttpJsonHandlerProperties{
		Limit:   1024,
		Handler: HandlerEcho{},
		Logger:  logger,
		Factory: mapEntityFactory(),
	})

	req, _ := http.NewRequest("GET", "/path", nil)
	req.ContentLength = -1

	v, err := handler.ServeHTTP(req)

	assert.Equal(t, "content-length header missing in request with status code 400", err.Error())
	assert.Nil(t, v)
}

func TestHttpJsonHandlerContentLengthMissingWithBody(t *testing.T) {
	handler := NewHttpJsonHandler(HttpJsonHandlerProperties{
		Limit:   1024,
		Handler: HandlerEcho{},
		Logger:  logger,
		Factory: mapEntityFactory(),
	})

	req, _ := http.NewRequest("GET", "/path", bytes.NewBufferString(""))
	req.ContentLength = -1

	v, err := handler.ServeHTTP(req)

	assert.Equal(t, "content-length header missing in request with status code 400", err.Error())
	assert.Nil(t, v)
}

func TestHttpJsonHandlerContentLengthExceedsLimit(t *testing.T) {
	handler := NewHttpJsonHandler(HttpJsonHandlerProperties{
		Limit:   1024,
		Handler: HandlerEcho{},
		Logger:  logger,
		Factory: mapEntityFactory(),
	})

	req, _ := http.NewRequest("GET", "/path", bytes.NewBufferString(""))
	req.ContentLength = 2048

	v, err := handler.ServeHTTP(req)

	assert.Equal(t, "content-length value exceeds request limit with status code 400", err.Error())
	assert.Nil(t, v)
}

func TestHttpJsonHandlerContentMissing(t *testing.T) {
	handler := NewHttpJsonHandler(HttpJsonHandlerProperties{
		Limit:   1024,
		Handler: HandlerEcho{},
		Logger:  logger,
		Factory: mapEntityFactory(),
	})

	req, _ := http.NewRequest("GET", "/path",
		bytes.NewBufferString("{\"hamburger\":\"rare\",\"potato\":\"fried\"}\n"))
	req.ContentLength = 38

	v, err := handler.ServeHTTP(req)

	assert.Equal(t, "content-type has unexpected value with status code 400", err.Error())
	assert.Nil(t, v)
}

func TestHttpJsonHandlerOK(t *testing.T) {
	handler := NewHttpJsonHandler(HttpJsonHandlerProperties{
		Limit:   1024,
		Handler: HandlerEcho{},
		Logger:  logger,
		Factory: mapEntityFactory(),
	})

	req, _ := http.NewRequest("GET", "/path",
		bytes.NewBufferString("{\"hamburger\":\"rare\",\"potato\":\"fried\"}\n"))
	req.ContentLength = 38
	req.Header.Add("Content-type", "application/json")

	v, err := handler.ServeHTTP(req)
	m := *v.(*map[string]string)
	assert.Nil(t, err)
	assert.Equal(t, map[string]string{"hamburger": "rare", "potato": "fried"}, m)
}

func TestHttpCorsPreProcessorOK(t *testing.T) {
	processor := NewHttpCorsPreProcessor(HttpCorsPreProcessorProps{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   nil,
		ExposedHeaders:   nil,
		MaxAge:           10,
		AllowCredentials: true,
		Enabled:          true,
	})

	req, err := http.NewRequest("GET", "http://localhost.com/path", nil)
	assert.Nil(t, err)

	req.Header.Add("Origin", "http://localhost.example")
	recorder := httptest.NewRecorder()

	ok, newReq := processor.ServeHTTP(recorder, req)

	assert.True(t, ok)
	assert.Equal(t, req, newReq)
}

func TestHttpCorsPreProcessorErrOriginNotAllowed(t *testing.T) {
	processor := NewHttpCorsPreProcessor(HttpCorsPreProcessorProps{
		AllowedOrigins:   []string{"http://localhost.example"},
		AllowedMethods:   nil,
		ExposedHeaders:   nil,
		MaxAge:           10,
		AllowCredentials: true,
		Enabled:          true,
	})

	req, err := http.NewRequest("GET", "http://potato.example/fries", nil)
	req.Header.Add("Origin", "http://potato.example")
	assert.Nil(t, err)
	recorder := httptest.NewRecorder()

	ok, newReq := processor.ServeHTTP(recorder, req)

	assert.True(t, ok)
	assert.Equal(t, recorder.Header(), http.Header{"Vary": []string{"Origin"}})
	assert.Equal(t, req, newReq)
}

func TestHttpCorsPreProcessorErrMethodNotAllowed(t *testing.T) {
	processor := NewHttpCorsPreProcessor(HttpCorsPreProcessorProps{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   nil,
		ExposedHeaders:   nil,
		MaxAge:           10,
		AllowCredentials: true,
		Enabled:          true,
	})

	req, err := http.NewRequest("OPTIONS", "/path", nil)
	req.Header.Add("Access-Control-Request-Method", "NOT_ALLOWED")
	assert.Nil(t, err)
	recorder := httptest.NewRecorder()

	ok, _ := processor.ServeHTTP(recorder, req)

	assert.False(t, ok)
}

type ctxKey string

// HttpPreProcessorWithValue replaces the request with one that carries
// a value in its context
type HttpPreProcessorWithValue struct{}

func (p HttpPreProcessorWithValue) ServeHTTP(w http.ResponseWriter, req *http.Request) (HttpPreProcessorResult, error) {
	return HttpPreProcessorResult{
		Request:  req.WithContext(context.WithValue(req.Context(), ctxKey("key"), "value")),
		Continue: true,
	}, nil
}

// HttpPreProcessorStop handles the request itself
type HttpPreProcessorStop struct{}

func (p HttpPreProcessorStop) ServeHTTP(w http.ResponseWriter, req *http.Request) (HttpPreProcessorResult, error) {
	w.WriteHeader(http.StatusTeapot)
	return HttpPreProcessorResult{Continue: false}, nil
}

func TestHttpRoutePreProcessorModifiedRequestIsUsed(t *testing.T) {
	route := NewHttpRoute(HttpRouteProps{
		Logger:  logger,
		Encoder: JsonEncoder{},
		Handlers: MethodHandlers{
			"GET": HttpMiddlewareFunc(func(req *http.Request) (interface{}, error) {
				return map[string]interface{}{"key": req.Context().Value(ctxKey("key"))}, nil
			}),
		},
		PreProcessors: []HttpPreProcessor{HttpPreProcessorWithValue{}},
	})

	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/path", nil)
	route.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "{\"key\":\"value\"}\n", recorder.Body.String())
}

func TestHttpRoutePreProcessorStops(t *testing.T) {
	called := false
	route := NewHttpRoute(HttpRouteProps{
		Logger:  logger,
		Encoder: JsonEncoder{},
		Handlers: MethodHandlers{
			"GET": HttpMiddlewareFunc(func(req *http.Request) (interface{}, error) {
				called = true
				return nil, nil
			}),
		},
		PreProcessors: []HttpPreProcessor{HttpPreProcessorStop{}},
	})

	recorder := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/path", nil)
	route.ServeHTTP(recorder, req)

	assert.False(t, called)
	assert.Equal(t, http.StatusTeapot, recorder.Code)
}

func TestHttpErrorWithoutCause(t *testing.T) {
	err := &HttpError{StatusCode: http.StatusNotFound}
	assert.Equal(t, "Not Found with status code 404", err.Error())

	err = HttpMethodNotAllowed(context.Background(), nil)
	assert.Equal(t, "Method Not Allowed with status code 405", err.Error())

	err = HttpBadRequest(context.Background(), stderr.New("bad input"))
	assert.Equal(t, "bad input with status code 400", err.Error())
}

func newJsonRequest(contentType, body string) *http.Request {
	req := httptest.NewRequest("POST", "/path", bytes.NewBufferString(body))
	if len(contentType) > 0 {
		req.Header.Set("Content-Type", contentType)
	}
	return req
}

func TestHttpJsonHandlerContentTypeWithParameters(t *testing.T) {
	handler := NewHttpJsonHandler(HttpJsonHandlerProperties{
		Limit:   1024,
		Handler: HandlerEcho{},
		Logger:  logger,
		Factory: mapEntityFactory(),
	})

	for _, contentType := range []string{
		"application/json",
		"application/json; charset=utf-8",
		"Application/JSON;charset=UTF-8",
	} {
		v, err := handler.ServeHTTP(newJsonRequest(contentType, `{"potato":"fried"}`))
		assert.Nil(t, err, contentType)
		if assert.NotNil(t, v, contentType) {
			assert.Equal(t, map[string]string{"potato": "fried"}, *v.(*map[string]string))
		}
	}
}

func TestHttpJsonHandlerContentTypeRejected(t *testing.T) {
	handler := NewHttpJsonHandler(HttpJsonHandlerProperties{
		Limit:   1024,
		Handler: HandlerEcho{},
		Logger:  logger,
		Factory: mapEntityFactory(),
	})

	for _, contentType := range []string{
		"",
		"text/plain",
		"text/plain; charset=utf-8",
		"application/jsonp",
		"application/json-patch+json",
		"application/json; charset", // malformed parameter
	} {
		v, err := handler.ServeHTTP(newJsonRequest(contentType, `{"potato":"fried"}`))
		assert.Nil(t, v, contentType)
		if httpErr, ok := err.(*HttpError); assert.True(t, ok, contentType) {
			assert.Equal(t, ErrHttpContentTypeNotJSON, httpErr.Cause, contentType)
			assert.Equal(t, http.StatusBadRequest, httpErr.StatusCode, contentType)
		}
	}
}

func setupRouteRouter(handlers MethodHandlers, preProcessors ...HttpPreProcessor) *HttpRouter {
	enc := JsonEncoder{}
	return &HttpRouter{
		encoder: enc,
		logger:  logger,
		mux: map[string]*HttpRoute{
			"/path": NewHttpRoute(HttpRouteProps{
				Logger:        logger,
				Encoder:       enc,
				Handlers:      handlers,
				PreProcessors: preProcessors,
			}),
		},
	}
}

func TestHttpRouterServeHTTPUnencodableBody(t *testing.T) {
	for _, body := range []interface{}{
		make(chan int),
		map[string]interface{}{"ok": 1, "fail": make(chan int)},
	} {
		router := setupRouteRouter(MethodHandlers{"GET": HttpMiddlewareOK{body: body}})

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/path", nil)
		router.ServeHTTP(recorder, req)

		// a clean error response, without any partially encoded body
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, "{\"errorCode\":-1,\"description\":\"Internal Server Error\"}\n", recorder.Body.String())
		// the trace ID is set once, not once per attempt to write a response
		assert.Len(t, recorder.Header().Values(HttpHeaderTraceID), 1)
	}
}

func TestHttpRouteReportSuccessUnencodable(t *testing.T) {
	route := NewHttpRoute(HttpRouteProps{Logger: logger, Encoder: JsonEncoder{}})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/path", nil)
	status, err := route.reportSuccess(recorder, req, make(chan int))

	assert.Error(t, err)
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
}

func TestHttpRouteReportErrorEncoderErr(t *testing.T) {
	route := NewHttpRoute(HttpRouteProps{Logger: logger, Encoder: ErrEncoder{}})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/path", nil)
	httpErr := HttpBadRequest(req.Context(), stderr.New("bad input"))
	status, err := route.reportError(recorder, req, httpErr)

	// the encoding error is returned, not the error being reported
	assert.Error(t, err)
	assert.Equal(t, "failed to encode", err.Error())
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Equal(t, "", recorder.Body.String())
}

func TestHttpRouteReportErrorOK(t *testing.T) {
	route := NewHttpRoute(HttpRouteProps{Logger: logger, Encoder: JsonEncoder{}})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/path", nil)
	status, err := route.reportError(recorder, req, HttpBadRequest(req.Context(), stderr.New("bad input")))

	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Equal(t, "{\"errorCode\":-1,\"description\":\"Bad Request\"}\n", recorder.Body.String())
}

func TestHttpRouterServeHTTPErrorEncoderErr(t *testing.T) {
	router := setupRouter()
	router.encoder = ErrEncoder{}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/panic", nil)
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, "", recorder.Body.String())
}

// HttpPreProcessorWriteThenPanic starts a response and then panics
type HttpPreProcessorWriteThenPanic struct{}

func (p HttpPreProcessorWriteThenPanic) ServeHTTP(w http.ResponseWriter, req *http.Request) (HttpPreProcessorResult, error) {
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte("partial"))
	panic("error")
}

func TestHttpRouterServeHTTPPanicAfterWrite(t *testing.T) {
	router := setupRouteRouter(
		MethodHandlers{"GET": HttpMiddlewareOK{body: nil}},
		HttpPreProcessorWriteThenPanic{})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/path", nil)
	router.ServeHTTP(recorder, req)

	// the response that was already started is left untouched
	assert.Equal(t, http.StatusAccepted, recorder.Code)
	assert.Equal(t, "partial", recorder.Body.String())
}

func TestHttpRouterServeHTTPPanicBeforeWrite(t *testing.T) {
	router := setupRouteRouter(MethodHandlers{"GET": HttpMiddlewarePanic{}})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/path", nil)
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, "{\"errorCode\":-1,\"description\":\"Internal Server Error\"}\n", recorder.Body.String())
}

func TestTrackingResponseWriterFlush(t *testing.T) {
	recorder := httptest.NewRecorder()
	w := &trackingResponseWriter{ResponseWriter: recorder}

	w.Flush()
	assert.True(t, w.wroteHeader)
	assert.True(t, recorder.Flushed)
	assert.Equal(t, http.ResponseWriter(recorder), w.Unwrap())
}

func TestHttpCorsPreProcessorAsPreProcessor(t *testing.T) {
	called := 0
	binder := NewHttpBinder(HttpBinderProperties{
		Encoder: JsonEncoder{},
		Logger:  logger,
		HandlerFactory: HttpHandlerFactoryFunc(func(factory EntityFactory, handler Handler) HttpMiddleware {
			return HttpMiddlewareFunc(func(req *http.Request) (interface{}, error) {
				called++
				return map[string]string{"result": "ok"}, nil
			})
		}),
	})
	binder.AddPreProcessor(NewHttpCorsPreProcessor(HttpCorsPreProcessorProps{
		Enabled:        true,
		AllowedOrigins: []string{"http://app.example"},
		AllowedMethods: []string{http.MethodPost},
		MaxAge:         60,
	}).AsPreProcessor())
	binder.Bind(http.MethodPost, "/path", HandlerEcho{}, nil)
	router := binder.Build()

	// preflight requests are answered by the pre processor
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/path", nil)
	req.Header.Set("Origin", "http://app.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.Equal(t, "http://app.example", recorder.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "POST", recorder.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "60", recorder.Header().Get("Access-Control-Max-Age"))
	assert.Equal(t, 0, called)

	// actual requests go through to the handler with the CORS headers
	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/path", nil)
	req.Header.Set("Origin", "http://app.example")
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "http://app.example", recorder.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "{\"result\":\"ok\"}\n", recorder.Body.String())
	assert.Equal(t, 1, called)

	// requests from other origins are not given CORS headers
	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/path", nil)
	req.Header.Set("Origin", "http://other.example")
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "", recorder.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, 2, called)
}

func TestHttpCorsPreProcessorAsPreProcessorDisabled(t *testing.T) {
	processor := NewHttpCorsPreProcessor(HttpCorsPreProcessorProps{Enabled: false}).AsPreProcessor()

	req := httptest.NewRequest(http.MethodOptions, "/path", nil)
	req.Header.Set("Origin", "http://app.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	recorder := httptest.NewRecorder()

	result, err := processor.ServeHTTP(recorder, req)
	assert.Nil(t, err)
	assert.True(t, result.Continue)
	assert.Equal(t, req, result.Request)
}
