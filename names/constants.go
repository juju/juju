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

	UnitTagPrefix    = UnitTagKind + "-"
	MachineTagPrefix = MachineTagKind + "-"
	ServiceTagPrefix = ServiceTagKind + "-"
	EnvironTagPrefix = EnvironTagKind + "-"
	UserTagPrefix    = UserTagKind + "-"

	NumberSnippet = "(0|[1-9][0-9]*)"
)

// TagKind returns one of the *TagKind constants for the given tag, or
// an error if none matches.
func TagKind(tag string) (string, error) {
	switch {
	case strings.HasPrefix(tag, UnitTagPrefix):
		return UnitTagKind, nil
	case strings.HasPrefix(tag, MachineTagPrefix):
		return MachineTagKind, nil
	case strings.HasPrefix(tag, ServiceTagPrefix):
		return ServiceTagKind, nil
	case strings.HasPrefix(tag, EnvironTagPrefix):
		return EnvironTagKind, nil
	case strings.HasPrefix(tag, UserTagPrefix):
		return UserTagKind, nil
	default:
		return "", fmt.Errorf("%q is not a valid tag", tag)
	}
}
