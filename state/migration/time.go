// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"reflect"
	"time"

	"github.com/juju/errors"
	"github.com/juju/schema"
)

// SchemaTime returns a Checker that accepts a string value, and returns
// the parsed time.Time value. Emtpy strings are considered empty times.
func SchemaTime() schema.Checker {
	return timeC{}
}

type timeC struct{}

// Coerce implements schema.Checker Coerce method.
func (c timeC) Coerce(v interface{}, path []string) (interface{}, error) {
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
	}
	return nil, errors.Errorf("%sexpected string, got %T(%#v)", path, v, v)
}
