// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers

import (
	"fmt"
	"reflect"

	. "launchpad.net/gocheck"
)

type isTrueChecker struct {
	*CheckerInfo
}

// IsTrue checks whether a value has an underlying
// boolean type and is true.
var IsTrue Checker = &isTrueChecker{
	&CheckerInfo{Name: "IsTrue", Params: []string{"obtained"}},
}

// IsTrue checks whether a value has an underlying
// boolean type and is false.
var IsFalse Checker = Not(IsTrue)

func (checker *isTrueChecker) Check(params []interface{}, names []string) (result bool, error string) {

	value := reflect.ValueOf(params[0])

	switch value.Kind() {
	case reflect.Bool:
		return value.Bool(), ""
	}

	return false, fmt.Sprintf("expected type bool, received type %s", value.Type())
}

type satisfiesChecker struct {
	*CheckerInfo
}

// Satisfies checks whether a value causes the argument
// function to return true. The function must be of
// type func(T) bool where the value being checked
// is assignable to T.
var Satisfies Checker = &satisfiesChecker{
	&CheckerInfo{
		Name:   "Satisfies",
		Params: []string{"obtained", "func(T) bool"},
	},
}

func (checker *satisfiesChecker) Check(params []interface{}, names []string) (result bool, error string) {
	f := reflect.ValueOf(params[1])
	ft := f.Type()
	if ft.Kind() != reflect.Func ||
		ft.NumIn() != 1 ||
		ft.NumOut() != 1 ||
		ft.Out(0) != reflect.TypeOf(true) {
		return false, fmt.Sprintf("expected func(T) bool, got %s", ft)
	}
	v := reflect.ValueOf(params[0])
	if !v.IsValid() {
		if !canBeNil(ft.In(0)) {
			return false, fmt.Sprintf("cannot assign nil to argument %T", ft.In(0))
		}
		v = reflect.Zero(ft.In(0))
	}
	if !v.Type().AssignableTo(ft.In(0)) {
		return false, fmt.Sprintf("wrong argument type %s for %s", v.Type(), ft)
	}
	return f.Call([]reflect.Value{v})[0].Interface().(bool), ""
}

func canBeNil(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Chan,
		reflect.Func,
		reflect.Interface,
		reflect.Map,
		reflect.Ptr,
		reflect.Slice:
		return true
	}
	return false
}
