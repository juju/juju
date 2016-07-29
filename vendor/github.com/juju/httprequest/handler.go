// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package httprequest

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"

	"github.com/julienschmidt/httprouter"
	"gopkg.in/errgo.v1"
)

// ErrorMapper holds a function that can convert a Go error
// into a form that can be returned as a JSON body from an HTTP request.
//
// The httpStatus value reports the desired HTTP status.
//
// If the returned errorBody implements HeaderSetter, then
// that method will be called to add custom headers to the request.
type ErrorMapper func(err error) (httpStatus int, errorBody interface{})

var (
	paramsType             = reflect.TypeOf(Params{})
	errorType              = reflect.TypeOf((*error)(nil)).Elem()
	httpResponseWriterType = reflect.TypeOf((*http.ResponseWriter)(nil)).Elem()
	httpHeaderType         = reflect.TypeOf(http.Header(nil))
	httpRequestType        = reflect.TypeOf((*http.Request)(nil))
	ioCloserType           = reflect.TypeOf((*io.Closer)(nil)).Elem()
)

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
// As an additional special case to the rules defined in Unmarshal,
// the tag on an anonymous field of type Route
// specifies the method and path to use in the HTTP request.
// It should hold two space-separated fields; the first specifies
// the HTTP method, the second the URL path to use for the request.
// If this is given, the returned handler will hold that
// method and path, otherwise they will be empty.
//
// If an error is returned from f, it is passed through the error mapper before
// writing as a JSON response.
//
// In the third form, when no error is returned, the result is written
// as a JSON response with status http.StatusOK. Also in this case,
// any calls to Params.Response.Write or Params.Response.WriteHeader
// will be ignored, as the response code and data should be defined
// entirely by the returned result and error.
//
// Handle will panic if the provided function is not in one
// of the above forms.
func (e ErrorMapper) Handle(f interface{}) Handler {
	fv := reflect.ValueOf(f)
	hf, rt, err := e.handlerFunc(fv.Type())
	if err != nil {
		panic(errgo.Notef(err, "bad handler function"))
	}
	return Handler{
		Method: rt.method,
		Path:   rt.path,
		Handle: func(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
			hf(fv, Params{
				Response:    w,
				Request:     req,
				PathVar:     p,
				PathPattern: rt.path,
			})
		},
	}
}

// Handlers returns a list of handlers that will be handled by the value
// returned by the given argument, which must be a function of the form:
//
// 	func(httprequest.Params) (T, error)
//
// for some type T. Each exported method defined on T defines a handler,
// and should be in one of the forms accepted by ErrorMapper.Handle.
//
// Handlers will panic if f is not of the required form, no methods are
// defined on T or any method defined on T is not suitable for Handle.
//
// When any of the returned handlers is invoked, f will be called and
// then the appropriate method will be called on the value it returns.
//
// If T implements io.Closer, its Close method will be called
// after the request is completed.
func (e ErrorMapper) Handlers(f interface{}) []Handler {
	fv := reflect.ValueOf(f)
	wt, err := checkHandlersWrapperFunc(fv)
	if err != nil {
		panic(errgo.Notef(err, "bad handler function"))
	}
	hasClose := wt.Implements(ioCloserType)
	hs := make([]Handler, 0, wt.NumMethod())
	numMethod := 0
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
		// The type in the Method struct includes the receiver type,
		// which we don't want to look at (and we won't see when
		// we get the method from the actual value at dispatch time),
		// so we hide it.
		mt := withoutReceiver(m.Type)
		hf, rt, err := e.handlerFunc(mt)
		if err != nil {
			panic(errgo.Notef(err, "bad type for method %s", m.Name))
		}
		if rt.method == "" || rt.path == "" {
			panic(errgo.Notef(err, "method %s does not specify route method and path", m.Name))
		}
		handler := func(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
			terrv := fv.Call([]reflect.Value{
				reflect.ValueOf(Params{
					Response:    w,
					Request:     req,
					PathVar:     p,
					PathPattern: rt.path,
				}),
			})
			tv, errv := terrv[0], terrv[1]
			if !errv.IsNil() {
				e.WriteError(w, errv.Interface().(error))
				return
			}
			if hasClose {
				defer tv.Interface().(io.Closer).Close()
			}
			hf(tv.Method(i), Params{
				Response:    w,
				Request:     req,
				PathVar:     p,
				PathPattern: rt.path,
			})

		}
		hs = append(hs, Handler{
			Method: rt.method,
			Path:   rt.path,
			Handle: handler,
		})
		numMethod++
	}
	if numMethod == 0 {
		panic(errgo.Newf("no exported methods defined on %s", wt))
	}
	return hs
}

func checkHandlersWrapperFunc(fv reflect.Value) (reflect.Type, error) {
	ft := fv.Type()
	if ft.Kind() != reflect.Func {
		return nil, errgo.Newf("expected function, got %v", ft)
	}
	if fv.IsNil() {
		return nil, errgo.Newf("function is nil")
	}
	if n := ft.NumIn(); n != 1 {
		return nil, errgo.Newf("got %d arguments, want 1", n)
	}
	if n := ft.NumOut(); n != 2 {
		return nil, errgo.Newf("function returns %d values, want 2", n)
	}
	if ft.In(0) != paramsType ||
		ft.Out(1) != errorType {
		return nil, errgo.Newf("invalid argument or return values, want func(httprequest.Params) (any, error), got %v", ft)
	}
	return ft.Out(0), nil
}

// Handler defines a HTTP handler that will handle the
// given HTTP method at the given httprouter path
type Handler struct {
	Method string
	Path   string
	Handle httprouter.Handle
}

func checkHandleType(t reflect.Type) (*requestType, error) {
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
	pt, err := getRequestType(t.In(t.NumIn() - 1))
	if err != nil {
		return nil, errgo.Notef(err, "last argument cannot be used for Unmarshal")
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
func (e ErrorMapper) handlerFunc(ft reflect.Type) (func(fv reflect.Value, p Params), *requestType, error) {
	rt, err := checkHandleType(ft)
	if err != nil {
		return nil, nil, errgo.Mask(err)
	}
	return e.handleResult(ft, handleParams(ft, rt)), rt, nil
}

// handleParams handles unmarshaling the parameters to be passed to
// a function of type ft. The rt parameter describes ft (as determined by
// checkHandleType). The returned function accepts the actual function
// value to use in the call as well as the request parameters and returns
// the result value to use for marshaling.
func handleParams(
	ft reflect.Type,
	rt *requestType,
) func(fv reflect.Value, p Params) ([]reflect.Value, error) {
	returnJSON := ft.NumOut() > 1
	needsParams := ft.In(0) == paramsType
	if needsParams {
		argStructType := ft.In(1).Elem()
		return func(fv reflect.Value, p Params) ([]reflect.Value, error) {
			if err := p.Request.ParseForm(); err != nil {
				return nil, errgo.WithCausef(err, ErrUnmarshal, "cannot parse HTTP request form")
			}
			if returnJSON {
				p.Response = headerOnlyResponseWriter{p.Response.Header()}
			}
			argv := reflect.New(argStructType)
			if err := unmarshal(p, argv, rt); err != nil {
				return nil, errgo.NoteMask(err, "cannot unmarshal parameters", errgo.Is(ErrUnmarshal))
			}
			return fv.Call([]reflect.Value{
				reflect.ValueOf(p),
				argv,
			}), nil
		}
	}
	argStructType := ft.In(0).Elem()
	return func(fv reflect.Value, p Params) ([]reflect.Value, error) {
		if err := p.Request.ParseForm(); err != nil {
			return nil, errgo.WithCausef(err, ErrUnmarshal, "cannot parse HTTP request form")
		}
		argv := reflect.New(argStructType)
		if err := unmarshal(p, argv, rt); err != nil {
			return nil, errgo.NoteMask(err, "cannot unmarshal parameters", errgo.Is(ErrUnmarshal))
		}
		return fv.Call([]reflect.Value{argv}), nil
	}

}

// handleResult handles the marshaling of the result values from the call to a function
// of type ft. The returned function accepts the actual function value to use in the
// call as well as the request parameters.
func (e ErrorMapper) handleResult(
	ft reflect.Type,
	f func(fv reflect.Value, p Params) ([]reflect.Value, error),
) func(fv reflect.Value, p Params) {
	switch ft.NumOut() {
	case 0:
		//	func(w http.ResponseWriter, p Params, arg *ArgT)
		return func(fv reflect.Value, p Params) {
			_, err := f(fv, p)
			if err != nil {
				e.WriteError(p.Response, err)
			}
		}
	case 1:
		//	func(w http.ResponseWriter, p Params, arg *ArgT) error
		return func(fv reflect.Value, p Params) {
			out, err := f(fv, p)
			if err != nil {
				e.WriteError(p.Response, err)
				return
			}
			herr := out[0].Interface()
			if herr != nil {
				e.WriteError(p.Response, herr.(error))
			}
		}
	case 2:
		//	func(header http.Header, p Params, arg *ArgT) (ResultT, error)
		return func(fv reflect.Value, p Params) {
			out, err := f(fv, p)
			if err != nil {
				e.WriteError(p.Response, err)
				return
			}
			herr := out[1].Interface()
			if herr != nil {
				e.WriteError(p.Response, herr.(error))
				return
			}
			err = WriteJSON(p.Response, http.StatusOK, out[0].Interface())
			if err != nil {
				e.WriteError(p.Response, err)
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
func (e ErrorMapper) HandleJSON(handle JSONHandler) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
		val, err := handle(Params{
			Response: headerOnlyResponseWriter{w.Header()},
			Request:  req,
			PathVar:  p,
		})
		if err == nil {
			if err = WriteJSON(w, http.StatusOK, val); err == nil {
				return
			}
		}
		e.WriteError(w, err)
	}
}

// HandleErrors returns a handler that passes any non-nil error returned
// by handle through the error mapper and writes it as a JSON response.
//
// Note that the Params argument passed to handle will not
// have its PathPattern set as that information is not available.
func (e ErrorMapper) HandleErrors(handle ErrorHandler) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
		w1 := responseWriter{
			ResponseWriter: w,
		}
		if err := handle(Params{
			Response: &w1,
			Request:  req,
			PathVar:  p,
		}); err != nil {
			// We write the error only if the header hasn't
			// already been written, because if it has, then
			// we will not be able to set the appropriate error
			// response code, and there's a danger that we
			// may be corrupting output by appending
			// a JSON error message to it.
			if !w1.headerWritten {
				e.WriteError(w, err)
			}
			// TODO log the error?
		}
	}
}

// WriteError writes an error to a ResponseWriter
// and sets the HTTP status code.
//
// It uses WriteJSON to write the error body returned from
// the ErrorMapper so it is possible to add custom
// headers to the HTTP error response by implementing
// HeaderSetter.
func (e ErrorMapper) WriteError(w http.ResponseWriter, err error) {
	status, resp := e(err)
	WriteJSON(w, status, resp)
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
		// TODO(rog) log an error if this fails and lose the
		// error return, because most callers will need
		// to do that anyway.
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
