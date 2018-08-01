// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package httprequest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"
)

// Server represents the server side of an HTTP servers, and can be
// used to create HTTP handlers although it is not an HTTP handler
// itself.
type Server struct {
	// ErrorMapper holds a function that can convert a Go error
	// into a form that can be returned as a JSON body from an HTTP request.
	//
	// The httpStatus value reports the desired HTTP status.
	//
	// If the returned errorBody implements HeaderSetter, then
	// that method will be called to add custom headers to the request.
	//
	// If this both this and ErrorWriter are nil, DefaultErrorMapper will be used.
	ErrorMapper func(ctxt context.Context, err error) (httpStatus int, errorBody interface{})

	// ErrorWriter is a more general form of ErrorMapper. If this
	// field is set, ErrorMapper will be ignored and any returned
	// errors will be passed to ErrorWriter, which should use
	// w to set the HTTP status and write an appropriate
	// error response.
	ErrorWriter func(ctx context.Context, w http.ResponseWriter, err error)
}

// Handler defines a HTTP handler that will handle the
// given HTTP method at the given httprouter path
type Handler struct {
	Method string
	Path   string
	Handle httprouter.Handle
}

// handlerFunc represents a function that can handle an HTTP request.
type handlerFunc struct {
	// unmarshal unmarshals the request parameters into
	// the argument value required by the method.
	unmarshal func(p Params) (reflect.Value, error)

	// call invokes the request on the given function value with the
	// given argument value (as returned by unmarshal).
	call func(fv, argv reflect.Value, p Params)

	// method holds the HTTP method the function will be
	// registered for.
	method string

	// pathPattern holds the path pattern the function will
	// be registered for.
	pathPattern string
}

var (
	paramsType             = reflect.TypeOf(Params{})
	errorType              = reflect.TypeOf((*error)(nil)).Elem()
	contextType            = reflect.TypeOf((*context.Context)(nil)).Elem()
	httpResponseWriterType = reflect.TypeOf((*http.ResponseWriter)(nil)).Elem()
	httpHeaderType         = reflect.TypeOf(http.Header(nil))
	httpRequestType        = reflect.TypeOf((*http.Request)(nil))
	ioCloserType           = reflect.TypeOf((*io.Closer)(nil)).Elem()
)

// AddHandlers adds all the handlers in the given slice to r.
func AddHandlers(r *httprouter.Router, hs []Handler) {
	for _, h := range hs {
		r.Handle(h.Method, h.Path, h.Handle)
	}
}

// Handle converts a function into a Handler. The argument f
// must be a function of one of the following six forms, where ArgT
// must be a struct type acceptable to Unmarshal and ResultT is a type
// that can be marshaled as JSON:
//
//	func(p Params, arg *ArgT)
//	func(p Params, arg *ArgT) error
//	func(p Params, arg *ArgT) (ResultT, error)
//
//	func(arg *ArgT)
//	func(arg *ArgT) error
//	func(arg *ArgT) (ResultT, error)
//
// When processing a call to the returned handler, the provided
// parameters are unmarshaled into a new ArgT value using Unmarshal,
// then f is called with this value. If the unmarshaling fails, f will
// not be called and the unmarshal error will be written as a JSON
// response.
//
// As an additional special case to the rules defined in Unmarshal, the
// tag on an anonymous field of type Route specifies the method and path
// to use in the HTTP request. It should hold two space-separated
// fields; the first specifies the HTTP method, the second the URL path
// to use for the request. If this is given, the returned handler will
// hold that method and path, otherwise they will be empty.
//
// If an error is returned from f, it is passed through the error mapper
// before writing as a JSON response.
//
// In the third form, when no error is returned, the result is written
// as a JSON response with status http.StatusOK. Also in this case, any
// calls to Params.Response.Write or Params.Response.WriteHeader will be
// ignored, as the response code and data should be defined entirely by
// the returned result and error.
//
// Handle will panic if the provided function is not in one of the above
// forms.
func (srv *Server) Handle(f interface{}) Handler {
	fv := reflect.ValueOf(f)
	hf, err := srv.handlerFunc(fv.Type(), nil)
	if err != nil {
		panic(errgo.Notef(err, "bad handler function"))
	}
	return Handler{
		Method: hf.method,
		Path:   hf.pathPattern,
		Handle: func(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
			ctx, cancel := contextFromRequest(req)
			defer cancel()
			p1 := Params{
				Response:    w,
				Request:     req,
				PathVar:     p,
				PathPattern: hf.pathPattern,
				Context:     ctx,
			}
			argv, err := hf.unmarshal(p1)
			if err != nil {
				srv.WriteError(ctx, w, err)
				return
			}
			hf.call(fv, argv, p1)
		},
	}
}

// Handlers returns a list of handlers that will be handled by the value
// returned by the given argument, which must be a function in one of the
// following forms:
//
// 	func(p httprequest.Params) (T, context.Context, error)
// 	func(p httprequest.Params, handlerArg I) (T, context.Context, error)
//
// for some type T and some interface type I. Each exported method defined on T defines a handler,
// and should be in one of the forms accepted by Server.Handle
// with the additional constraint that the argument to each
// of the handlers must be compatible with the type I when the
// second form is used above.
//
// The returned context will be used as the value of Params.Context
// when Params is passed to any method. It will also be used
// when writing an error if the function returns an error.
// Note that it is OK to use both the standard library context.Context
// or golang.org/x/net/context.Context in the context return value.
//
// Handlers will panic if f is not of the required form, no methods are
// defined on T or any method defined on T is not suitable for Handle.
//
// When any of the returned handlers is invoked, f will be called and
// then the appropriate method will be called on the value it returns.
// If specified, the handlerArg parameter to f will hold the ArgT argument that
// will be passed to the handler method.
//
// If T implements io.Closer, its Close method will be called
// after the request is completed.
func (srv *Server) Handlers(f interface{}) []Handler {
	rootv := reflect.ValueOf(f)
	wt, argInterfacet, err := checkHandlersWrapperFunc(rootv)
	if err != nil {
		panic(errgo.Notef(err, "bad handler function"))
	}
	hasClose := wt.Implements(ioCloserType)
	hs := make([]Handler, 0, wt.NumMethod())
	for i := 0; i < wt.NumMethod(); i++ {
		i := i
		m := wt.Method(i)
		if m.PkgPath != "" {
			continue
		}
		if m.Name == "Close" {
			if !hasClose {
				panic(errgo.Newf("bad type for Close method (got %v want func(%v) error", m.Type, wt))
			}
			continue
		}
		h, err := srv.methodHandler(m, rootv, argInterfacet, hasClose)
		if err != nil {
			panic(err)
		}
		hs = append(hs, h)
	}
	if len(hs) == 0 {
		panic(errgo.Newf("no exported methods defined on %s", wt))
	}
	return hs
}

func (srv *Server) methodHandler(m reflect.Method, rootv reflect.Value, argInterfacet reflect.Type, hasClose bool) (Handler, error) {
	// The type in the Method struct includes the receiver type,
	// which we don't want to look at (and we won't see when
	// we get the method from the actual value at dispatch time),
	// so we hide it.
	mt := withoutReceiver(m.Type)
	hf, err := srv.handlerFunc(mt, argInterfacet)
	if err != nil {
		return Handler{}, errgo.Notef(err, "bad type for method %s", m.Name)
	}
	if hf.method == "" || hf.pathPattern == "" {
		return Handler{}, errgo.Notef(err, "method %s does not specify route method and path", m.Name)
	}
	handler := func(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
		ctx, cancel := contextFromRequest(req)
		defer cancel()
		p1 := Params{
			Response:    w,
			Request:     req,
			PathVar:     p,
			PathPattern: hf.pathPattern,
			Context:     ctx,
		}
		inv, err := hf.unmarshal(p1)
		if err != nil {
			srv.WriteError(ctx, w, err)
			return
		}
		var outv []reflect.Value
		if argInterfacet != nil {
			outv = rootv.Call([]reflect.Value{
				reflect.ValueOf(p1),
				// Pass the value to the root function so it can do wrappy things with it.
				// Note that because of the checks we've applied earlier, we can be
				// sure that the value will implement the interface type of this argument.
				inv,
			})
		} else {
			outv = rootv.Call([]reflect.Value{
				reflect.ValueOf(p1),
			})
		}
		tv, ctxv, errv := outv[0], outv[1], outv[2]
		// Get the context value robustly even if the
		// handler stupidly decides to return nil, and fall
		// back to the original context if it does.
		ctx1, _ := ctxv.Interface().(context.Context)
		if ctx1 != nil {
			ctx = ctx1
		}
		if !errv.IsNil() {
			srv.WriteError(ctx, w, errv.Interface().(error))
			return
		}
		if hasClose {
			defer tv.Interface().(io.Closer).Close()
		}
		hf.call(tv.Method(m.Index), inv, Params{
			Response:    w,
			Request:     req,
			PathVar:     p,
			PathPattern: hf.pathPattern,
			Context:     ctx,
		})
	}
	return Handler{
		Method: hf.method,
		Path:   hf.pathPattern,
		Handle: handler,
	}, nil
}

func checkHandlersWrapperFunc(fv reflect.Value) (returnt, argInterfacet reflect.Type, err error) {
	ft := fv.Type()
	if ft.Kind() != reflect.Func {
		return nil, nil, errgo.Newf("expected function, got %v", ft)
	}
	if fv.IsNil() {
		return nil, nil, errgo.Newf("function is nil")
	}
	if n := ft.NumIn(); n != 1 && n != 2 {
		return nil, nil, errgo.Newf("got %d arguments, want 1 or 2", n)
	}
	if n := ft.NumOut(); n != 3 {
		return nil, nil, errgo.Newf("function returns %d values, want (<T>, context.Context, error)", n)
	}
	if t := ft.In(0); t != paramsType {
		return nil, nil, errgo.Newf("invalid first argument, want httprequest.Params, got %v", t)
	}
	if ft.NumIn() > 1 {
		if t := ft.In(1); t.Kind() != reflect.Interface {
			return nil, nil, errgo.Newf("invalid second argument, want interface type, got %v", t)
		}
		argInterfacet = ft.In(1)
	}
	if t := ft.Out(1); !t.Implements(contextType) {
		return nil, nil, errgo.Newf("second return parameter of type %v does not implement context.Context", t)
	}
	if t := ft.Out(2); t != errorType {
		return nil, nil, errgo.Newf("invalid third return parameter, want error, got %v", t)
	}
	return ft.Out(0), argInterfacet, nil
}

func checkHandleType(t, argInterfacet reflect.Type) (*requestType, error) {
	if t.Kind() != reflect.Func {
		return nil, errgo.New("not a function")
	}
	if n := t.NumIn(); n != 1 && n != 2 {
		return nil, errgo.Newf("has %d parameters, need 1 or 2", t.NumIn())
	}
	if t.NumOut() > 2 {
		return nil, errgo.Newf("has %d result parameters, need 0, 1 or 2", t.NumOut())
	}
	if t.NumIn() == 2 {
		if t.In(0) != paramsType {
			return nil, errgo.Newf("first argument is %v, need httprequest.Params", t.In(0))
		}
	} else {
		if t.In(0) == paramsType {
			return nil, errgo.Newf("no argument parameter after Params argument")
		}
	}
	argt := t.In(t.NumIn() - 1)
	pt, err := getRequestType(argt)
	if err != nil {
		return nil, errgo.Notef(err, "last argument cannot be used for Unmarshal")
	}
	if argInterfacet != nil && !argt.Implements(argInterfacet) {
		return nil, errgo.Notef(err, "argument of type %v does not implement interface required by root handler %v", argt, argInterfacet)
	}
	if t.NumOut() > 0 {
		//	func(p Params, arg *ArgT) error
		//	func(p Params, arg *ArgT) (ResultT, error)
		if et := t.Out(t.NumOut() - 1); et != errorType {
			return nil, errgo.Newf("final result parameter is %s, need error", et)
		}
	}
	return pt, nil
}

// handlerFunc returns a function that will call a function of the given type,
// unmarshaling request parameters and marshaling the response as
// appropriate.
func (srv *Server) handlerFunc(ft, argInterfacet reflect.Type) (handlerFunc, error) {
	rt, err := checkHandleType(ft, argInterfacet)
	if err != nil {
		return handlerFunc{}, errgo.Mask(err)
	}
	return handlerFunc{
		unmarshal:   handlerUnmarshaler(ft, rt),
		call:        srv.handlerCaller(ft, rt),
		method:      rt.method,
		pathPattern: rt.path,
	}, nil
}

func handlerUnmarshaler(
	ft reflect.Type,
	rt *requestType,
) func(p Params) (reflect.Value, error) {
	argStructType := ft.In(ft.NumIn() - 1).Elem()
	return func(p Params) (reflect.Value, error) {
		if err := p.Request.ParseForm(); err != nil {
			return reflect.Value{}, errgo.WithCausef(err, ErrUnmarshal, "cannot parse HTTP request form")
		}
		argv := reflect.New(argStructType)
		if err := unmarshal(p, argv, rt); err != nil {
			return reflect.Value{}, errgo.NoteMask(err, "cannot unmarshal parameters", errgo.Is(ErrUnmarshal))
		}
		return argv, nil
	}
}

func (srv *Server) handlerCaller(
	ft reflect.Type,
	rt *requestType,
) func(fv, argv reflect.Value, p Params) {
	returnJSON := ft.NumOut() > 1
	needsParams := ft.In(0) == paramsType
	respond := srv.handlerResponder(ft)
	return func(fv, argv reflect.Value, p Params) {
		var rv []reflect.Value
		if needsParams {
			p := p
			if returnJSON {
				p.Response = headerOnlyResponseWriter{p.Response.Header()}
			}
			rv = fv.Call([]reflect.Value{
				reflect.ValueOf(p),
				argv,
			})
		} else {
			rv = fv.Call([]reflect.Value{
				argv,
			})
		}
		respond(p, rv)
	}
}

// handlerResponder handles the marshaling of the result values from the call to a function
// of type ft. The returned function accepts the values returned by the handler.
func (srv *Server) handlerResponder(ft reflect.Type) func(p Params, outv []reflect.Value) {
	switch ft.NumOut() {
	case 0:
		// func(...)
		return func(Params, []reflect.Value) {}
	case 1:
		// func(...) error
		return func(p Params, outv []reflect.Value) {
			if err := outv[0].Interface(); err != nil {
				srv.WriteError(p.Context, p.Response, err.(error))
			}
		}
	case 2:
		// func(...) (ResultT, error)
		return func(p Params, outv []reflect.Value) {
			if err := outv[1].Interface(); err != nil {
				srv.WriteError(p.Context, p.Response, err.(error))
				return
			}
			if err := WriteJSON(p.Response, http.StatusOK, outv[0].Interface()); err != nil {
				srv.WriteError(p.Context, p.Response, err)
			}
		}
	default:
		panic("unreachable")
	}
}

// ToHTTP converts an httprouter.Handle into an http.Handler.
// It will pass no path variables to h.
func ToHTTP(h httprouter.Handle) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		h(w, req, nil)
	})
}

// JSONHandler is like httprouter.Handle except that it returns a
// body (to be converted to JSON) and an error.
// The Header parameter can be used to set
// custom headers on the response.
type JSONHandler func(Params) (interface{}, error)

// ErrorHandler is like httprouter.Handle except it returns an error
// which may be returned as the error body of the response.
// An ErrorHandler function should not itself write to the ResponseWriter
// if it returns an error.
type ErrorHandler func(Params) error

// HandleJSON returns a handler that writes the return value of handle
// as a JSON response. If handle returns an error, it is passed through
// the error mapper.
//
// Note that the Params argument passed to handle will not
// have its PathPattern set as that information is not available.
func (srv *Server) HandleJSON(handle JSONHandler) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
		ctx, cancel := contextFromRequest(req)
		defer cancel()
		val, err := handle(Params{
			Response: headerOnlyResponseWriter{w.Header()},
			Request:  req,
			PathVar:  p,
			Context:  ctx,
		})
		if err == nil {
			if err = WriteJSON(w, http.StatusOK, val); err == nil {
				return
			}
		}
		srv.WriteError(ctx, w, err)
	}
}

// HandleErrors returns a handler that passes any non-nil error returned
// by handle through the error mapper and writes it as a JSON response.
//
// Note that the Params argument passed to handle will not
// have its PathPattern set as that information is not available.
func (srv *Server) HandleErrors(handle ErrorHandler) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
		w1 := responseWriter{
			ResponseWriter: w,
		}
		ctx, cancel := contextFromRequest(req)
		defer cancel()
		if err := handle(Params{
			Response: &w1,
			Request:  req,
			PathVar:  p,
			Context:  ctx,
		}); err != nil {
			if w1.headerWritten {
				// The header has already been written,
				// so we can't set the appropriate error
				// response code and there's a danger
				// that we may be corrupting the
				// response by appending a JSON error
				// message to it.
				// TODO log an error in this case.
				return
			}
			srv.WriteError(ctx, w, err)
		}
	}
}

// WriteError writes an error to a ResponseWriter and sets the HTTP
// status code, using srv.ErrorMapper to determine the actually written
// response.
//
// It uses WriteJSON to write the error body returned from the
// ErrorMapper so it is possible to add custom headers to the HTTP error
// response by implementing HeaderSetter.
func (srv *Server) WriteError(ctx context.Context, w http.ResponseWriter, err error) {
	if srv.ErrorWriter != nil {
		srv.ErrorWriter(ctx, w, err)
		return
	}
	errorMapper := srv.ErrorMapper
	if errorMapper == nil {
		errorMapper = DefaultErrorMapper
	}
	status, resp := errorMapper(ctx, err)
	err1 := WriteJSON(w, status, resp)
	if err1 == nil {
		return
	}
	// TODO log an error ?

	// JSON-marshaling the original error failed, so try to send that
	// error instead; if that fails, give up and go home.
	status1, resp1 := errorMapper(ctx, errgo.Notef(err1, "cannot marshal error response %q", err))
	err2 := WriteJSON(w, status1, resp1)
	if err2 == nil {
		return
	}

	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(fmt.Sprintf("really cannot marshal error response %q: %v", err, err1)))
}

// WriteJSON writes the given value to the ResponseWriter
// and sets the HTTP status to the given code.
//
// If val implements the HeaderSetter interface, the SetHeader
// method will be called to add additional headers to the
// HTTP response. It is called after the Content-Type header
// has been added, so can be used to override the content type
// if required.
func WriteJSON(w http.ResponseWriter, code int, val interface{}) error {
	// TODO consider marshalling directly to w using json.NewEncoder.
	// pro: this will not require a full buffer allocation.
	// con: if there's an error after the first write, it will be lost.
	data, err := json.Marshal(val)
	if err != nil {
		return errgo.Mask(err)
	}
	w.Header().Set("content-type", "application/json")
	if headerSetter, ok := val.(HeaderSetter); ok {
		headerSetter.SetHeader(w.Header())
	}
	w.WriteHeader(code)
	w.Write(data)
	return nil
}

// HeaderSetter is the interface checked for by WriteJSON.
// If implemented on a value passed to WriteJSON, the SetHeader
// method will be called to allow it to set custom headers
// on the response.
type HeaderSetter interface {
	SetHeader(http.Header)
}

// CustomHeader is a type that allows a JSON value to
// set custom HTTP headers associated with the
// HTTP response.
type CustomHeader struct {
	// Body holds the JSON-marshaled body of the response.
	Body interface{}

	// SetHeaderFunc holds a function that will be called
	// to set any custom headers on the response.
	SetHeaderFunc func(http.Header)
}

// MarshalJSON implements json.Marshaler by marshaling
// h.Body.
func (h CustomHeader) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.Body)
}

// SetHeader implements HeaderSetter by calling
// h.SetHeaderFunc.
func (h CustomHeader) SetHeader(header http.Header) {
	h.SetHeaderFunc(header)
}

// Ensure statically that responseWriter does implement http.Flusher.
var _ http.Flusher = (*responseWriter)(nil)

// responseWriter wraps http.ResponseWriter but allows us
// to find out whether any body has already been written.
type responseWriter struct {
	headerWritten bool
	http.ResponseWriter
}

func (w *responseWriter) Write(data []byte) (int, error) {
	w.headerWritten = true
	return w.ResponseWriter.Write(data)
}

func (w *responseWriter) WriteHeader(code int) {
	w.headerWritten = true
	w.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher.Flush.
func (w *responseWriter) Flush() {
	w.headerWritten = true
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

type headerOnlyResponseWriter struct {
	h http.Header
}

func (w headerOnlyResponseWriter) Header() http.Header {
	return w.h
}

func (w headerOnlyResponseWriter) Write([]byte) (int, error) {
	// TODO log or panic when this happens?
	return 0, errgo.New("inappropriate call to ResponseWriter.Write in JSON-returning handler")
}

func (w headerOnlyResponseWriter) WriteHeader(code int) {
	// TODO log or panic when this happens?
}

func withoutReceiver(t reflect.Type) reflect.Type {
	return withoutReceiverType{t}
}

type withoutReceiverType struct {
	reflect.Type
}

func (t withoutReceiverType) NumIn() int {
	return t.Type.NumIn() - 1
}

func (t withoutReceiverType) In(i int) reflect.Type {
	return t.Type.In(i + 1)
}
