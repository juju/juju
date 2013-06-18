// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers

import (
	. "launchpad.net/gocheck"
)

// IsTrue checker

type isTrueChecker struct {
	*CheckerInfo
}

var IsTrue Checker = &isTrueChecker{
	&CheckerInfo{Name: "IsTrue", Params: []string{"obtained"}},
}

func (checker *isTrueChecker) Check(params []interface{}, names []string) (result bool, error string) {
	value, result := params[0].(bool)
	if result {
		return value, ""
	}
	return false, "obtained value not a bool"
}
