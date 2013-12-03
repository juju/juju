// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

import (
	"regexp"
	"strings"
)

const (
	ContainerTypeSnippet = "[a-z]+"
	ContainerSnippet     = "(/" + ContainerTypeSnippet + "/" + NumberSnippet + ")"
	MachineSnippet       = NumberSnippet + ContainerSnippet + "*"
)

var validMachine = regexp.MustCompile("^" + MachineSnippet + "$")

// IsMachine returns whether id is a valid machine id.
func IsMachine(id string) bool {
	return validMachine.MatchString(id)
}

// MachineTag returns the tag for the machine with the given id.
func MachineTag(id string) string {
	tag := makeTag(MachineTagKind, id)
	// Containers require "/" to be replaced by "-".
	tag = strings.Replace(tag, "/", "-", -1)
	return tag
}

func machineTagSuffixToId(s string) string {
	return strings.Replace(s, "-", "/", -1)
}
