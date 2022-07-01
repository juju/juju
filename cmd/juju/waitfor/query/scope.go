// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/juju/errors"
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
				fmt.Printf("%+v\n", v)
				return v, nil
			},
			"forEach": func(values, expr interface{}) (interface{}, error) {
				scopes, ok := values.(Box)
				if !ok {
					return nil, RuntimeErrorf("unexpected lambda values %T", values)
				}
				lambda, ok := expr.(*BoxLambda)
				if !ok {
					return nil, RuntimeErrorf("unexpected lambda %T", expr)
				}

				var (
					err    error
					called bool
					result = true
				)
				ForEach(scopes, func(value interface{}) bool {
					called = true

					nestedScope, ok := value.(Scope)
					if !ok {
						err = RuntimeErrorf("unexpected scope type %T", value)
						return false
					}

					namedScope := MakeNestedScope(scope)
					namedScope.SetScope(lambda.ArgName(), nestedScope)

					var results []Box
					results, err = lambda.Call(namedScope)
					if err != nil {
						return false
					}
					var lambdaResult bool
					for _, result := range results {
						lambdaResult = !result.IsZero()
					}
					result = result && lambdaResult
					return result
				})
				if err != nil {
					return nil, errors.Trace(err)
				}
				if !called {
					return false, nil
				}
				return result, nil
			},
		},
	}
}

// Add a function to the global scope.
func (s *GlobalFuncScope) Add(name string, fn interface{}) {
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
		return nil, RuntimeErrorf("number of params not valid for function call, wanted: %d, received: %d", f.Type().NumIn(), len(params))
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

// NestedScope allows scopes to be nested together in a named manor.
type NestedScope struct {
	base   Scope
	scopes map[string]Scope
}

// MakeNestedScope creates a new NestedScope.
func MakeNestedScope(base Scope) NestedScope {
	return NestedScope{
		base:   base,
		scopes: make(map[string]Scope),
	}
}

// GetIdents returns the identifiers with in a given scope.
func (m NestedScope) GetIdents() []string {
	return m.base.GetIdents()
}

// GetIdentValue returns the value of the identifier in a given scope.
func (m NestedScope) GetIdentValue(name string) (Box, error) {
	parts := strings.Split(name, ".")
	scope, ok := m.scopes[parts[0]]
	if !ok {
		return m.base.GetIdentValue(name)
	}
	if len(parts) != 2 {
		return &BoxNestedScope{
			value: scope,
		}, nil
	}
	return scope.GetIdentValue(parts[1])
}

// SetScope will set a scope on a given scope.
func (m NestedScope) SetScope(name string, scope Scope) {
	m.scopes[name] = scope
}

// BoxNestedScope defines an ordered integer.
type BoxNestedScope struct {
	value Scope
}

// Less checks if a BoxNestedScope is less than another BoxNestedScope.
func (o *BoxNestedScope) Less(other Ord) bool {
	return false
}

// Equal checks if an BoxNestedScope is equal to another BoxNestedScope.
func (o *BoxNestedScope) Equal(other Ord) bool {
	return false
}

// IsZero returns if the underlying value is zero.
func (o *BoxNestedScope) IsZero() bool {
	return false
}

// Value defines the shadow type value of the Box.
func (o *BoxNestedScope) Value() interface{} {
	return o.value
}

// ForEach will call the function on every value within a Box.
// If a Box isn't an iterable then we perform a no-op.
func ForEach(box Box, fn func(value interface{}) bool) {
	type iterable interface {
		ForEach(func(interface{}) bool)
	}
	if e, ok := box.(iterable); ok {
		e.ForEach(fn)
	}
}
