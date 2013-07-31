// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"strings"
)

const (
	UnitTagKind    = "unit"
	MachineTagKind = "machine"
	ServiceTagKind = "service"
	EnvironTagKind = "environment"
	UserTagKind    = "user"
)

var validKinds = map[string]bool{
	UnitTagKind:    true,
	MachineTagKind: true,
	ServiceTagKind: true,
	EnvironTagKind: true,
	UserTagKind:    true,
}

// TagKind returns one of the *TagKind constants for the given tag, or
// an error if none matches.
func TagKind(tag string) (string, error) {
	i := strings.Index(tag, "-")
	if i <= 0 || !validKinds[tag[:i]] {
		return "", fmt.Errorf("%q is not a valid tag", tag)
	}
	return tag[:i], nil
}

func splitTag(tag string) (kind, rest string, err error) {
	kind, err = TagKind(tag)
	if err != nil {
		return "", "", err
	}
	return kind, tag[len(kind)+1:], nil
}

func makeTag(kind, rest string) string {
	return kind + "-" + rest
}
