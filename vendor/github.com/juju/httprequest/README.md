# httprequest
--
    import "github.com/juju/httprequest"

Package httprequest provides functionality for unmarshaling HTTP request
parameters into a struct type.

Please note that the API is not considered stable at this point and may be
changed in a backwardly incompatible manner at any time.

## Usage

```go
var (
	ErrUnmarshal        = errgo.New("httprequest unmarshal error")
	ErrBadUnmarshalType = errgo.New("httprequest bad unmarshal type")
)
```

#### func  Marshal

```go
func Marshal(baseURL, method string, x interface{}) (*http.Request, error)
```
Marshal is the counterpart of Unmarshal. It takes information from x, which must
be a pointer to a struct, and returns an HTTP request using the given method
that holds all of the information

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

For example, this code:

    type UserDetails struct {
        Age int
    }

    type Test struct {
        Username string `httprequest:"user,path"`
        ContextId int64 `httprequest:"context,form"`
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

    "form" - the field is taken from the given name in p.Form
    	(note that this covers both URL query parameters and
    	POST form parameters)

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

#### func  WriteJSON

```go
func WriteJSON(w http.ResponseWriter, code int, val interface{}) error
```
WriteJSON writes the given value to the ResponseWriter and sets the HTTP status
to the given code.

#### type ErrorHandler

```go
type ErrorHandler func(http.ResponseWriter, Params) error
```

ErrorHandler is like httprouter.Handle except it returns an error which may be
returned as the error body of the response. An ErrorHandler function should not
itself write to the ResponseWriter if it returns an error.

#### type ErrorMapper

```go
type ErrorMapper func(err error) (httpStatus int, errorBody interface{})
```

ErrorMapper holds a function that can convert a Go error into a form that can be
returned as a JSON body from an HTTP request. The httpStatus value reports the
desired HTTP status.

#### func (ErrorMapper) Handle

```go
func (e ErrorMapper) Handle(f interface{}) httprouter.Handle
```
Handle converts a function into an httprouter.Handle. The argument f must be a
function of one of the following three forms, where ArgT must be a struct type
acceptable to Unmarshal and ResultT is a type that can be marshaled as JSON:

    func(w http.ResponseWriter, p Params, arg *ArgT)
    func(w http.ResponseWriter, p Params, arg *ArgT) error
    func(header http.Header, p Params, arg *ArgT) (ResultT, error)

When processing a call to the returned handler, the provided parameters are
unmarshaled into a new ArgT value using Unmarshal, then f is called with this
value. If the unmarshaling fails, f will not be called and the unmarshal error
will be written as a JSON response.

If an error is returned from f, it is passed through the error mapper before
writing as a JSON response.

In the third form, when no error is returned, the result is written as a JSON
response with status http.StatusOK.

Handle will panic if the provided function is not in one of the above forms.

#### func (ErrorMapper) HandleErrors

```go
func (e ErrorMapper) HandleErrors(handle ErrorHandler) httprouter.Handle
```
HandleErrors returns a handler that passes any non-nil error returned by handle
through the error mapper and writes it as a JSON response.

#### func (ErrorMapper) HandleJSON

```go
func (e ErrorMapper) HandleJSON(handle JSONHandler) httprouter.Handle
```
HandleJSON returns a handler that writes the return value of handle as a JSON
response. If handle returns an error, it is passed through the error mapper.

#### func (ErrorMapper) WriteError

```go
func (e ErrorMapper) WriteError(w http.ResponseWriter, err error)
```
WriteError writes an error to a ResponseWriter and sets the HTTP status code.

#### type JSONHandler

```go
type JSONHandler func(http.Header, Params) (interface{}, error)
```

JSONHandler is like httprouter.Handle except that it returns a body (to be
converted to JSON) and an error. The Header parameter can be used to set custom
headers on the response.

#### type Params

```go
type Params struct {
	*http.Request
	PathVar httprouter.Params
}
```

Params holds request parameters that can be unmarshaled into a struct.
