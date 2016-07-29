// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"regexp"
	"strings"
)

const MachineTagKind = "machine"

const (
	ContainerTypeSnippet = "[a-z]+"
	ContainerSnippet     = "/" + ContainerTypeSnippet + "/" + NumberSnippet + ""
	MachineSnippet       = NumberSnippet + "(?:" + ContainerSnippet + ")*"
)

var validMachine = regexp.MustCompile("^" + MachineSnippet + "$")

// IsValidMachine returns whether id is a valid machine id.
func IsValidMachine(id string) bool {
	return validMachine.MatchString(id)
}

// IsContainerMachine returns whether id is a valid container machine id.
func IsContainerMachine(id string) bool {
	return validMachine.MatchString(id) && strings.Contains(id, "/")
}

type MachineTag struct {
	id string
}

func (t MachineTag) String() string { return t.Kind() + "-" + t.id }
func (t MachineTag) Kind() string   { return MachineTagKind }
func (t MachineTag) Id() string     { return machineTagSuffixToId(t.id) }

// NewMachineTag returns the tag for the machine with the given id.
func NewMachineTag(id string) MachineTag {
	id = strings.Replace(id, "/", "-", -1)
	return MachineTag{id: id}
}

// ParseMachineTag parses a machine tag string.
func ParseMachineTag(machineTag string) (MachineTag, error) {
	tag, err := ParseTag(machineTag)
	if err != nil {
		return MachineTag{}, err
	}
	mt, ok := tag.(MachineTag)
	if !ok {
		return MachineTag{}, invalidTagError(machineTag, MachineTagKind)
	}
	return mt, nil
}

func machineTagSuffixToId(s string) string {
	return strings.Replace(s, "-", "/", -1)
}
