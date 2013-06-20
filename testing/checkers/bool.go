// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers

import (
	"fmt"
	"reflect"

	. "launchpad.net/gocheck"
)

// IsTrue checker

type isTrueChecker struct {
	*CheckerInfo
}

var IsTrue Checker = &isTrueChecker{
	&CheckerInfo{Name: "IsTrue", Params: []string{"obtained"}},
}

var IsFalse Checker = Not(IsTrue)

func (checker *isTrueChecker) Check(params []interface{}, names []string) (result bool, error string) {

	value := reflect.ValueOf(params[0])

	switch value.Kind() {
	case reflect.Bool:
		return value.Bool(), ""
	}

	return false, fmt.Sprintf("expected bool:true, received %s:%#v", value.Kind(), params[0])
}
