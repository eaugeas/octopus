package rpcs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"

	errs "github.com/eaugeas/octopus/errors"
	"github.com/eaugeas/octopus/logs"
	rw "github.com/eaugeas/octopus/readwrite"
	stderr "github.com/pkg/errors"
	"github.com/rs/cors"
)

const HttpHeaderTraceID = "X-TRACE-ID"

type HttpPreProcessorResult struct {
	Request  *http.Request
	Continue bool
}

// HttpPreProcessor processes a request and can directly write a response
// to the writer if required.
type HttpPreProcessor interface {
	// ServeHTTP is a similar interface to http.Handler with the difference that
	// it returns true parameters. The boolean parameter indicates in case of its
	// value being true that the request can be further processed by another handler.
	// In case that it's false, no further processing of the request is required.
	// The *http.Request returned is a potentially modified request resulting of
	// mutating the original *http.Request
	ServeHTTP(w http.ResponseWriter, req *http.Request) (HttpPreProcessorResult, error)
}

// HttpMiddleware are the handlers that offer extra functionality to a request and
// that in success will forward the request to another handler
type HttpMiddleware interface {
	// ServeHTTP allows to handle an http request. The response will be serialized
	// by an HttpRouter
	ServeHTTP(req *http.Request) (interface{}, error)
}

// HttpMiddlewareFunc allows functions to implement the HttpMiddleware interface
type HttpMiddlewareFunc func(req *http.Request) (interface{}, error)

func (f HttpMiddlewareFunc) ServeHTTP(req *http.Request) (interface{}, error) {
	return f(req)
}

// HttpError holds the necessary information to return an error when
// using the http protocol
type HttpError struct {
	// Cause of the creation of this HttpError instance
	Cause error

	// StatusCode is the HTTP status code that defines the error cause
	StatusCode int

	// Message is the human-readable string that defines the error cause
	Message string
}

// Log implementation of logs.Loggable
func (e *HttpError) Log(fields logs.Fields) {
	fields.Add("status_code", e.StatusCode)

	if e.Cause != nil {
		switch e.Cause.(type) {
		case *errs.Error:
			e.Cause.(*errs.Error).Log(fields)
		default:
			fields.Add("description", e.Cause.Error())
		}
	}
}

// Error is the implementation of go's error interface for Error
func (e *HttpError) Error() string {
	return fmt.Sprintf("%s with status code %d", e.Cause.Error(), e.StatusCode)
}

// MakeHttpError makes a new http error
func MakeHttpError(ctx context.Context, err error, statusCode int, msg string) *HttpError {
	return &HttpError{
		Cause:      err,
		StatusCode: statusCode,
		Message:    msg,
	}
}

// HttpBadRequest returns an HTTP bad request error
func HttpBadRequest(ctx context.Context, err error) *HttpError {
	return HttpBadRequestWithMessage(ctx, err, "Bad Request")
}

// HttpBadRequestWithMessage returns an HTTP bad request error with
// a custom message
func HttpBadRequestWithMessage(ctx context.Context, err error, msg string) *HttpError {
	return MakeHttpError(ctx, err, http.StatusBadRequest, msg)
}

// HttpForbidden returns an HTTP not found error
func HttpForbidden(ctx context.Context, err error) *HttpError {
	return MakeHttpError(ctx, err, http.StatusForbidden, "Forbidden")
}

// HttpNotFound returns an HTTP not found error
func HttpNotFound(ctx context.Context, err error) *HttpError {
	return MakeHttpError(ctx, err, http.StatusNotFound, "Not Found")
}

// HttpMethodNotAllowed returns an HTTP not found error
func HttpMethodNotAllowed(ctx context.Context, err error) *HttpError {
	return MakeHttpError(ctx, err, http.StatusMethodNotAllowed, "Method Not Allowed")
}

// HttpTooMayRequests return an HTTP too many requests error
func HttpTooManyRequests(ctx context.Context, err error) *HttpError {
	return MakeHttpError(ctx, err, http.StatusTooManyRequests, "Too many requests")
}

// HttpNotFound returns an HTTP not found error
func HttpNotImplemented(ctx context.Context, err error) *HttpError {
	return MakeHttpError(ctx, err, http.StatusNotImplemented, "Not Implemented")
}

// HttpInternalServerError returns an HTTP internal server error
func HttpInternalServerError(ctx context.Context, err error) *HttpError {
	return MakeHttpError(ctx, err, http.StatusInternalServerError, "Internal Server Error")
}

// MethodHandlers keeps the handlers for each of the methods
type MethodHandlers map[string]HttpMiddleware

// Add a new handler to the set
func (h MethodHandlers) Add(method string, middleware HttpMiddleware) {
	h[method] = middleware
}

// HttpRoute multiplexes the handling of a request to the handler
// that expects a particular method
type HttpRoute struct {
	logger        logs.Logger
	handlers      map[string]HttpMiddleware
	preProcessors []HttpPreProcessor
	encoder       Encoder
}

// HttpRouteProps are the required properties to create
// a new HttpRoute instance
type HttpRouteProps struct {
	Logger        logs.Logger
	Encoder       Encoder
	Handlers      MethodHandlers
	PreProcessors []HttpPreProcessor
}

// NewHttpRoute creates a new route instance
func NewHttpRoute(props HttpRouteProps) *HttpRoute {
	methods := make([]string, 0, len(props.Handlers))

	for method := range props.Handlers {
		methods = append(methods, method)
	}

	return &HttpRoute{
		logger:        props.Logger,
		handlers:      props.Handlers,
		preProcessors: props.PreProcessors,
		encoder:       props.Encoder,
	}
}

// HasHandler returns true if the route has a handler that
// would handle the provided method
func (h *HttpRoute) HasHandler(method string) bool {
	_, ok := h.handlers[method]
	return ok
}

func (h *HttpRoute) reportSuccess(
	res http.ResponseWriter,
	req *http.Request,
	body interface{},
) (int, error) {
	path := req.URL.EscapedPath()
	method := req.Method

	res.Header().Add(HttpHeaderTraceID, strconv.FormatInt(logs.GetTraceID(req.Context()), 10))

	if body == nil {
		res.WriteHeader(http.StatusNoContent)
		return http.StatusNoContent, nil
	}

	if err := h.encoder.Encode(res, body); err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		h.logger.Info(req.Context(), "", logs.MapFields{
			"path":        path,
			"method":      method,
			"call_type":   "HttpRequestHandleSuccess",
			"status_code": http.StatusNoContent,
		})
		return 0, err
	}

	return http.StatusOK, nil
}

// HttpRoute implementation of HttpMiddleware
func (h *HttpRoute) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	h.logger.Debug(req.Context(), "handle request", logs.MapFields(map[string]interface{}{
		"path":   req.URL.Path,
		"method": req.Method,
	}))
	result := HttpPreProcessorResult{Continue: true, Request: req}
	var err error
	for _, preProcessor := range h.preProcessors {
		result, err = preProcessor.ServeHTTP(res, result.Request)
		if err != nil {
			h.logger.Warn(req.Context(), "error", logs.MapFields(map[string]interface{}{
				"path":   req.URL.Path,
				"method": req.Method,
				"err":    err.Error(),
			}))
			return
		}

		if !result.Continue {
			return
		}
	}

	status, err := h.serveHTTP(res, req)
	var errMsg = ""
	if err != nil {
		errMsg = err.Error()
	}
	switch {
	case status >= http.StatusOK && status <= 299:
		h.logger.Info(req.Context(), "success", logs.MapFields(map[string]interface{}{
			"path":   req.URL.Path,
			"method": req.Method,
			"status": status,
			"err":    errMsg,
		}))
	case status < http.StatusOK:
		h.logger.Debug(req.Context(), "logic", logs.MapFields(map[string]interface{}{
			"path":   req.URL.Path,
			"method": req.Method,
			"status": status,
			"err":    errMsg,
		}))
	case status > 299 && status < 400:
		h.logger.Debug(req.Context(), "redirect", logs.MapFields(map[string]interface{}{
			"path":   req.URL.Path,
			"method": req.Method,
			"status": status,
			"err":    errMsg,
		}))
	case status >= 400:
		h.logger.Warn(req.Context(), "error", logs.MapFields(map[string]interface{}{
			"path":   req.URL.Path,
			"method": req.Method,
			"status": status,
			"err":    errMsg,
		}))
	}
}

func (h *HttpRoute) serveHTTP(res http.ResponseWriter, req *http.Request) (int, error) {
	handler, ok := h.handlers[req.Method]
	if !ok {
		return h.reportError(res, req, HttpMethodNotAllowed(req.Context(), nil))
	}

	v, err := handler.ServeHTTP(req)
	if err != nil {
		return h.reportAnyError(res, req, err)
	}

	return h.reportSuccess(res, req, v)
}

func (h *HttpRoute) reportAnyError(res http.ResponseWriter, req *http.Request, err error) (int, error) {
	switch err := err.(type) {
	case *HttpError:
		return h.reportError(res, req, err)
	default:
		return h.reportError(res, req, HttpInternalServerError(
			req.Context(), err))
	}
}

func (h *HttpRoute) reportError(res http.ResponseWriter, req *http.Request, err *HttpError) (int, error) {
	path := req.URL.EscapedPath()
	method := req.Method

	res.Header().Add(HttpHeaderTraceID, strconv.FormatInt(logs.GetTraceID(req.Context()), 10))
	res.WriteHeader(err.StatusCode)

	if err.Cause != nil {
		code := -1
		desc := err.Message

		if e, ok := err.Cause.(*errs.Error); ok {
			code = e.ErrorCode
		}

		if eerr := h.encoder.Encode(res, errs.Error{
			ErrorCode:   code,
			Description: desc,
		}); eerr != nil {
			h.logger.Warn(req.Context(), "failed to encode error response to response writer", logs.MapFields{
				"path":      path,
				"method":    method,
				"call_type": "HttpEncodeErrorError",
			}, &errs.Error{Description: eerr.Error()})
			return 0, err
		}
	}
	return err.StatusCode, nil
}

// HttpRouter multiplexes the handling of server request amongst the different
// handlers
type HttpRouter struct {
	encoder Encoder
	mux     map[string]*HttpRoute
	logger  logs.Logger
}

// HasRoute returns true if the router has a route to
// handle a request to the path
func (h *HttpRouter) HasRoute(path string) bool {
	_, ok := h.mux[path]
	return ok
}

// HasHandler returns true if the router has a handle to
// handle a request to the path and method
func (h *HttpRouter) HasHandler(path, method string) bool {
	route, ok := h.mux[path]
	if !ok {
		return false
	}

	return route.HasHandler(method)
}

// HttpRouter implementation of http.Handler
func (h *HttpRouter) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	path := req.URL.EscapedPath()
	method := req.Method
	traceID := ParseTraceID(req.Header.Get(HttpHeaderTraceID))
	req = req.WithContext(context.WithValue(req.Context(), logs.ContextKeyTraceID, traceID))

	h.logger.Debug(req.Context(), "", logs.MapFields{
		"path":      path,
		"method":    method,
		"call_type": "HttpRequestHandleAttempt",
	})

	defer func() {
		if r := recover(); r != nil {
			var err error
			stacktrace := debug.Stack()

			switch x := r.(type) {
			case string:
				err = stderr.New(x)
			case error:
				err = x
			default:
				err = fmt.Errorf("unknown panic %+v", r)
			}

			h.logger.Warn(req.Context(), "unexpected panic caught", logs.MapFields{
				"path":       path,
				"method":     method,
				"call_type":  "HttpRequestHandleFailure",
				"err":        err,
				"stacktrace": string(stacktrace),
			})
			// the `err` generated above is an internal error that should not
			// be exposed to the client. Instead, this we just return a generic
			// error
			h.reportAnyError(res, req, stderr.New("Unexpected error occurred."))
		}
	}()

	route, ok := h.mux[path]
	if !ok {
		h.reportError(res, req, &HttpError{StatusCode: http.StatusNotFound})
		return
	}

	route.ServeHTTP(res, req)
}

func (h *HttpRouter) reportAnyError(res http.ResponseWriter, req *http.Request, err error) {
	switch err := err.(type) {
	case *HttpError:
		h.reportError(res, req, err)
	default:
		h.reportError(res, req, HttpInternalServerError(
			req.Context(), err))
	}
}

func (h *HttpRouter) reportError(res http.ResponseWriter, req *http.Request, err *HttpError) {
	path := req.URL.EscapedPath()
	method := req.Method

	res.Header().Add(HttpHeaderTraceID, strconv.FormatInt(logs.GetTraceID(req.Context()), 10))
	res.WriteHeader(err.StatusCode)

	if err.Cause != nil {
		code := -1
		desc := err.Message

		if e, ok := err.Cause.(*errs.Error); ok {
			code = e.ErrorCode
		}
		if eerr := h.encoder.Encode(res, errs.Error{
			ErrorCode:   code,
			Description: desc,
		}); eerr != nil {
			h.logger.Debug(req.Context(), "failed to encode error response to response writer", logs.MapFields{
				"path":      path,
				"method":    method,
				"call_type": "HttpRequestHandleFailure",
			}, &errs.Error{Description: eerr.Error()})
			return
		}
	}

	h.logger.Info(req.Context(), "", logs.MapFields{
		"path":      path,
		"method":    method,
		"call_type": "HttpRequestHandleFailure",
	}, err)
}

// HttpCorsPreProcessorProps properties used to define the behaviour
// of the CORS implementation
type HttpCorsPreProcessorProps struct {
	// Enabled if true the HttpCorsHandler will verify requests, if false
	// the handler will just pass on a request to the next middleware
	Enabled bool

	// AllowedOrigins is a list of origins a cross-domain request can be executed from.
	// If the special "*" value is present in the list, all origins will be allowed.
	// An origin may contain a wildcard (*) to replace 0 or more characters
	// (i.e.: http://*.domain.com). Usage of wildcards implies a small performance penalty.
	// Only one wildcard can be used per origin.
	// Default value is ["*"]
	AllowedOrigins []string

	// AllowedMethods is a list of methods the client is allowed to use with
	// cross-domain requests. Default value is simple methods (HEAD, GET and POST).
	AllowedMethods []string

	// AllowedHeaders is list of non simple headers the client is allowed to use with
	// cross-domain requests.
	// If the special "*" value is present in the list, all headers will be allowed.
	// Default value is [] but "Origin" is always appended to the list.
	AllowedHeaders []string

	// ExposedHeaders indicates which headers are safe to expose to the API of a CORS
	// API specification
	ExposedHeaders []string

	// MaxAge indicates how long (in seconds) the results of a preflight request
	// can be cached
	MaxAge int

	// AllowCredentials indicates whether the request can include user credentials like
	// cookies, HTTP authentication or client side SSL certificates.
	AllowCredentials bool
}

// HttpCorsPreProcessor handles CORS https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS
// for requests
type HttpCorsPreProcessor struct {
	cors    *cors.Cors
	enabled bool
}

// NewHttpCorsPreProcessor creates a new instance of a Cors Http PreProcessor
func NewHttpCorsPreProcessor(props HttpCorsPreProcessorProps) *HttpCorsPreProcessor {
	preProcessor := cors.New(cors.Options{
		AllowedOrigins:     props.AllowedOrigins,
		AllowedMethods:     props.AllowedMethods,
		AllowedHeaders:     props.AllowedHeaders,
		ExposedHeaders:     props.ExposedHeaders,
		MaxAge:             props.MaxAge,
		AllowCredentials:   props.AllowCredentials,
		OptionsPassthrough: false,
		Debug:              false,
	})

	return &HttpCorsPreProcessor{
		cors:    preProcessor,
		enabled: props.Enabled,
	}
}

// ServeHTTP is the implementation of HttpMiddleware for HttpCorsHandler
func (h *HttpCorsPreProcessor) ServeHTTP(w http.ResponseWriter, req *http.Request) (bool, *http.Request) {
	if !h.enabled {
		return true, req
	}

	var (
		next    bool
		nextReq *http.Request
	)

	h.cors.ServeHTTP(w, req, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodOptions {
			// if it is a query request this handler can give a response directly
			w.WriteHeader(http.StatusOK)
			next = false
			return
		}

		next = true
		nextReq = req
	})

	return next, nextReq
}

// HttpJsonHandler handles requests that expect a body in the JSON format,
// handles the body and executes the final handler with the expected type
type HttpJsonHandler struct {
	limit   uint
	decoder JsonDecoder
	handler Handler
	logger  logs.Logger
	factory EntityFactory
}

type HttpJsonHandlerProperties struct {
	// Limit is the maximum number of bytes an Http body can have. Bodies
	// with a higher limit will fail to deserialize and be rejected
	Limit uint

	// Handler is the rpc handler that will be used to handle the request
	Handler Handler

	// Logger
	Logger logs.Logger

	// Factory for creating new instances of objects to which the Http body
	// will be deserialized. Those instances will be passed to the handler
	Factory EntityFactory
}

// NewHttpJsonHandlerFactory creates a new instance of an rpc handler
// that deserializes json objects into Go objects
func NewHttpJsonHandler(properties HttpJsonHandlerProperties) *HttpJsonHandler {
	limit := properties.Limit

	// set a reasonable default limit in case Limit is not set
	if limit == 0 {
		limit = 1 << 14 // 16 KB
	}

	if properties.Handler == nil {
		panic("handler must be sett")
	}

	if properties.Logger == nil {
		panic("logger must be set")
	}

	if properties.Factory == nil {
		panic("factory must be set")
	}

	return &HttpJsonHandler{
		limit:   limit,
		decoder: JsonDecoder{},
		handler: properties.Handler,
		logger:  properties.Logger.ForClass("http", "HttpJsonHandler"),
		factory: properties.Factory,
	}
}

type ErrMsgs error

var (
	ErrHttpContentLengthMissing ErrMsgs = errors.New("content-length header missing in request")
	ErrHttpContentLengthExceeds ErrMsgs = errors.New("content-length value exceeds request limit")
	ErrHttpContentTypeNotJSON   ErrMsgs = errors.New("content-type has unexpected value")
	ErrHttpHandleExpectsNoBody  ErrMsgs = errors.New("http handle expects no request body")
	ErrHttpDecodeJSON           ErrMsgs = errors.New("error decoding body as json")
)

// ServeHTTP is the implementation of HttpMiddleware for HttpJsonHandler
func (h *HttpJsonHandler) ServeHTTP(req *http.Request) (interface{}, error) {
	// verify that content length is set and it is correct
	if req.ContentLength < 0 {
		return nil, &HttpError{Cause: ErrHttpContentLengthMissing, StatusCode: http.StatusBadRequest}
	}

	if uint64(req.ContentLength) > uint64(h.limit) {
		return nil, &HttpError{Cause: ErrHttpContentLengthExceeds, StatusCode: http.StatusBadRequest}
	}

	// verify that content type is set and it is correct
	contentType := req.Header.Get("Content-type")
	if req.ContentLength > 0 && contentType != "application/json" {
		return nil, &HttpError{Cause: ErrHttpContentTypeNotJSON, StatusCode: http.StatusBadRequest}
	}

	// parse body into Go object
	body := h.factory.Create()
	if body == nil && req.ContentLength > 0 {
		return nil, &HttpError{Cause: ErrHttpHandleExpectsNoBody, StatusCode: http.StatusBadRequest}
	}

	if body != nil && req.ContentLength > 0 {
		if err := h.decoder.DecodeWithLimit(req.Body, body, rw.ReadLimitProps{
			Limit:        req.ContentLength,
			FailOnExceed: true,
		}); err != nil {
			h.logger.Debug(req.Context(), "failed to decode json", logs.MapFields{
				"path":           req.URL.EscapedPath(),
				"method":         req.Method,
				"content_length": req.ContentLength,
				"call_type":      "HttpJsonRequestHandleFailure",
				"err":            err.Error(),
			})
			return nil, &HttpError{Cause: ErrHttpDecodeJSON, StatusCode: http.StatusBadRequest}
		}
	}

	// provide the parsed body to the handler and handle execution
	return h.handler.Handle(req.Context(), body)
}

// HttpHandlerFactory converts an rpc Handler into HttpMiddleware
// that can be plugged into a router
type HttpHandlerFactory interface {
	Make(factory EntityFactory, handler Handler) HttpMiddleware
}

// HttpHandlerFactoryFunc to allow functions to act as an HttpHandlerFactory
type HttpHandlerFactoryFunc func(factory EntityFactory, handler Handler) HttpMiddleware

// Make is the implementation of HttpHandlerFactory for HttpHandlerFactoryFunc
func (f HttpHandlerFactoryFunc) Make(factory EntityFactory, handler Handler) HttpMiddleware {
	return f(factory, handler)
}

// HttpBinder is the binder for http. It is also the only mechanism to build
// HttpRouter's. This is done so that an HttpRouter cannot be modified
// after it has been created
type HttpBinder struct {
	handlers      map[string]MethodHandlers
	preProcessors []HttpPreProcessor
	encoder       Encoder
	logger        logs.Logger
	factory       HttpHandlerFactory
}

// Bind is the implementation of HandlerBinder for HttpBinder
func (b *HttpBinder) Bind(method string, uri string, handler Handler, factory EntityFactory) {
	route, ok := b.handlers[uri]
	if !ok {
		route = make(MethodHandlers)
		b.handlers[uri] = route
	}

	route.Add(method, b.factory.Make(factory, handler))
}

func (b *HttpBinder) AddPreProcessor(preProcessor HttpPreProcessor) {
	b.preProcessors = append(b.preProcessors, preProcessor)
}

// Build creates a new HttpRouter and clears the handler map of the
// HttpBinder, so if new instances of HttpRouters need to be build
// Bind needs to be used again
func (b *HttpBinder) Build() *HttpRouter {
	mux := make(map[string]*HttpRoute)

	for path, handlers := range b.handlers {
		route := NewHttpRoute(HttpRouteProps{
			Logger:        b.logger,
			Encoder:       b.encoder,
			Handlers:      handlers,
			PreProcessors: b.preProcessors,
		})

		mux[path] = route
	}

	// avoid modification of the router handlers after the router
	// handler has been created
	b.handlers = make(map[string]MethodHandlers)

	return &HttpRouter{
		encoder: b.encoder,
		logger:  b.logger.ForClass("http", "router"),
		mux:     mux,
	}
}

// HttpBinderProperties are the properties used to create
// a new instance of an HttpBinder
type HttpBinderProperties struct {
	Encoder        Encoder
	Logger         logs.Logger
	HandlerFactory HttpHandlerFactory
}

// NewHttpBinder creates a new instance of the HttpBinder. It will
// panic in case there are errors in the construction of the binder
func NewHttpBinder(properties HttpBinderProperties) *HttpBinder {
	if properties.Encoder == nil {
		panic("Encoder must be set")
	}

	if properties.Logger == nil {
		panic("Logger must be set")
	}

	if properties.HandlerFactory == nil {
		panic("HandlerFactory must be set")
	}

	return &HttpBinder{
		handlers: make(map[string]MethodHandlers),
		encoder:  properties.Encoder,
		logger:   properties.Logger,
		factory:  properties.HandlerFactory,
	}
}
