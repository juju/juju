// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package httprequest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/julienschmidt/httprouter"
	"gopkg.in/errgo.v1"
)

var emptyReader = bytes.NewReader(nil)

// Marshal is the counterpart of Unmarshal. It takes information from
// x, which must be a pointer to a struct, and returns an HTTP request
// using the given method that holds all of the information.
//
// The Body field in the returned request will always be of type
// BytesReaderCloser.
//
// If x implements the HeaderSetter interface, its SetHeader method will
// be called to add additional headers to the HTTP request after it has
// been marshaled. If x is pointer to a CustomHeader object then Marshal will use
// its Body member to create the HTTP request.
//
// The HTTP request will use the given method.  Named fields in the given
// baseURL will be filled out from "path"-tagged fields in x to form the
// URL path in the returned request.  These are specified as for httprouter.
//
// If a field in baseURL is a suffix of the form "*var" (a trailing wildcard element
// that holds the rest of the path), the marshaled string must begin with a "/".
// This matches the httprouter convention that it always returns such fields
// with a "/" prefix.
//
// If a field is of type string or []string, the value of the field will
// be used directly; otherwise if implements encoding.TextMarshaler, that
// will be used to marshal the field, otherwise fmt.Sprint will be used.
//
// For example, this code:
//
//	type UserDetails struct {
//	    Age int
//	}
//
//	type Test struct {
//	    Username string `httprequest:"user,path"`
//	    ContextId int64 `httprequest:"context,form"`
//	    Details UserDetails `httprequest:",body"`
//	}
//	req, err := Marshal("GET", "http://example.com/users/:user/details", &Test{
//	    Username: "bob",
//	    ContextId: 1234,
//	    Details: UserDetails{
//	        Age: 36,
//	    }
//	})
//	if err != nil {
//	    ...
//	}
//
// will produce an HTTP request req with a URL of
// http://example.com/users/bob/details?context=1234 and a JSON-encoded
// body holding `{"Age":36}`.
//
// It is an error if there is a field specified in the URL that is not
// found in x.
func Marshal(baseURL, method string, x interface{}) (*http.Request, error) {
	var xv reflect.Value
	if ch, ok := x.(*CustomHeader); ok {
		xv = reflect.ValueOf(ch.Body)
	} else {
		xv = reflect.ValueOf(x)
	}
	pt, err := getRequestType(xv.Type())
	if err != nil {
		return nil, errgo.WithCausef(err, ErrBadUnmarshalType, "bad type %s", xv.Type())
	}
	req, err := http.NewRequest(method, baseURL, BytesReaderCloser{emptyReader})
	if err != nil {
		return nil, errgo.Mask(err)
	}
	req.Form = url.Values{}
	p := &Params{
		Request: req,
	}
	if err := marshal(p, xv, pt); err != nil {
		return nil, errgo.Mask(err, errgo.Is(ErrUnmarshal))
	}
	if headerSetter, ok := x.(HeaderSetter); ok {
		headerSetter.SetHeader(p.Request.Header)
	}
	return p.Request, nil
}

// marshal is the internal version of Marshal.
func marshal(p *Params, xv reflect.Value, pt *requestType) error {
	xv = xv.Elem()
	for _, f := range pt.fields {
		fv := xv.FieldByIndex(f.index)
		if f.isPointer {
			if fv.IsNil() {
				continue
			}
			fv = fv.Elem()
		}
		// TODO store the field name in the field so
		// that we can produce a nice error message.
		if err := f.marshal(fv, p); err != nil {
			return errgo.WithCausef(err, ErrUnmarshal, "cannot marshal field")
		}
	}
	path, err := buildPath(p.Request.URL.Path, p.PathVar)
	if err != nil {
		return errgo.Mask(err)
	}
	p.Request.URL.Path = path
	if q := p.Request.Form.Encode(); q != "" && p.Request.URL.RawQuery != "" {
		p.Request.URL.RawQuery += "&" + q
	} else {
		p.Request.URL.RawQuery += q
	}
	return nil
}

func buildPath(path string, p httprouter.Params) (string, error) {
	pathBytes := make([]byte, 0, len(path)*2)
	for {
		s, rest := nextPathSegment(path)
		if s == "" {
			break
		}
		if s[0] != ':' && s[0] != '*' {
			pathBytes = append(pathBytes, s...)
			path = rest
			continue
		}
		if s[0] == '*' && rest != "" {
			return "", errgo.New("star path parameter is not at end of path")
		}
		if len(s) == 1 {
			return "", errgo.New("empty path parameter")
		}
		val := p.ByName(s[1:])
		if val == "" {
			return "", errgo.Newf("missing value for path parameter %q", s[1:])
		}
		if s[0] == '*' {
			if !strings.HasPrefix(val, "/") {
				return "", errgo.Newf("value %q for path parameter %q does not start with required /", val, s)
			}
			val = val[1:]
		}
		pathBytes = append(pathBytes, val...)
		path = rest
	}
	return string(pathBytes), nil
}

// nextPathSegment returns the next wildcard or constant
// segment of the given path and everything after that
// segment.
func nextPathSegment(s string) (string, string) {
	if s == "" {
		return "", ""
	}
	if s[0] == ':' || s[0] == '*' {
		if i := strings.Index(s, "/"); i != -1 {
			return s[0:i], s[i:]
		}
		return s, ""
	}
	if i := strings.IndexAny(s, ":*"); i != -1 {
		return s[0:i], s[i:]
	}
	return s, ""
}

// getMarshaler returns a marshaler function suitable for marshaling
// a field with the given tag into and http request.
func getMarshaler(tag tag, t reflect.Type) (marshaler, error) {
	switch {
	case tag.source == sourceNone:
		return marshalNop, nil
	case tag.source == sourceBody:
		return marshalBody, nil
	case t == reflect.TypeOf([]string(nil)):
		switch tag.source {
		default:
			return nil, errgo.New("invalid target type []string for path parameter")
		case sourceForm:
			return marshalAllField(tag.name), nil
		case sourceHeader:
			return marshalAllHeader(tag.name), nil
		}
	case t == reflect.TypeOf(""):
		return marshalString(tag), nil
	case implementsTextMarshaler(t):
		return marshalWithMarshalText(t, tag), nil
	default:
		return marshalWithSprint(tag), nil
	}
}

// marshalNop does nothing with the value.
func marshalNop(v reflect.Value, p *Params) error {
	return nil
}

// mashalBody marshals the specified value into the body of the http request.
func marshalBody(v reflect.Value, p *Params) error {
	// TODO allow body types that aren't necessarily JSON.
	data, err := json.Marshal(v.Addr().Interface())
	if err != nil {
		return errgo.Notef(err, "cannot marshal request body")
	}
	p.Request.Body = BytesReaderCloser{bytes.NewReader(data)}
	p.Request.ContentLength = int64(len(data))
	p.Request.Header.Set("Content-Type", "application/json")
	return nil
}

// marshalAllField marshals a []string slice into form fields.
func marshalAllField(name string) marshaler {
	return func(v reflect.Value, p *Params) error {
		p.Request.Form[name] = v.Interface().([]string)
		return nil
	}
}

// marshalAllHeader marshals a []string slice into a header.
func marshalAllHeader(name string) marshaler {
	return func(v reflect.Value, p *Params) error {
		p.Request.Header[name] = v.Interface().([]string)
		return nil
	}
}

// marshalString marshals s string field.
func marshalString(tag tag) marshaler {
	formSet := formSetters[tag.source]
	if formSet == nil {
		panic("unexpected source")
	}
	return func(v reflect.Value, p *Params) error {
		formSet(tag.name, v.String(), p)
		return nil
	}
}

// encodingTextMarshaler is the same as encoding.TextUnmarshaler
// but avoids us importing the encoding package, which some
// broken gccgo installations do not allow.
// TODO remove this and use encoding.TextMarshaler instead.
type encodingTextMarshaler interface {
	MarshalText() (text []byte, err error)
}

var textMarshalerType = reflect.TypeOf((*encodingTextMarshaler)(nil)).Elem()

func implementsTextMarshaler(t reflect.Type) bool {
	// Use the pointer type, because a pointer
	// type will implement a superset of the methods
	// of a non-pointer type.
	return reflect.PtrTo(t).Implements(textMarshalerType)
}

// marshalWithMarshalText returns a marshaler
// that marshals the given type from the given tag
// using its MarshalText method.
func marshalWithMarshalText(t reflect.Type, tag tag) marshaler {
	formSet := formSetters[tag.source]
	if formSet == nil {
		panic("unexpected source")
	}
	return func(v reflect.Value, p *Params) error {
		m := v.Addr().Interface().(encodingTextMarshaler)
		data, err := m.MarshalText()
		if err != nil {
			return errgo.Mask(err)
		}
		formSet(tag.name, string(data), p)

		return nil
	}
}

// marshalWithScan returns an marshaler
// that unmarshals the given tag using fmt.Sprint.
func marshalWithSprint(tag tag) marshaler {
	formSet := formSetters[tag.source]
	if formSet == nil {
		panic("unexpected source")
	}
	return func(v reflect.Value, p *Params) error {
		valueString := fmt.Sprint(v.Interface())

		formSet(tag.name, valueString, p)

		return nil
	}
}

// formSetters maps from source to a function that
// sets the value for a given key.
var formSetters = []func(string, string, *Params){
	sourceForm: func(name, value string, p *Params) {
		p.Request.Form.Set(name, value)
	},
	sourcePath: func(name, value string, p *Params) {
		p.PathVar = append(p.PathVar, httprouter.Param{Key: name, Value: value})
	},
	sourceBody: nil,
	sourceHeader: func(name, value string, p *Params) {
		p.Request.Header.Set(name, value)
	},
}

// BytesReaderCloser is a bytes.Reader which
// implements io.Closer with a no-op Close method.
type BytesReaderCloser struct {
	*bytes.Reader
}

// Close implements io.Closer.Close.
func (BytesReaderCloser) Close() error {
	return nil
}
