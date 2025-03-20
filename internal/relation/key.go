// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"slices"
	"strings"

	"github.com/juju/names/v6"
)

// Key is the natural key of a relation. "application:endpoint application:endpoint"
// in sorted order based on the application.
type Key string

func (k Key) String() string {
	return string(k)
}

// ParseKeyFromTagString returns a Key for the given string
// in relation tag format.
func ParseKeyFromTagString(s string) (Key, error) {
	relTag, err := names.ParseRelationTag(s)
	if err != nil {
		return "", err
	}
	return Key(relTag.Id()), nil
}

// NewKey generates a unique sorted string representation of relation endpoints
// based on their roles and identifiers. It is a natural key for relations.
func NewKey(endpoints []Endpoint) Key {
	eps := slices.SortedFunc(slices.Values(endpoints), func(ep1 Endpoint, ep2 Endpoint) int {
		if ep1.Role != ep2.Role {
			return roleOrder[ep1.Role] - roleOrder[ep2.Role]
		}
		return strings.Compare(ep1.String(), ep2.String())
	})
	endpointNames := make([]string, 0, len(eps))
	for _, ep := range eps {
		endpointNames = append(endpointNames, ep.String())
	}
	return Key(strings.Join(endpointNames, " "))
}
