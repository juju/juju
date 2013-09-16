// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers

import (
	"fmt"
	"reflect"

	gc "launchpad.net/gocheck"
)

// GreaterThan checker

type greaterThanChecker struct {
	*gc.CheckerInfo
}

var GreaterThan gc.Checker = &greaterThanChecker{
	&gc.CheckerInfo{Name: "GreaterThan", Params: []string{"obtained", "expected"}},
}

func (checker *greaterThanChecker) Check(params []interface{}, names []string) (result bool, error string) {
	defer func() {
		if v := recover(); v != nil {
			result = false
			error = fmt.Sprint(v)
		}
	}()

	p0value := reflect.ValueOf(params[0])
	p1value := reflect.ValueOf(params[1])
	switch p0value.Kind() {
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64:
		return p0value.Int() > p1value.Int(), ""
	case reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64:
		return p0value.Uint() > p1value.Uint(), ""
	case reflect.Float32,
		reflect.Float64:
		return p0value.Float() > p1value.Float(), ""
	default:
	}
	return false, fmt.Sprintf("obtained value %s:%#v not supported", p0value.Kind(), params[0])
}

// LessThan checker

type lessThanChecker struct {
	*gc.CheckerInfo
}

var LessThan gc.Checker = &lessThanChecker{
	&gc.CheckerInfo{Name: "LessThan", Params: []string{"obtained", "expected"}},
}

func (checker *lessThanChecker) Check(params []interface{}, names []string) (result bool, error string) {
	defer func() {
		if v := recover(); v != nil {
			result = false
			error = fmt.Sprint(v)
		}
	}()

	p0value := reflect.ValueOf(params[0])
	p1value := reflect.ValueOf(params[1])
	switch p0value.Kind() {
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64:
		return p0value.Int() < p1value.Int(), ""
	case reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64:
		return p0value.Uint() < p1value.Uint(), ""
	case reflect.Float32,
		reflect.Float64:
		return p0value.Float() < p1value.Float(), ""
	default:
	}
	return false, fmt.Sprintf("obtained value %s:%#v not supported", p0value.Kind(), params[0])
}
