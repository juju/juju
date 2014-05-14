// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// The Coerce method of the Checker interface is called recursively when
// v is being validated.  If err is nil, newv is used as the new value
// at the recursion point.  If err is non-nil, v is taken as invalid and
// may be either ignored or error out depending on where in the schema
// checking process the error happened. Checkers like OneOf may continue
// with an alternative, for instance.
type Checker interface {
	Coerce(v interface{}, path []string) (newv interface{}, err error)
}

type error_ struct {
	want string
	got  interface{}
	path []string
}

// pathAsString returns a string consisting of the path elements. If path
// starts with a ".", the dot is omitted.
func pathAsString(path []string) string {
	if path[0] == "." {
		return strings.Join(path[1:], "")
	} else {
		return strings.Join(path, "")
	}
}

func (e error_) Error() string {
	path := pathAsString(e.path)
	if e.want == "" {
		return fmt.Sprintf("%s: unexpected value %#v", path, e.got)
	}
	if e.got == nil {
		return fmt.Sprintf("%s: expected %s, got nothing", path, e.want)
	}
	return fmt.Sprintf("%s: expected %s, got %T(%#v)", path, e.want, e.got, e.got)
}

// Any returns a Checker that succeeds with any input value and
// results in the value itself unprocessed.
func Any() Checker {
	return anyC{}
}

type anyC struct{}

func (c anyC) Coerce(v interface{}, path []string) (interface{}, error) {
	return v, nil
}

// Const returns a Checker that only succeeds if the input matches
// value exactly.  The value is compared with reflect.DeepEqual.
func Const(value interface{}) Checker {
	return constC{value}
}

type constC struct {
	value interface{}
}

func (c constC) Coerce(v interface{}, path []string) (interface{}, error) {
	if reflect.DeepEqual(v, c.value) {
		return v, nil
	}
	return nil, error_{fmt.Sprintf("%#v", c.value), v, path}
}

// OneOf returns a Checker that attempts to Coerce the value with each
// of the provided checkers. The value returned by the first checker
// that succeeds will be returned by the OneOf checker itself.  If no
// checker succeeds, OneOf will return an error on coercion.
func OneOf(options ...Checker) Checker {
	return oneOfC{options}
}

type oneOfC struct {
	options []Checker
}

func (c oneOfC) Coerce(v interface{}, path []string) (interface{}, error) {
	for _, o := range c.options {
		newv, err := o.Coerce(v, path)
		if err == nil {
			return newv, nil
		}
	}
	return nil, error_{"", v, path}
}

// Bool returns a Checker that accepts boolean values only.
func Bool() Checker {
	return boolC{}
}

type boolC struct{}

func (c boolC) Coerce(v interface{}, path []string) (interface{}, error) {
	if v != nil {
		switch reflect.TypeOf(v).Kind() {
		case reflect.Bool:
			return v, nil
		case reflect.String:
			val, err := strconv.ParseBool(reflect.ValueOf(v).String())
			if err == nil {
				return val, nil
			}
		}
	}
	return nil, error_{"bool", v, path}
}

// Int returns a Checker that accepts any integer value, and returns
// the same value consistently typed as an int64.
func Int() Checker {
	return intC{}
}

type intC struct{}

func (c intC) Coerce(v interface{}, path []string) (interface{}, error) {
	if v == nil {
		return nil, error_{"int", v, path}
	}
	switch reflect.TypeOf(v).Kind() {
	case reflect.Int:
	case reflect.Int8:
	case reflect.Int16:
	case reflect.Int32:
	case reflect.Int64:
	case reflect.String:
		val, err := strconv.ParseInt(reflect.ValueOf(v).String(), 0, 64)
		if err == nil {
			return val, nil
		} else {
			return nil, error_{"int", v, path}
		}
	default:
		return nil, error_{"int", v, path}
	}
	return reflect.ValueOf(v).Int(), nil
}

// ForceInt returns a Checker that accepts any integer or float value, and
// returns the same value consistently typed as an int. This is required
// in order to handle the interface{}/float64 type conversion performed by
// the JSON serializer used as part of the API infrastructure.
func ForceInt() Checker {
	return forceIntC{}
}

type forceIntC struct{}

func (c forceIntC) Coerce(v interface{}, path []string) (interface{}, error) {
	if v != nil {
		switch vv := reflect.TypeOf(v); vv.Kind() {
		case reflect.String:
			vstr := reflect.ValueOf(v).String()
			intValue, err := strconv.ParseInt(vstr, 0, 64)
			if err == nil {
				return int(intValue), nil
			}
			floatValue, err := strconv.ParseFloat(vstr, 64)
			if err == nil {
				return int(floatValue), nil
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return int(reflect.ValueOf(v).Int()), nil
		case reflect.Float32, reflect.Float64:
			return int(reflect.ValueOf(v).Float()), nil
		}
	}
	return nil, error_{"number", v, path}
}

// Float returns a Checker that accepts any float value, and returns
// the same value consistently typed as a float64.
func Float() Checker {
	return floatC{}
}

type floatC struct{}

func (c floatC) Coerce(v interface{}, path []string) (interface{}, error) {
	if v == nil {
		return nil, error_{"float", v, path}
	}
	switch reflect.TypeOf(v).Kind() {
	case reflect.Float32:
	case reflect.Float64:
	default:
		return nil, error_{"float", v, path}
	}
	return reflect.ValueOf(v).Float(), nil
}

// String returns a Checker that accepts a string value only and returns
// it unprocessed.
func String() Checker {
	return stringC{}
}

type stringC struct{}

func (c stringC) Coerce(v interface{}, path []string) (interface{}, error) {
	if v != nil && reflect.TypeOf(v).Kind() == reflect.String {
		return reflect.ValueOf(v).String(), nil
	}
	return nil, error_{"string", v, path}
}

func SimpleRegexp() Checker {
	return sregexpC{}
}

type sregexpC struct{}

func (c sregexpC) Coerce(v interface{}, path []string) (interface{}, error) {
	// XXX The regexp package happens to be extremely simple right now.
	//     Once exp/regexp goes mainstream, we'll have to update this
	//     logic to use a more widely accepted regexp subset.
	if v != nil && reflect.TypeOf(v).Kind() == reflect.String {
		s := reflect.ValueOf(v).String()
		_, err := regexp.Compile(s)
		if err != nil {
			return nil, error_{"valid regexp", s, path}
		}
		return v, nil
	}
	return nil, error_{"regexp string", v, path}
}

// List returns a Checker that accepts a slice value with values
// that are processed with the elem checker.  If any element of the
// provided slice value fails to be processed, processing will stop
// and return with the obtained error.
//
// The coerced output value has type []interface{}.
func List(elem Checker) Checker {
	return listC{elem}
}

type listC struct {
	elem Checker
}

func (c listC) Coerce(v interface{}, path []string) (interface{}, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil, error_{"list", v, path}
	}

	path = append(path, "[", "?", "]")

	l := rv.Len()
	out := make([]interface{}, 0, l)
	for i := 0; i != l; i++ {
		path[len(path)-2] = strconv.Itoa(i)
		elem, err := c.elem.Coerce(rv.Index(i).Interface(), path)
		if err != nil {
			return nil, err
		}
		out = append(out, elem)
	}
	return out, nil
}

// Map returns a Checker that accepts a map value. Every key and value
// in the map are processed with the respective checker, and if any
// value fails to be coerced, processing stops and returns with the
// underlying error.
//
// The coerced output value has type map[interface{}]interface{}.
func Map(key Checker, value Checker) Checker {
	return mapC{key, value}
}

type mapC struct {
	key   Checker
	value Checker
}

func (c mapC) Coerce(v interface{}, path []string) (interface{}, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return nil, error_{"map", v, path}
	}

	vpath := append(path, ".", "?")

	l := rv.Len()
	out := make(map[interface{}]interface{}, l)
	keys := rv.MapKeys()
	for i := 0; i != l; i++ {
		k := keys[i]
		newk, err := c.key.Coerce(k.Interface(), path)
		if err != nil {
			return nil, err
		}
		vpath[len(vpath)-1] = fmt.Sprint(k.Interface())
		newv, err := c.value.Coerce(rv.MapIndex(k).Interface(), vpath)
		if err != nil {
			return nil, err
		}
		out[newk] = newv
	}
	return out, nil
}

// StringMap returns a Checker that accepts a map value. Every key in
// the map must be a string, and every value in the map are processed
// with the provided checker. If any value fails to be coerced,
// processing stops and returns with the underlying error.
//
// The coerced output value has type map[string]interface{}.
func StringMap(value Checker) Checker {
	return stringMapC{value}
}

type stringMapC struct {
	value Checker
}

func (c stringMapC) Coerce(v interface{}, path []string) (interface{}, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return nil, error_{"map", v, path}
	}

	vpath := append(path, ".", "?")
	key := String()

	l := rv.Len()
	out := make(map[string]interface{}, l)
	keys := rv.MapKeys()
	for i := 0; i != l; i++ {
		k := keys[i]
		newk, err := key.Coerce(k.Interface(), path)
		if err != nil {
			return nil, err
		}
		vpath[len(vpath)-1] = fmt.Sprint(k.Interface())
		newv, err := c.value.Coerce(rv.MapIndex(k).Interface(), vpath)
		if err != nil {
			return nil, err
		}
		out[newk.(string)] = newv
	}
	return out, nil
}

// Omit is a marker for FieldMap and StructFieldMap defaults parameter.
// If a field is not present in the map and defaults to Omit, the missing
// field will be ommitted from the coerced map as well.
var Omit omit

type omit struct{}

type Fields map[string]Checker
type Defaults map[string]interface{}

// FieldMap returns a Checker that accepts a map value with defined
// string keys. Every key has an independent checker associated,
// and processing will only succeed if all the values succeed
// individually. If a field fails to be processed, processing stops
// and returns with the underlying error.
//
// Fields in defaults will be set to the provided value if not present
// in the coerced map. If the default value is schema.Omit, the
// missing field will be omitted from the coerced map.
//
// The coerced output value has type map[string]interface{}.
func FieldMap(fields Fields, defaults Defaults) Checker {
	return fieldMapC{fields, defaults, false}
}

// StrictFieldMap returns a Checker that acts as the one returned by FieldMap,
// but the Checker returns an error if it encounters an unknown key.
func StrictFieldMap(fields Fields, defaults Defaults) Checker {
	return fieldMapC{fields, defaults, true}
}

type fieldMapC struct {
	fields   Fields
	defaults Defaults
	strict   bool
}

func (c fieldMapC) Coerce(v interface{}, path []string) (interface{}, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return nil, error_{"map", v, path}
	}

	if c.strict {
		for _, k := range rv.MapKeys() {
			ks := k.String()
			if _, found := c.fields[ks]; !found {
				value := interface{}("invalid")
				valuev := rv.MapIndex(k)
				if valuev.IsValid() {
					value = valuev.Interface()
				}
				return nil, fmt.Errorf("%v: unknown key %q (value %q)", pathAsString(path), ks, value)
			}
		}
	}

	vpath := append(path, ".", "?")

	l := rv.Len()
	out := make(map[string]interface{}, l)
	for k, checker := range c.fields {
		valuev := rv.MapIndex(reflect.ValueOf(k))
		var value interface{}
		if valuev.IsValid() {
			value = valuev.Interface()
		} else if dflt, ok := c.defaults[k]; ok {
			if dflt == Omit {
				continue
			}
			value = dflt
		}
		vpath[len(vpath)-1] = k
		newv, err := checker.Coerce(value, vpath)
		if err != nil {
			return nil, err
		}
		out[k] = newv
	}
	for k, v := range c.defaults {
		if v == Omit {
			continue
		}
		if _, ok := out[k]; !ok {
			checker, ok := c.fields[k]
			if !ok {
				return nil, fmt.Errorf("got default value for unknown field %q", k)
			}
			vpath[len(vpath)-1] = k
			newv, err := checker.Coerce(v, vpath)
			if err != nil {
				return nil, err
			}
			out[k] = newv
		}
	}
	return out, nil
}

// FieldMapSet returns a Checker that accepts a map value checked
// against one of several FieldMap checkers.  The actual checker
// used is the first one whose checker associated with the selector
// field processes the map correctly. If no checker processes
// the selector value correctly, an error is returned.
//
// The coerced output value has type map[string]interface{}.
func FieldMapSet(selector string, maps []Checker) Checker {
	fmaps := make([]fieldMapC, len(maps))
	for i, m := range maps {
		if fmap, ok := m.(fieldMapC); ok {
			if checker, _ := fmap.fields[selector]; checker == nil {
				panic("FieldMapSet has a FieldMap with a missing selector")
			}
			fmaps[i] = fmap
		} else {
			panic("FieldMapSet got a non-FieldMap checker")
		}
	}
	return mapSetC{selector, fmaps}
}

type mapSetC struct {
	selector string
	fmaps    []fieldMapC
}

func (c mapSetC) Coerce(v interface{}, path []string) (interface{}, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return nil, error_{"map", v, path}
	}

	var selector interface{}
	selectorv := rv.MapIndex(reflect.ValueOf(c.selector))
	if selectorv.IsValid() {
		selector = selectorv.Interface()
		for _, fmap := range c.fmaps {
			_, err := fmap.fields[c.selector].Coerce(selector, path)
			if err != nil {
				continue
			}
			return fmap.Coerce(v, path)
		}
	}
	return nil, error_{"supported selector", selector, append(path, ".", c.selector)}
}
