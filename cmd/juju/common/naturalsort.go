// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// SortStringsNaturally sorts strings according to their natural sort order.
func SortStringsNaturally(s []string) []string {
	sort.Sort(naturally(s))
	return s
}

type naturally []string

func (n naturally) Len() int {
	return len(n)
}

func (n naturally) Swap(a, b int) {
	n[a], n[b] = n[b], n[a]
}

// Less sorts by non-numeric prefix and numeric suffix
// when one exists.
func (n naturally) Less(a, b int) bool {
	aPrefix, aNumber := splitAtNumber(n[a])
	bPrefix, bNumber := splitAtNumber(n[b])
	if aPrefix == bPrefix {
		return aNumber < bNumber
	}
	return n[a] < n[b]
}

// splitAtNumber splits given string into prefix and numeric suffix.
// If no numeric suffix exists, full original string is returned as
// prefix with -1 as a suffix.
func splitAtNumber(str string) (string, int) {
	i := strings.LastIndexFunc(str, func(r rune) bool {
		return !unicode.IsDigit(r)
	}) + 1
	if i == len(str) {
		// no numeric suffix
		return str, -1
	}
	n, err := strconv.Atoi(str[i:])
	if err != nil {
		panic(fmt.Sprintf("parsing number %v: %v", str[i:], err)) // should never happen
	}
	return str[:i], n
}
