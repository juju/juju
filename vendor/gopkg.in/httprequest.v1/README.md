# httprequest
--
    import "gopkg.in/httprequest.v1"

Package httprequest provides functionality for unmarshaling HTTP request
parameters into a struct type.

## Usage

```go
var (
	ErrUnmarshal        = errgo.New("httprequest unmarshal error")
	ErrBadUnmarshalType = errgo.New("httprequest bad unmarshal type")
)
```

```go
var DefaultErrorUnmarshaler = ErrorUnmarshaler(new(RemoteError))
```
DefaultErrorUnmarshaler is the default error unmarshaler used by Client.

#### func  AddHandlers

```go
func AddHandlers(r *httprouter.Router, hs []Handler)
```
AddHandlers adds all the handlers in the given slice to r.

#### func  ErrorUnmarshaler

```go
func ErrorUnmarshaler(template error) func(*http.Response) error
```
ErrorUnmarshaler returns a function which will unmarshal error responses into
new values of the same type as template. The argument must be a pointer. A new
instance of it is created every time the returned function is called.

If the error cannot by unmarshaled, the function will return an
*HTTPResponseError holding the response from the request.

#### func  Marshal

```go
func Marshal(baseURL, method string, x interface{}) (*http.Request, error)
```
Marshal is the counterpart of Unmarshal. It takes information from x, which must
be a pointer to a struct, and returns an HTTP request using the given method
that holds all of the information.

The Body field in the returned request will always be of type BytesReaderCloser.

If x implements the HeaderSetter interface, its SetHeader method will be called
to add additional headers to the HTTP request after it has been marshaled. If x
is pointer to a CustomHeader object then Marshal will use its Body member to
create the HTTP request.

The HTTP request will use the given method. Named fields in the given baseURL
will be filled out from "path"-tagged fields in x to form the URL path in the
returned request. These are specified as for httprouter.

If a field in baseURL is a suffix of the form "*var" (a trailing wildcard
element that holds the rest of the path), the marshaled string must begin with a
"/". This matches the httprouter convention that it always returns such fields
with a "/" prefix.

If a field is of type string or []string, the value of the field will be used
directly; otherwise if implements encoding.TextMarshaler, that will be used to
marshal the field, otherwise fmt.Sprint will be used.

An "omitempty" attribute on a form or header field specifies that if the form or
header value is empty, the form or header entry will be omitted.

For example, this code:

    type UserDetails struct {
        Age int
    }

    type Test struct {
        Username string `httprequest:"user,path"`
        ContextId int64 `httprequest:"context,form"`
        Extra string `httprequest:"context,form,omitempty"`
        Details UserDetails `httprequest:",body"`
    }
    req, err := Marshal("GET", "http://example.com/users/:user/details", &Test{
        Username: "bob",
        ContextId: 1234,
        Details: UserDetails{
            Age: 36,
        }
    })
    if err != nil {
        ...
    }

will produce an HTTP request req with a URL of
http://example.com/users/bob/details?context=1234 and a JSON-encoded body
holding `{"Age":36}`.

It is an error if there is a field specified in the URL that is not found in x.

#### func  ToHTTP

```go
func ToHTTP(h httprouter.Handle) http.Handler
```
ToHTTP converts an httprouter.Handle into an http.Handler. It will pass no path
variables to h.

#### func  Unmarshal

```go
func Unmarshal(p Params, x interface{}) error
```
Unmarshal takes values from given parameters and fills out fields in x, which
must be a pointer to a struct.

Tags on the struct's fields determine where each field is filled in from.
Similar to encoding/json and other encoding packages, the tag holds a
comma-separated list. The first item in the list is an alternative name for the
field (the field name itself will be used if this is empty). The next item
specifies where the field is filled in from. It may be:

    "path" - the field is taken from a parameter in p.PathVar
    	with a matching field name.

    "form" - the field is taken from the given name in p.Request.Form
    	(note that this covers both URL query parameters and
    	POST form parameters).

    "header" - the field is taken from the given name in
    	p.Request.Header.

    "body" - the field is filled in by parsing the request body
    	as JSON.

For path and form parameters, the field will be filled out from the field in
p.PathVar or p.Form using one of the following methods (in descending order of
preference):

- if the type is string, it will be set from the first value.

- if the type is []string, it will be filled out using all values for that field

    (allowed only for form)

- if the type implements encoding.TextUnmarshaler, its UnmarshalText method will
be used

- otherwise fmt.Sscan will be used to set the value.

When the unmarshaling fails, Unmarshal returns an error with an ErrUnmarshal
cause. If the type of x is inappropriate, it returns an error with an
ErrBadUnmarshalType cause.

#### func  UnmarshalJSONResponse

```go
func UnmarshalJSONResponse(resp *http.Response, x interface{}) error
```
UnmarshalJSONResponse unmarshals the given HTTP response into x, which should be
a pointer to the result to be unmarshaled into.

If the response cannot be unmarshaled, an error of type *DecodeResponseError
will be returned.

#### func  WriteJSON

```go
func WriteJSON(w http.ResponseWriter, code int, val interface{}) error
```
WriteJSON writes the given value to the ResponseWriter and sets the HTTP status
to the given code.

If val implements the HeaderSetter interface, the SetHeader method will be
called to add additional headers to the HTTP response. It is called after the
Content-Type header has been added, so can be used to override the content type
if required.

#### type BytesReaderCloser

```go
type BytesReaderCloser struct {
	*bytes.Reader
}
```

BytesReaderCloser is a bytes.Reader which implements io.Closer with a no-op
Close method.

#### func (BytesReaderCloser) Close

```go
func (BytesReaderCloser) Close() error
```
Close implements io.Closer.Close.

#### type Client

```go
type Client struct {
	// BaseURL holds the base URL to use when making
	// HTTP requests.
	BaseURL string

	// Doer holds a value that will be used to actually
	// make the HTTP request. If it is nil, http.DefaultClient
	// will be used instead. If Doer implements DoerWithContext,
	// DoWithContext will be used instead.
	Doer Doer

	// If a request returns an HTTP response that signifies an
	// error, UnmarshalError is used to unmarshal the response into
	// an appropriate error. See ErrorUnmarshaler for a convenient
	// way to create an UnmarshalError function for a given type. If
	// this is nil, DefaultErrorUnmarshaler will be used.
	UnmarshalError func(resp *http.Response) error
}
```

Client represents a client that can invoke httprequest endpoints.

#### func (*Client) Call

```go
func (c *Client) Call(ctx context.Context, params, resp interface{}) error
```
Call invokes the endpoint implied by the given params, which should be of the
form accepted by the ArgT argument to a function passed to Handle, and
unmarshals the response into the given response parameter, which should be a
pointer to the response value.

If params implements the HeaderSetter interface, its SetHeader method will be
called to add additional headers to the HTTP request.

If resp is nil, the response will be ignored if the request was successful.

If resp is of type **http.Response, instead of unmarshaling into it, its element
will be set to the returned HTTP response directly and the caller is responsible
for closing its Body field.

Any error that c.UnmarshalError or c.Doer returns will not have its cause
masked.

If the request returns a response with a status code signifying success, but the
response could not be unmarshaled, a *DecodeResponseError will be returned
holding the response. Note that if the request returns an error status code, the
Client.UnmarshalError function is responsible for doing this if desired (the
default error unmarshal functions do).

#### func (*Client) CallURL

```go
func (c *Client) CallURL(ctx context.Context, url string, params, resp interface{}) error
```
CallURL is like Call except that the given URL is used instead of c.BaseURL.

#### func (*Client) Do

```go
func (c *Client) Do(ctx context.Context, req *http.Request, resp interface{}) error
```
Do sends the given request and unmarshals its JSON result into resp, which
should be a pointer to the response value. If an error status is returned, the
error will be unmarshaled as in Client.Call.

If resp is nil, the response will be ignored if the response was successful.

If resp is of type **http.Response, instead of unmarshaling into it, its element
will be set to the returned HTTP response directly and the caller is responsible
for closing its Body field.

Any error that c.UnmarshalError or c.Doer returns will not have its cause
masked.

If req.URL does not have a host part it will be treated as relative to
c.BaseURL. req.URL will be updated to the actual URL used.

If the response cannot by unmarshaled, a *DecodeResponseError will be returned
holding the response from the request. the entire response body.

#### func (*Client) Get

```go
func (c *Client) Get(ctx context.Context, url string, resp interface{}) error
```
Get is a convenience method that uses c.Do to issue a GET request to the given
URL. If the given URL does not have a host part then it will be treated as
relative to c.BaseURL.

#### type CustomHeader

```go
type CustomHeader struct {
	// Body holds the JSON-marshaled body of the response.
	Body interface{}

	// SetHeaderFunc holds a function that will be called
	// to set any custom headers on the response.
	SetHeaderFunc func(http.Header)
}
```

CustomHeader is a type that allows a JSON value to set custom HTTP headers
associated with the HTTP response.

#### func (CustomHeader) MarshalJSON

```go
func (h CustomHeader) MarshalJSON() ([]byte, error)
```
MarshalJSON implements json.Marshaler by marshaling h.Body.

#### func (CustomHeader) SetHeader

```go
func (h CustomHeader) SetHeader(header http.Header)
```
SetHeader implements HeaderSetter by calling h.SetHeaderFunc.

#### type DecodeRequestError

```go
type DecodeRequestError struct {
	// Request holds the problematic HTTP request.
	// The body of this does not need to be closed
	// and may be truncated if the response is large.
	Request *http.Request

	// DecodeError holds the error that was encountered
	// when decoding.
	DecodeError error
}
```

DecodeRequestError represents an error when an HTTP request could not be
decoded.

#### func (*DecodeRequestError) Error

```go
func (e *DecodeRequestError) Error() string
```

#### type DecodeResponseError

```go
type DecodeResponseError struct {
	// Response holds the problematic HTTP response.
	// The body of this does not need to be closed
	// and may be truncated if the response is large.
	Response *http.Response

	// DecodeError holds the error that was encountered
	// when decoding.
	DecodeError error
}
```

DecodeResponseError represents an error when an HTTP response could not be
decoded.

#### func (*DecodeResponseError) Error

```go
func (e *DecodeResponseError) Error() string
```

#### type Doer

```go
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}
```

Doer is implemented by HTTP client packages to make an HTTP request. It is
notably implemented by http.Client and httpbakery.Client.

#### type DoerWithContext

```go
type DoerWithContext interface {
	DoWithContext(ctx context.Context, req *http.Request) (*http.Response, error)
}
```

DoerWithContext is implemented by HTTP clients that can use a context with the
HTTP request.

#### type ErrorHandler

```go
type ErrorHandler func(Params) error
```

ErrorHandler is like httprouter.Handle except it returns an error which may be
returned as the error body of the response. An ErrorHandler function should not
itself write to the ResponseWriter if it returns an error.

#### type Handler

```go
type Handler struct {
	Method string
	Path   string
	Handle httprouter.Handle
}
```

Handler defines a HTTP handler that will handle the given HTTP method at the
given httprouter path

#### type HeaderSetter

```go
type HeaderSetter interface {
	SetHeader(http.Header)
}
```

HeaderSetter is the interface checked for by WriteJSON. If implemented on a
value passed to WriteJSON, the SetHeader method will be called to allow it to
set custom headers on the response.

#### type JSONHandler

```go
type JSONHandler func(Params) (interface{}, error)
```

JSONHandler is like httprouter.Handle except that it returns a body (to be
converted to JSON) and an error. The Header parameter can be used to set custom
headers on the response.

#### type Params

```go
type Params struct {
	Response http.ResponseWriter
	Request  *http.Request
	PathVar  httprouter.Params
	// PathPattern holds the path pattern matched by httprouter.
	// It is only set where httprequest has the information;
	// that is where the call was made by Server.Handler
	// or Server.Handlers.
	PathPattern string
	// Context holds a context for the request. In Go 1.7 and later,
	// this should be used in preference to Request.Context.
	Context context.Context
}
```

Params holds the parameters provided to an HTTP request.

#### type RemoteError

```go
type RemoteError struct {
	// Message holds the error message.
	Message string

	// Code may hold a code that classifies the error.
	Code string `json:",omitempty"`

	// Info holds any other information associated with the error.
	Info *json.RawMessage `json:",omitempty"`
}
```

RemoteError holds the default type of a remote error used by Client when no
custom error unmarshaler is set.

#### func (*RemoteError) Error

```go
func (e *RemoteError) Error() string
```
Error implements the error interface.

#### type Route

```go
type Route struct{}
```

Route is the type of a field that specifies a routing path and HTTP method. See
Marshal and Unmarshal for details.

#### type Server

```go
type Server struct {
	// ErrorMapper holds a function that can convert a Go error
	// into a form that can be returned as a JSON body from an HTTP request.
	//
	// The httpStatus value reports the desired HTTP status.
	//
	// If the returned errorBody implements HeaderSetter, then
	// that method will be called to add custom headers to the request.
	ErrorMapper func(ctxt context.Context, err error) (httpStatus int, errorBody interface{})
}
```

Server represents the server side of an HTTP servers, and can be used to create
HTTP handlers although it is not an HTTP handler itself.

#### func (*Server) Handle

```go
func (srv *Server) Handle(f interface{}) Handler
```
Handle converts a function into a Handler. The argument f must be a function of
one of the following six forms, where ArgT must be a struct type acceptable to
Unmarshal and ResultT is a type that can be marshaled as JSON:

    func(p Params, arg *ArgT)
    func(p Params, arg *ArgT) error
    func(p Params, arg *ArgT) (ResultT, error)

    func(arg *ArgT)
    func(arg *ArgT) error
    func(arg *ArgT) (ResultT, error)

When processing a call to the returned handler, the provided parameters are
unmarshaled into a new ArgT value using Unmarshal, then f is called with this
value. If the unmarshaling fails, f will not be called and the unmarshal error
will be written as a JSON response.

As an additional special case to the rules defined in Unmarshal, the tag on an
anonymous field of type Route specifies the method and path to use in the HTTP
request. It should hold two space-separated fields; the first specifies the HTTP
method, the second the URL path to use for the request. If this is given, the
returned handler will hold that method and path, otherwise they will be empty.

If an error is returned from f, it is passed through the error mapper before
writing as a JSON response.

In the third form, when no error is returned, the result is written as a JSON
response with status http.StatusOK. Also in this case, any calls to
Params.Response.Write or Params.Response.WriteHeader will be ignored, as the
response code and data should be defined entirely by the returned result and
error.

Handle will panic if the provided function is not in one of the above forms.

#### func (*Server) HandleErrors

```go
func (srv *Server) HandleErrors(handle ErrorHandler) httprouter.Handle
```
HandleErrors returns a handler that passes any non-nil error returned by handle
through the error mapper and writes it as a JSON response.

Note that the Params argument passed to handle will not have its PathPattern set
as that information is not available.

#### func (*Server) HandleJSON

```go
func (srv *Server) HandleJSON(handle JSONHandler) httprouter.Handle
```
HandleJSON returns a handler that writes the return value of handle as a JSON
response. If handle returns an error, it is passed through the error mapper.

Note that the Params argument passed to handle will not have its PathPattern set
as that information is not available.

#### func (Server) Handlers

```go
func (srv Server) Handlers(f interface{}) []Handler
```
Handlers returns a list of handlers that will be handled by the value returned
by the given argument, which must be a function in one of the following forms:

    func(p httprequest.Params) (T, context.Context, error)
    func(p httprequest.Params, handlerArg I) (T, context.Context, error)

for some type T and some interface type I. Each exported method defined on T
defines a handler, and should be in one of the forms accepted by
ErrorMapper.Handle with the additional constraint that the argument to each of
the handlers must be compatible with the type I when the second form is used
above.

The returned context will be used as the value of Params.Context when Params is
passed to any method. It will also be used when writing an error if the function
returns an error. Note that it is OK to use both the standard library
context.Context or golang.org/x/net/context.Context in the context return value.

Handlers will panic if f is not of the required form, no methods are defined on
T or any method defined on T is not suitable for Handle.

When any of the returned handlers is invoked, f will be called and then the
appropriate method will be called on the value it returns. If specified, the
handlerArg parameter to f will hold the ArgT argument that will be passed to the
handler method.

If T implements io.Closer, its Close method will be called after the request is
completed.

#### func (*Server) WriteError

```go
func (srv *Server) WriteError(ctx context.Context, w http.ResponseWriter, err error)
```
WriteError writes an error to a ResponseWriter and sets the HTTP status code.

It uses WriteJSON to write the error body returned from the ErrorMapper so it is
possible to add custom headers to the HTTP error response by implementing
HeaderSetter.
