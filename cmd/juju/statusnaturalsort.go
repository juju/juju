// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"regexp"
	"strconv"
)

type naturally []string

var splitRegexp = regexp.MustCompile("^(?P<prefix>.*?)(?P<number>\\d+)$")

func (n naturally) Len() int {
	return len(n)
}

func (n naturally) Swap(a, b int) {
	n[a], n[b] = n[b], n[a]
}

// Less sorts by non-numeric prefix and numeric suffix.
// For example, both "abc" and "abc/999' and "abc999" are catered for.
// However, "abc123defg142/h2' is not really.
func (n naturally) Less(a, b int) bool {
	aPrefix, aNumber := splitAtNumber(n[a])
	bPrefix, bNumber := splitAtNumber(n[b])
	if aPrefix == bPrefix {
		return aNumber < bNumber
	}
	return n[a] < n[b]
}

// splitAtNumber splits given string at first encountered digit.
// It returns non-numeric prefix and numeric suffix. For e.g.:
//     "abc"        > "abc", -1
//     "abc/999"    > "abc/", 999
//     "abc999"     > "abc", 999
func splitAtNumber(str string) (string, int) {
	prefix := splitRegexp.ReplaceAllString(str, "$prefix")
	number := splitRegexp.ReplaceAllString(str, "$number")

	if prefix == number {
		// no number suffix exists
		return str, -1
	}
	new_s, err := strconv.Atoi(number)
	if err != nil {
		panic(fmt.Sprintf("parsing number %v", err)) // should never happen
	}
	return prefix, new_s
}
