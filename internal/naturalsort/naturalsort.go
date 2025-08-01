// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
// Copied over from https://github.com/juju/naturalsort with minor adjustments.

package naturalsort

import (
	"fmt"
	"sort"
	"strconv"
	"unicode"
)

// Sort sorts strings according to their natural sort order.
func Sort(s []string) []string {
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

		aPrefix, aNumber, aRemainder, errA := splitAtNumber(aVal)
		bPrefix, bNumber, bRemainder, errB := splitAtNumber(bVal)

		if errA == nil && errB == nil {
			if aPrefix != bPrefix {
				return aPrefix < bPrefix
			}
			if aNumber != bNumber {
				return aNumber < bNumber
			}

			// Everything is the same so far, try again with the remainder.
			aVal = aRemainder
			bVal = bRemainder
			continue
		}

		return aVal < bVal
	}
}

// splitAtNumber splits given string at the first digit, returning the
// prefix before the number, the integer represented by the first
// series of digits, the remainder of the string after the first
// series of digits, and an error if parsing fails. If no digits are
// present, the number is returned as -1 and the remainder is empty.
func splitAtNumber(str string) (string, int, string, error) {
	i := indexOfDigit(str)
	if i == -1 {
		// no numbers
		return str, -1, "", nil
	}
	j := i + indexOfNonDigit(str[i:])
	num, err := strconv.Atoi(str[i:j])
	if err != nil {
		return str, -1, "", fmt.Errorf("parsing number %v: %v", str[i:j], err)
	}
	return str[:i], num, str[j:], nil
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
