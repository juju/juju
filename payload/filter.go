// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload

import (
	"strings"
)

// TODO(ericsnow) Rename to Predicate.

// A PayloadPredicate determines if the given payload matches
// the condition the predicate represents.
type PayloadPredicate func(FullPayloadInfo) bool

// Filter applies the provided predicates to the payloads and returns
// only those that matched.
func Filter(payloads []FullPayloadInfo, predicates ...PayloadPredicate) []FullPayloadInfo {
	var results []FullPayloadInfo
	for _, payload := range payloads {
		if matched := filterOne(payload, predicates); matched {
			results = append(results, payload)
		}
	}
	return results
}

func filterOne(payload FullPayloadInfo, predicates []PayloadPredicate) bool {
	if len(predicates) == 0 {
		return true
	}

	for _, pred := range predicates {
		if matched := pred(payload); matched {
			return true
		}
	}
	return false
}

// TODO(ericsnow) BuildPredicatesFor is mostly something that can be generalized...

// BuildPredicatesFor converts the provided patterns into predicates
// that may be passed to Filter.
func BuildPredicatesFor(patterns []string) ([]PayloadPredicate, error) {
	var predicates []PayloadPredicate
	for i := range patterns {
		pattern := patterns[i]

		predicate := func(payload FullPayloadInfo) bool {
			return Match(payload, pattern)
		}
		predicates = append(predicates, predicate)
	}
	return predicates, nil
}

// Match determines if the given payload matches the pattern.
func Match(payload FullPayloadInfo, pattern string) bool {
	pattern = strings.ToLower(pattern)

	switch {
	case strings.ToLower(payload.Name) == pattern:
		return true
	case strings.ToLower(payload.Type) == pattern:
		return true
	case strings.ToLower(payload.ID) == pattern:
		return true
	case strings.ToLower(payload.Status) == pattern:
		return true
	case strings.ToLower(payload.Unit) == pattern:
		return true
	case strings.ToLower(payload.Machine) == pattern:
		return true
	default:
		for _, tag := range payload.Labels {
			if strings.ToLower(tag) == pattern {
				return true
			}
		}
	}
	return false
}
