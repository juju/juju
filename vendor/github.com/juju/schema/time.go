// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package schema

import (
	"reflect"
	"time"
)

// Time returns a Checker that accepts a string value, and returns
// the parsed time.Time value. Emtpy strings are considered empty times.
func Time() Checker {
	return timeC{}
}

type timeC struct{}

// Coerce implements Checker Coerce method.
func (c timeC) Coerce(v interface{}, path []string) (interface{}, error) {
	if v == nil {
		return nil, error_{"string or time.Time", v, path}
	}
	var empty time.Time
	switch reflect.TypeOf(v).Kind() {
	case reflect.TypeOf(empty).Kind():
		return v, nil
	case reflect.String:
		vstr := reflect.ValueOf(v).String()
		if vstr == "" {
			return empty, nil
		}
		v, err := time.Parse(time.RFC3339Nano, vstr)
		if err != nil {
			return nil, err
		}
		return v, nil
	default:
		return nil, error_{"string or time.Time", v, path}
	}
}
