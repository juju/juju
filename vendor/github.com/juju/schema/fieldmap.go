// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package schema

import (
	"fmt"
	"reflect"
)

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

var stringType = reflect.TypeOf("")

func hasStrictStringKeys(rv reflect.Value) bool {
	if rv.Type().Key() == stringType {
		return true
	}
	if rv.Type().Key().Kind() != reflect.Interface {
		return false
	}
	for _, k := range rv.MapKeys() {
		if k.Elem().Type() != stringType {
			return false
		}
	}
	return true
}

func (c fieldMapC) Coerce(v interface{}, path []string) (interface{}, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return nil, error_{"map", v, path}
	}
	if !hasStrictStringKeys(rv) {
		return nil, error_{"map[string]", v, path}
	}

	if c.strict {
		for _, k := range rv.MapKeys() {
			ks := k.String()
			if _, ok := c.fields[ks]; !ok {
				return nil, fmt.Errorf("%sunknown key %q (value %#v)", pathAsPrefix(path), ks, rv.MapIndex(k).Interface())
			}
		}
	}

	vpath := append(path, ".", "?")

	out := make(map[string]interface{}, rv.Len())
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
