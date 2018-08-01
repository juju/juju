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

// Parent returns the machineTag for the host of the container if the machineTag
// is a container, otherwise it returns nil.
func (t MachineTag) Parent() Tag {
	parts := strings.Split(t.id, "-")
	if len(parts) < 3 {
		return nil
	}
	return MachineTag{id: strings.Join(parts[:len(parts)-2], "-")}
}

// ContainerType returns the type of container for this machine.
// If the machine isn't a container, then the empty string is returned.
func (t MachineTag) ContainerType() string {
	parts := strings.Split(t.id, "-")
	size := len(parts)
	if size < 3 {
		return ""
	}
	return parts[size-2]
}

// ChildId returns just the last segment of the ID.
func (t MachineTag) ChildId() string {
	parts := strings.Split(t.id, "-")
	return parts[len(parts)-1]
}

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
