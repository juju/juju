// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Package httprequest provides functionality for unmarshaling
// HTTP request parameters into a struct type.
//
// Please note that the API is not considered stable at this
// point and may be changed in a backwardly incompatible
// manner at any time.
package httprequest

import (
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/julienschmidt/httprouter"
	"gopkg.in/errgo.v1"
)

// TODO include field name and source in error messages.

var (
	typeMutex sync.RWMutex
	typeMap   = make(map[reflect.Type]*requestType)
)

// Route is the type of a field that specifies a routing
// path and HTTP method. See Marshal and Unmarshal
// for details.
type Route struct{}

// Params holds the parameters provided to an HTTP request.
type Params struct {
	Response http.ResponseWriter
	Request  *http.Request
	PathVar  httprouter.Params
	// PathPattern holds the path pattern matched by httprouter.
	// It is only set where httprequest has the information;
	// that is where the call was made by ErrorMapper.Handler
	// or ErrorMapper.Handlers.
	PathPattern string
}

// resultMaker is provided to the unmarshal functions.
// When called with the value passed to the unmarshaler,
// it returns the field value to be assigned to,
// creating it if necessary.
type resultMaker func(reflect.Value) reflect.Value

// unmarshaler unmarshals some value from params into
// the given value. The value should not be assigned to directly,
// but passed to makeResult and then updated.
type unmarshaler func(v reflect.Value, p Params, makeResult resultMaker) error

// marshaler marshals the specified value into params.
// The value is always the value type, even if the field type
// is a pointer.
type marshaler func(reflect.Value, *Params) error

// requestType holds information derived from a request
// type, preprocessed so that it's quick to marshal or unmarshal.
type requestType struct {
	method string
	path   string
	fields []field
}

// field holds preprocessed information on an individual field
// in the request.
type field struct {
	// index holds the index slice of the field.
	index []int

	// unmarshal is used to unmarshal the value into
	// the given field. The value passed as its first
	// argument is not a pointer type, but is addressable.
	unmarshal unmarshaler

	// marshal is used to marshal the value into the
	// give filed. The value passed as its first argument is not
	// a pointer type, but it is addressable.
	marshal marshaler

	// makeResult is the resultMaker that will be
	// passed into the unmarshaler.
	makeResult resultMaker

	// isPointer is true if the field is pointer to the underlying type.
	isPointer bool
}

// getRequestType is like parseRequestType except that
// it returns the cached requestType when possible,
// adding the type to the cache otherwise.
func getRequestType(t reflect.Type) (*requestType, error) {
	typeMutex.RLock()
	pt := typeMap[t]
	typeMutex.RUnlock()
	if pt != nil {
		return pt, nil
	}
	typeMutex.Lock()
	defer typeMutex.Unlock()
	if pt = typeMap[t]; pt != nil {
		// The type has been parsed after we dropped
		// the read lock, so use it.
		return pt, nil
	}
	pt, err := parseRequestType(t)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	typeMap[t] = pt
	return pt, nil
}

// parseRequestType preprocesses the given type
// into a form that can be efficiently interpreted
// by Unmarshal.
func parseRequestType(t reflect.Type) (*requestType, error) {
	if t.Kind() != reflect.Ptr || t.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("type is not pointer to struct")
	}

	hasBody := false
	var pt requestType
	foundRoute := false
	for _, f := range fields(t.Elem()) {
		if f.PkgPath != "" && !f.Anonymous {
			// Ignore non-anonymous unexported fields.
			continue
		}
		if !foundRoute && f.Anonymous && f.Type == reflect.TypeOf(Route{}) {
			var err error
			pt.method, pt.path, err = parseRouteTag(f.Tag)
			if err != nil {
				return nil, errgo.Notef(err, "bad route tag %q", f.Tag)
			}
			foundRoute = true
			continue
		}
		tag, err := parseTag(f.Tag, f.Name)
		if err != nil {
			return nil, errgo.Notef(err, "bad tag %q in field %s", f.Tag, f.Name)
		}
		if tag.source == sourceBody {
			if hasBody {
				return nil, errgo.New("more than one body field specified")
			}
			hasBody = true
		}
		field := field{
			index: f.Index,
		}
		if f.Type.Kind() == reflect.Ptr {
			// The field is a pointer, so when the value is set,
			// we need to create a new pointer to put
			// it into.
			field.makeResult = makePointerResult
			field.isPointer = true
			f.Type = f.Type.Elem()
		} else {
			field.makeResult = makeValueResult
			field.isPointer = false
		}

		field.unmarshal, err = getUnmarshaler(tag, f.Type)
		if err != nil {
			return nil, errgo.Mask(err)
		}

		field.marshal, err = getMarshaler(tag, f.Type)
		if err != nil {
			return nil, errgo.Mask(err)
		}

		if f.Anonymous {
			if tag.source != sourceBody && tag.source != sourceNone {
				return nil, errgo.New("httprequest tag not yet supported on anonymous fields")
			}
		}
		pt.fields = append(pt.fields, field)
	}
	return &pt, nil
}

// Note: we deliberately omit HEAD and OPTIONS
// from this list. HEAD will be routed through GET handlers
// and OPTIONS is handled separately.
var validMethod = map[string]bool{
	"PUT":    true,
	"POST":   true,
	"DELETE": true,
	"GET":    true,
	"PATCH":  true,
}

func parseRouteTag(tag reflect.StructTag) (method, path string, err error) {
	tagStr := tag.Get("httprequest")
	if tagStr == "" {
		return "", "", errgo.New("no httprequest tag")
	}
	f := strings.Fields(tagStr)
	switch len(f) {
	case 2:
		path = f[1]
		fallthrough
	case 1:
		method = f[0]
	default:
		return "", "", errgo.New("wrong field count")
	}
	if !validMethod[method] {
		return "", "", errgo.Newf("invalid method")
	}
	// TODO check that path looks valid
	return method, path, nil
}

func makePointerResult(v reflect.Value) reflect.Value {
	if v.IsNil() {
		v.Set(reflect.New(v.Type().Elem()))
	}
	return v.Elem()
}

func makeValueResult(v reflect.Value) reflect.Value {
	return v
}

type tagSource uint8

const (
	sourceNone = iota
	sourcePath
	sourceForm
	sourceBody
	sourceHeader
)

type tag struct {
	name   string
	source tagSource
}

// parseTag parses the given struct tag attached to the given
// field name into a tag structure.
func parseTag(rtag reflect.StructTag, fieldName string) (tag, error) {
	t := tag{
		name: fieldName,
	}
	tagStr := rtag.Get("httprequest")
	if tagStr == "" {
		return t, nil
	}
	fields := strings.Split(tagStr, ",")
	if fields[0] != "" {
		t.name = fields[0]
	}
	for _, f := range fields[1:] {
		switch f {
		case "path":
			t.source = sourcePath
		case "form":
			t.source = sourceForm
		case "body":
			t.source = sourceBody
		case "header":
			t.source = sourceHeader
		default:
			return tag{}, fmt.Errorf("unknown tag flag %q", f)
		}
	}
	return t, nil
}

// fields returns all the fields in the given struct type
// including fields inside anonymous struct members.
// The fields are ordered with top level fields first
// followed by the members of those fields
// for anonymous fields.
func fields(t reflect.Type) []reflect.StructField {
	byName := make(map[string]reflect.StructField)
	addFields(t, byName, nil)
	fields := make(fieldsByIndex, 0, len(byName))
	for _, f := range byName {
		if f.Name != "" {
			fields = append(fields, f)
		}
	}
	sort.Sort(fields)
	return fields
}

func addFields(t reflect.Type, byName map[string]reflect.StructField, index []int) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		index := append(index, i)
		var add bool
		old, ok := byName[f.Name]
		switch {
		case ok && len(old.Index) == len(index):
			// Fields with the same name at the same depth
			// cancel one another out. Set the field name
			// to empty to signify that has happened.
			old.Name = ""
			byName[f.Name] = old
			add = false
		case ok:
			// Fields at less depth win.
			add = len(index) < len(old.Index)
		default:
			// The field did not previously exist.
			add = true
		}
		if add {
			// copy the index so that it's not overwritten
			// by the other appends.
			f.Index = append([]int(nil), index...)
			byName[f.Name] = f
		}
		if f.Anonymous {
			if f.Type.Kind() == reflect.Ptr {
				f.Type = f.Type.Elem()
			}
			if f.Type.Kind() == reflect.Struct {
				addFields(f.Type, byName, index)
			}
		}
	}
}

type fieldsByIndex []reflect.StructField

func (f fieldsByIndex) Len() int {
	return len(f)
}

func (f fieldsByIndex) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}

func (f fieldsByIndex) Less(i, j int) bool {
	indexi, indexj := f[i].Index, f[j].Index
	for len(indexi) != 0 && len(indexj) != 0 {
		ii, ij := indexi[0], indexj[0]
		if ii != ij {
			return ii < ij
		}
		indexi, indexj = indexi[1:], indexj[1:]
	}
	return len(indexi) < len(indexj)
}
