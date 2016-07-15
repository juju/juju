// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package schema

import (
	"fmt"
	"reflect"
	"regexp"
)

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

// SimpleRegexp returns a checker that accepts a string value that is
// a valid regular expression and returns it unprocessed.
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

// UUID returns a Checker that accepts a string value only and returns
// it unprocessed.
func UUID() Checker {
	return uuidC{}
}

type uuidC struct{}

var uuidregex = regexp.MustCompile(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`)

func (c uuidC) Coerce(v interface{}, path []string) (interface{}, error) {
	if v != nil && reflect.TypeOf(v).Kind() == reflect.String {
		uuid := reflect.ValueOf(v).String()
		if uuidregex.MatchString(uuid) {
			return uuid, nil
		}
	}
	return nil, error_{"uuid", v, path}
}

// Stringified returns a checker that accepts a bool/int/float/string
// value and returns its string. Other value types may be supported by
// passing in their checkers.
func Stringified(checkers ...Checker) Checker {
	return stringifiedC{
		checkers: checkers,
	}
}

type stringifiedC struct {
	checkers []Checker
}

func (c stringifiedC) Coerce(v interface{}, path []string) (interface{}, error) {
	if newStr, err := String().Coerce(v, path); err == nil {
		return newStr, nil
	}
	_, err := OneOf(append(c.checkers,
		Bool(),
		Int(),
		Float(),
		String(),
	)...).Coerce(v, path)
	if err != nil {
		return nil, err
	}
	return fmt.Sprintf("%#v", v), nil
}

// NonEmptyString returns a Checker that only accepts non-empty strings. To
// tweak the error message, valueLabel can contain a label of the value being
// checked, e.g. "my special name". If valueLabel is "", "string" will be used
// as a label instead.
//
// Example 1:
// schema.NonEmptyString("widget").Coerce("", nil) will return an error message
// like `expected non-empty widget, got string("")`.
//
// Example 2:
// schema.NonEmptyString("").Coerce("", nil) will return an error message like
// `expected non-empty string, got string("")`.
func NonEmptyString(valueLabel string) Checker {
	if valueLabel == "" {
		valueLabel = "string"
	}
	return nonEmptyStringC{valueLabel}
}

type nonEmptyStringC struct {
	valueLabel string
}

func (c nonEmptyStringC) Coerce(v interface{}, path []string) (interface{}, error) {
	label := fmt.Sprintf("non-empty %s", c.valueLabel)
	invalidError := error_{label, v, path}

	if v == nil || reflect.TypeOf(v).Kind() != reflect.String {
		return nil, invalidError
	}
	if stringValue := reflect.ValueOf(v).String(); stringValue != "" {
		return stringValue, nil
	}
	return nil, invalidError
}
