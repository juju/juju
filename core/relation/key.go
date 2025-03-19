// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
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
