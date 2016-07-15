// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package schema

import (
	"fmt"
	"reflect"
)

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
