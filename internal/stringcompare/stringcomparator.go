// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stringcompare

// LevenshteinDistance calculates the Levenshtein distance between strings a and b.
//
// The Levenshtein distance is a measure of the minimum number of
// single-character edits (insertions, deletions, or substitutions)
// required to transform one string into the other.
//
// Adapted from a Golang implementation shared on https://groups.google.com/forum/#!topic/golang-nuts/YyH1f_qCZVc
//
// Returns the computed Levenshtein distance as an integer.
func LevenshteinDistance(a, b string) int {
	la := len(a)
	lb := len(b)
	d := make([]int, la+1)
	var lastdiag, olddiag, temp int

	for i := 1; i <= la; i++ {
		d[i] = i
	}
	for i := 1; i <= lb; i++ {
		d[0] = i
		lastdiag = i - 1
		for j := 1; j <= la; j++ {
			olddiag = d[j]
			min := d[j] + 1
			if (d[j-1] + 1) < min {
				min = d[j-1] + 1
			}
			if a[j-1] == b[i-1] {
				temp = 0
			} else {
				temp = 1
			}
			if (lastdiag + temp) < min {
				min = lastdiag + temp
			}
			d[j] = min
			lastdiag = olddiag
		}
	}
	return d[la]
}
