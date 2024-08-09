// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interval

import (
	"sort"
)

// IntegerInterval represents the integers contained in the mathematical interval
// [Lower, Higher]. That is, the set of all integers x such that Lower <= x
// and x <= Upper.
type IntegerInterval struct {
	Lower int
	Upper int
}

// NewIntegerInterval returns a new IntegerInterval with the given bounds.
func NewIntegerInterval(a, b int) IntegerInterval {
	return IntegerInterval{
		Lower: min(a, b),
		Upper: max(a, b),
	}
}

// Intersects returns true if the two intervals contain any common elements.
func (i IntegerInterval) Intersects(other IntegerInterval) bool {
	return i.Lower <= other.Upper && i.Upper >= other.Lower
}

// Adjacent returns true if the two intervals are adjacent to each other. That
// is, if the upper bound of one interval is one less than the lower bound of
// the other.
//
// If two intervals are adjacent, they can be merged into a single interval.
func (i IntegerInterval) Adjacent(other IntegerInterval) bool {
	return i.Upper+1 == other.Lower || i.Lower-1 == other.Upper
}

// IsSupersetOf returns true if the other interval is a subset of this interval.
// That is, if all elements in this interval are also in the other interval.
func (i IntegerInterval) IsSubsetOf(other IntegerInterval) bool {
	return i.Lower >= other.Lower && i.Upper <= other.Upper
}

// Difference returns the set-difference of this interval with the other interval.
// That is, the set of all integers in this interval that are not in the other.
//
// NOTE: We return a slice of intervals because the difference of two intervals
// may result in two disjoint intervals, or no intervals at all.
func (i IntegerInterval) Difference(other IntegerInterval) IntegerIntervals {
	if !i.Intersects(other) {
		return []IntegerInterval{i}
	}

	if i.IsSubsetOf(other) {
		return []IntegerInterval{}
	}

	// We shouldn't just use IsSubsetOf because it would return true for
	// intervals which match at one end, meaning the difference only on
	// one side. This is covered in later cases.
	if i.Lower < other.Lower && i.Upper > other.Upper {
		return []IntegerInterval{
			{Lower: i.Lower, Upper: other.Lower - 1},
			{Lower: other.Upper + 1, Upper: i.Upper},
		}
	}

	if i.Lower < other.Lower {
		return []IntegerInterval{{
			Lower: i.Lower,
			Upper: other.Lower - 1,
		}}
	}
	return []IntegerInterval{{
		Lower: other.Upper + 1,
		Upper: i.Upper,
	}}
}

type IntegerIntervals []IntegerInterval

func NewIntegerIntervals(intervals ...IntegerInterval) IntegerIntervals {
	iis := IntegerIntervals{}
	for _, interval := range intervals {
		iis = iis.Union(interval)
	}
	return iis
}

func (is IntegerIntervals) Union(newInterval IntegerInterval) IntegerIntervals {
	res := IntegerIntervals{}
	i := 0
	merged := false
	for i < len(is) {
		interval := is[i]
		if !interval.Intersects(newInterval) && !interval.Adjacent(newInterval) {
			res = append(res, interval)
			i++
			continue
		}
		// It is possible that the new interval bridges across multiple existing
		// intervals. Since intervals are ordered, we can look ahead for the first
		// interval not touching the new interval, and then merge all the intervals
		// in between.
		n := i + 1
		for n < len(is) && (is[n].Intersects(newInterval) || is[n].Adjacent(newInterval)) {
			n++
		}
		mergedInterval := IntegerInterval{
			Lower: min(interval.Lower, newInterval.Lower),
			Upper: max(is[n-1].Upper, newInterval.Upper),
		}
		res = append(res, mergedInterval)
		i = n
		merged = true
	}
	if !merged {
		res = append(res, newInterval)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Lower < res[j].Lower
	})
	return res
}

func (is IntegerIntervals) Difference(subtraction IntegerInterval) IntegerIntervals {
	res := IntegerIntervals{}
	for _, interval := range is {
		res = append(res, interval.Difference(subtraction)...)
	}
	return res
}
