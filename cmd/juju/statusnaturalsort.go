// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type naturally []string

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
	if str == "" {
		return str, -1
	}
	num := make([]string, 0)
	alpha := make([]string, 0)
	for _, element := range str {
		if unicode.IsNumber(element) {
			num = append(num, string(element))
			continue
		}
		if len(num) > 0 {
			//have already got all the numbers
			//and are back to non-numeric characters
			break
		}
		alpha = append(alpha, string(element))
	}
	if len(num) == 0 {
		// no numbers in given string
		return str, -1
	}
	new_s, err := strconv.Atoi(strings.Join(num, ""))
	if err != nil {
		panic(fmt.Sprintf("parsing number %v", err)) // should never happen
	}
	return strings.Join(alpha, ""), new_s
}
