// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
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

// Less sorts by non-numeric prefix and numeric suffix
// when one exists.
func (n naturally) Less(a, b int) bool {
	return n.examineElement(n[a], n[b])
}

// Recursion: no characters are ignored, not even white spaces and leading 0s
func (n naturally) examineElement(a, b string) bool {
	// CHECK IF SHOULD JUMP OUT OF RECURSION
	if done, answer := n.isOneStringFinished(a, b); done {
		return answer
	}

	if compared, outcome := n.comparedNumbers(a, b); compared {
		return outcome
	}

	// no numbers are encountered
	if a[0] == b[0] {
		return n.examineElement(a[1:], b[1:])
	}
	return a[0] < b[0]
}

func (n naturally) comparedNumbers(a, b string) (compared, result bool) {
	aIsNumber, aEnd, aFloat := n.extractNumber(a)
	bIsNumber, bEnd, bFloat := n.extractNumber(b)

	// dealing with 2 numbers
	if aIsNumber && bIsNumber {
		// if numbers are equal then recurse
		if aFloat == bFloat {
			return true, n.examineElement(aEnd, bEnd)
		}
		return true, aFloat < bFloat
	}
	if aIsNumber {
		// number go before letter in alphanumeric ordering
		return true, true
	}
	if bIsNumber {
		// number go before letter in alphanumeric ordering
		return true, false
	}
	return false, false
}

func (n naturally) isOneStringFinished(a, b string) (finished, result bool) {
	// check a length
	if len(a) == 0 {
		// matters if b has something in it
		if len(b) > 0 {
			return true, true
		}
		return true, false
	}
	// check b length
	if len(b) == 0 {
		// matters if a has something in it
		if len(a) > 0 {
			return true, false
		}
		return true, true
	}
	return false, true
}

func (n naturally) extractNumber(original string) (found bool, suffix string, number float64) {
	if unicode.IsDigit(rune(original[0])) {
		found = true
		end := strings.IndexFunc(original, func(r rune) bool {
			return !unicode.IsDigit(r)
		})
		// Handle float vs IP
		if end > -1 && end < len(original) {
			//look forward is this a float, get its decimal?
			// otherwise treat as dot separated numbers, for eg. IPs
			if rune(original[end]) == rune("."[0]) {
				//could be a float
				maybeEnd := strings.IndexFunc(original[end+1:], func(r rune) bool {
					return !unicode.IsDigit(r)
				})
				if maybeEnd == -1 {
					// the whole string is a float
					end = -1
				} else if maybeEnd < len(original[end:]) {
					if rune(original[maybeEnd+end+1]) != rune("."[0]) {
						// just this is a float followed by other characters
						end = end + maybeEnd + 1
					}
				}
			}
		}
		convert := original
		if end > -1 {
			convert = original[:end]
			suffix = original[end:]
		}
		var err error
		number, err = strconv.ParseFloat(convert, 64)
		if err != nil {
			panic(err)
		}
	}
	return found, suffix, number
}
