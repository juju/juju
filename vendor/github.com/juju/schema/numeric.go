// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package schema

import (
	"reflect"
	"strconv"
)

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

// Uint returns a Checker that accepts any integer or unsigned value, and
// returns the same value consistently typed as an uint64. If the integer
// value is negative an error is raised.
func Uint() Checker {
	return uintC{}
}

type uintC struct{}

func (c uintC) Coerce(v interface{}, path []string) (interface{}, error) {
	if v == nil {
		return nil, error_{"uint", v, path}
	}
	switch reflect.TypeOf(v).Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return reflect.ValueOf(v).Uint(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		val := reflect.ValueOf(v).Int()
		if val < 0 {
			return nil, error_{"uint", v, path}
		}
		// All positive int64 values fit into uint64.
		return uint64(val), nil
	case reflect.String:
		val, err := strconv.ParseUint(reflect.ValueOf(v).String(), 0, 64)
		if err == nil {
			return val, nil
		} else {
			return nil, error_{"uint", v, path}
		}
	default:
		return nil, error_{"uint", v, path}
	}
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

// ForceUint returns a Checker that accepts any integer or float value, and
// returns the same value consistently typed as an uint64. This is required
// in order to handle the interface{}/float64 type conversion performed by
// the JSON serializer used as part of the API infrastructure. If the integer
// value is negative an error is raised.
func ForceUint() Checker {
	return forceUintC{}
}

type forceUintC struct{}

func (c forceUintC) Coerce(v interface{}, path []string) (interface{}, error) {
	if v != nil {
		switch vv := reflect.TypeOf(v); vv.Kind() {
		case reflect.String:
			vstr := reflect.ValueOf(v).String()
			intValue, err := strconv.ParseUint(vstr, 0, 64)
			if err == nil {
				return intValue, nil
			}
			floatValue, err := strconv.ParseFloat(vstr, 64)
			if err == nil {
				if floatValue < 0 {
					return nil, error_{"uint", v, path}
				}
				return uint64(floatValue), nil
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return reflect.ValueOf(v).Uint(), nil
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val := reflect.ValueOf(v).Int()
			if val < 0 {
				return nil, error_{"uint", v, path}
			}
			// All positive int64 values fit into uint64.
			return uint64(val), nil
		case reflect.Float32, reflect.Float64:
			val := reflect.ValueOf(v).Float()
			if val < 0 {
				return nil, error_{"uint", v, path}
			}
			return uint64(val), nil
		}
	}
	return nil, error_{"uint", v, path}
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
