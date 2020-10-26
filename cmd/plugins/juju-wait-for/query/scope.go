// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"fmt"
	"reflect"
)

// GlobalFuncScope defines a set of builtin functions that can be executed based
// on a set of arguments.
type GlobalFuncScope struct {
	scope Scope
	funcs map[string]interface{}
}

// NewGlobalFuncScope creates a new scope for executing functions.
func NewGlobalFuncScope(scope Scope) *GlobalFuncScope {
	return &GlobalFuncScope{
		scope: scope,
		funcs: map[string]interface{}{
			"len": func(v interface{}) (int, error) {
				val := reflect.ValueOf(v)
				switch val.Kind() {
				case reflect.Map, reflect.Slice, reflect.String:
					return val.Len(), nil
				default:
					return -1, RuntimeErrorf("unexpected type %T passed to len", v)
				}
			},
			"print": func(v interface{}) (interface{}, error) {
				fmt.Printf("%v\n", v)
				return v, nil
			},
		},
	}
}

// Add a function to the global scope.
func (s *GlobalFuncScope) Add(name string, fn func(interface{}) (interface{}, error)) {
	s.funcs[name] = fn
}

// Call a function with a set of arguments.
func (s *GlobalFuncScope) Call(ident *Identifier, params []Box) (interface{}, error) {
	name := ident.Token.Literal
	fn, ok := s.funcs[name]
	if !ok {
		return nil, RuntimeErrorf("no function %q", name)
	}

	f := reflect.ValueOf(fn)
	if len(params) != f.Type().NumIn() {
		return nil, RuntimeErrorf("number of params not valid for function call")
	}
	if f.Type().NumOut() != 2 {
		return nil, RuntimeErrorf("number of results is not value for function call")
	}

	var args []reflect.Value
	for _, arg := range params {
		args = append(args, reflect.ValueOf(arg.Value()))
	}

	results := f.Call(args)
	if len(results) != 2 {
		return nil, RuntimeErrorf("number of results does not match expected function call results set")
	}
	if err, ok := results[1].Interface().(error); ok && err != nil {
		return nil, err
	}
	return results[0].Interface(), nil
}
