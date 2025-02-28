// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"regexp"

	gc "gopkg.in/check.v1"
)

type BytesToStringChecker struct {
	*gc.CheckerInfo
}

// BytesToStringMatch allows comparison of a []byte with a regex
// expression by converting the byte slice to a string and then
// performing a regex match.
var BytesToStringMatch gc.Checker = &BytesToStringChecker{
	&gc.CheckerInfo{Name: "BytesToStringChecker", Params: []string{"obtained", "expected"}},
}

func (c *BytesToStringChecker) Check(params []interface{}, name []string) (bool, string) {
	bytes, ok := params[0].([]byte)
	if !ok {
		return false, "param 0 is not of type []byte"
	}
	regexMatch, ok := params[1].(string)
	if !ok {
		return false, "param 1 is not of type string"
	}

	re := regexp.MustCompile(regexMatch)
	return re.Match(bytes), ""
}
