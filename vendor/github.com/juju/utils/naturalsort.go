// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"sort"
	"strconv"
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
	aVal := n[a]
	bVal := n[b]

	for {
		// If bVal is empty, then aVal can't be less than it.
		if bVal == "" {
			return false
		}
		// If aVal is empty here, then is must be less than bVal.
		if aVal == "" {
			return true
		}

		aPrefix, aNumber, aRemainder := splitAtNumber(aVal)
		bPrefix, bNumber, bRemainder := splitAtNumber(bVal)
		if aPrefix != bPrefix {
			return aPrefix < bPrefix
		}
		if aNumber != bNumber {
			return aNumber < bNumber
		}

		// Everything is the same so far, try again with the remainer.
		aVal = aRemainder
		bVal = bRemainder
	}
}

// splitAtNumber splits given string at the first digit, returning the
// prefix before the number, the integer represented by the first
// series of digits, and the remainder of the string after the first
// series of digits. If no digits are present, the number is returned
// as -1 and the remainder is empty.
func splitAtNumber(str string) (string, int, string) {
	i := indexOfDigit(str)
	if i == -1 {
		// no numbers
		return str, -1, ""
	}
	j := i + indexOfNonDigit(str[i:])
	n, err := strconv.Atoi(str[i:j])
	if err != nil {
		panic(fmt.Sprintf("parsing number %v: %v", str[i:j], err)) // should never happen
	}
	return str[:i], n, str[j:]
}

func indexOfDigit(str string) int {
	for i, rune := range str {
		if unicode.IsDigit(rune) {
			return i
		}
	}
	return -1
}

func indexOfNonDigit(str string) int {
	for i, rune := range str {
		if !unicode.IsDigit(rune) {
			return i
		}
	}
	return len(str)
}
