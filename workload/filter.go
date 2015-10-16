// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload

import (
	"strings"
)

// Filter applies the provided predicates to the payloads and returns
// only those that matched.
func Filter(payloads []Payload, predicates ...func(Payload) bool) []Payload {
	var results []Payload
	for _, payload := range payloads {
		if matched := filterOne(payload, predicates); matched {
			results = append(results, payload)
		}
	}
	return results
}

func filterOne(payload Payload, predicates []func(Payload) bool) bool {
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

// TODO(ericsnow) ParseEntityFilters is mostly something that can be generalized...

// BuildPredicatesFor converts the provided patterns into predicates
// that may be passed to Filter.
func BuildPredicatesFor(patterns []string) ([]func(Payload) bool, error) {
	var predicates []func(Payload) bool
	for i := range patterns {
		pattern := strings.ToLower(patterns[i])

		predicate := func(payload Payload) bool {
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
				for _, tag := range payload.Tags {
					if strings.ToLower(tag) == pattern {
						return true
					}
				}
			}
			return false
		}
		predicates = append(predicates, predicate)
	}
	return predicates, nil
}
