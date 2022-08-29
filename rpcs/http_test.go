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

	assert.Error(t, err)
	assert.Equal(t, 0, status)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
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
