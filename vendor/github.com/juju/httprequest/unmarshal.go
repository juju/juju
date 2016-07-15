package httprequest

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"

	"gopkg.in/errgo.v1"
)

var (
	ErrUnmarshal        = errgo.New("httprequest unmarshal error")
	ErrBadUnmarshalType = errgo.New("httprequest bad unmarshal type")
)

// Unmarshal takes values from given parameters and fills
// out fields in x, which must be a pointer to a struct.
//
// Tags on the struct's fields determine where each field is filled in
// from. Similar to encoding/json and other encoding packages, the tag
// holds a comma-separated list. The first item in the list is an
// alternative name for the field (the field name itself will be used if
// this is empty). The next item specifies where the field is filled in
// from. It may be:
//
//	"path" - the field is taken from a parameter in p.PathVar
//		with a matching field name.
//
// 	"form" - the field is taken from the given name in p.Request.Form
//		(note that this covers both URL query parameters and
//		POST form parameters).
//
//	"header" - the field is taken from the given name in
//		p.Request.Header.
//
//	"body" - the field is filled in by parsing the request body
//		as JSON.
//
// For path and form parameters, the field will be filled out from
// the field in p.PathVar or p.Form using one of the following
// methods (in descending order of preference):
//
// - if the type is string, it will be set from the first value.
//
// - if the type is []string, it will be filled out using all values for that field
//    (allowed only for form)
//
// - if the type implements encoding.TextUnmarshaler, its
// UnmarshalText method will be used
//
// -  otherwise fmt.Sscan will be used to set the value.
//
// When the unmarshaling fails, Unmarshal returns an error with an
// ErrUnmarshal cause. If the type of x is inappropriate,
// it returns an error with an ErrBadUnmarshalType cause.
func Unmarshal(p Params, x interface{}) error {
	xv := reflect.ValueOf(x)
	pt, err := getRequestType(xv.Type())
	if err != nil {
		return errgo.WithCausef(err, ErrBadUnmarshalType, "bad type %s", xv.Type())
	}
	if err := unmarshal(p, xv, pt); err != nil {
		return errgo.Mask(err, errgo.Is(ErrUnmarshal))
	}
	return nil
}

// unmarshal is the internal version of Unmarshal.
func unmarshal(p Params, xv reflect.Value, pt *requestType) error {
	xv = xv.Elem()
	for _, f := range pt.fields {
		fv := xv.FieldByIndex(f.index)
		// TODO store the field name in the field so
		// that we can produce a nice error message.
		if err := f.unmarshal(fv, p, f.makeResult); err != nil {
			return errgo.WithCausef(err, ErrUnmarshal, "cannot unmarshal into field")
		}
	}
	return nil
}

// getUnmarshaler returns an unmarshaler function
// suitable for unmarshaling a field with the given tag
// into a value of the given type.
func getUnmarshaler(tag tag, t reflect.Type) (unmarshaler, error) {
	switch {
	case tag.source == sourceNone:
		return unmarshalNop, nil
	case tag.source == sourceBody:
		return unmarshalBody, nil
	case t == reflect.TypeOf([]string(nil)):
		switch tag.source {
		default:
			return nil, errgo.New("invalid target type []string for path parameter")
		case sourceForm:
			return unmarshalAllField(tag.name), nil
		case sourceHeader:
			return unmarshalAllHeader(tag.name), nil
		}
	case t == reflect.TypeOf(""):
		return unmarshalString(tag), nil
	case implementsTextUnmarshaler(t):
		return unmarshalWithUnmarshalText(t, tag), nil
	default:
		return unmarshalWithScan(tag), nil
	}
}

// unmarshalNop just creates the result value but does not
// fill it out with anything. This is used to create pointers
// to new anonymous field members.
func unmarshalNop(v reflect.Value, p Params, makeResult resultMaker) error {
	makeResult(v)
	return nil
}

// unmarshalAllField unmarshals all the form fields for a given
// attribute into a []string slice.
func unmarshalAllField(name string) unmarshaler {
	return func(v reflect.Value, p Params, makeResult resultMaker) error {
		vals := p.Request.Form[name]
		if len(vals) > 0 {
			makeResult(v).Set(reflect.ValueOf(vals))
		}
		return nil
	}
}

// unmarshalAllHeader unmarshals all the header fields for a given
// attribute into a []string slice.
func unmarshalAllHeader(name string) unmarshaler {
	return func(v reflect.Value, p Params, makeResult resultMaker) error {
		vals := p.Request.Header[name]
		if len(vals) > 0 {
			makeResult(v).Set(reflect.ValueOf(vals))
		}
		return nil
	}
}

// unmarshalString unmarshals into a string field.
func unmarshalString(tag tag) unmarshaler {
	getVal := formGetters[tag.source]
	if getVal == nil {
		panic("unexpected source")
	}
	return func(v reflect.Value, p Params, makeResult resultMaker) error {
		val, ok := getVal(tag.name, p)
		if ok {
			makeResult(v).SetString(val)
		}
		return nil
	}
}

// unmarshalBody unmarshals the http request body
// into the given value.
func unmarshalBody(v reflect.Value, p Params, makeResult resultMaker) error {
	if err := checkIsJSON(p.Request.Header, p.Request.Body); err != nil {
		return errgo.Mask(err)
	}
	data, err := ioutil.ReadAll(p.Request.Body)
	if err != nil {
		return errgo.Notef(err, "cannot read request body")
	}
	// TODO allow body types that aren't necessarily JSON.
	result := makeResult(v)
	if err := json.Unmarshal(data, result.Addr().Interface()); err != nil {
		return errgo.Notef(err, "cannot unmarshal request body")
	}
	return nil
}

// formGetters maps from source to a function that
// returns the value for a given key and reports
// whether the value was found.
var formGetters = []func(name string, p Params) (string, bool){
	sourceForm: func(name string, p Params) (string, bool) {
		vs := p.Request.Form[name]
		if len(vs) == 0 {
			return "", false
		}
		return vs[0], true
	},
	sourcePath: func(name string, p Params) (string, bool) {
		for _, pv := range p.PathVar {
			if pv.Key == name {
				return pv.Value, true
			}
		}
		return "", false
	},
	sourceBody: nil,
	sourceHeader: func(name string, p Params) (string, bool) {
		vs := p.Request.Header[name]
		if len(vs) == 0 {
			return "", false
		}
		return vs[0], true
	},
}

// encodingTextUnmarshaler is the same as encoding.TextUnmarshaler
// but avoids us importing the encoding package, which some
// broken gccgo installations do not allow.
// TODO remove this and use encoding.TextUnmarshaler instead.
type encodingTextUnmarshaler interface {
	UnmarshalText(text []byte) error
}

var textUnmarshalerType = reflect.TypeOf((*encodingTextUnmarshaler)(nil)).Elem()

func implementsTextUnmarshaler(t reflect.Type) bool {
	// Use the pointer type, because a pointer
	// type will implement a superset of the methods
	// of a non-pointer type.
	return reflect.PtrTo(t).Implements(textUnmarshalerType)
}

// unmarshalWithUnmarshalText returns an unmarshaler
// that unmarshals the given type from the given tag
// using its UnmarshalText method.
func unmarshalWithUnmarshalText(t reflect.Type, tag tag) unmarshaler {
	getVal := formGetters[tag.source]
	if getVal == nil {
		panic("unexpected source")
	}
	return func(v reflect.Value, p Params, makeResult resultMaker) error {
		val, _ := getVal(tag.name, p)
		uv := makeResult(v).Addr().Interface().(encodingTextUnmarshaler)
		return uv.UnmarshalText([]byte(val))
	}
}

// unmarshalWithScan returns an unmarshaler
// that unmarshals the given tag using fmt.Scan.
func unmarshalWithScan(tag tag) unmarshaler {
	formGet := formGetters[tag.source]
	if formGet == nil {
		panic("unexpected source")
	}
	return func(v reflect.Value, p Params, makeResult resultMaker) error {
		val, ok := formGet(tag.name, p)
		if !ok {
			// TODO allow specifying that a field is mandatory?
			return nil
		}
		_, err := fmt.Sscan(val, makeResult(v).Addr().Interface())
		if err != nil {
			return errgo.Notef(err, "cannot parse %q into %s", val, v.Type())
		}
		return nil
	}
}
