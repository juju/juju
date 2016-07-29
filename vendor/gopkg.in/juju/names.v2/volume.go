// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"regexp"
	"strings"
)

const VolumeTagKind = "volume"

// Volumes may be bound to a machine, meaning that the volume cannot
// exist without that machine. We encode this in the tag to allow
var validVolume = regexp.MustCompile("^(" + MachineSnippet + "/)?" + NumberSnippet + "$")

type VolumeTag struct {
	id string
}

func (t VolumeTag) String() string { return t.Kind() + "-" + t.id }
func (t VolumeTag) Kind() string   { return VolumeTagKind }
func (t VolumeTag) Id() string     { return volumeTagSuffixToId(t.id) }

// NewVolumeTag returns the tag for the volume with the given ID.
// It will panic if the given volume ID is not valid.
func NewVolumeTag(id string) VolumeTag {
	tag, ok := tagFromVolumeId(id)
	if !ok {
		panic(fmt.Sprintf("%q is not a valid volume ID", id))
	}
	return tag
}

// ParseVolumeTag parses a volume tag string.
func ParseVolumeTag(volumeTag string) (VolumeTag, error) {
	tag, err := ParseTag(volumeTag)
	if err != nil {
		return VolumeTag{}, err
	}
	dt, ok := tag.(VolumeTag)
	if !ok {
		return VolumeTag{}, invalidTagError(volumeTag, VolumeTagKind)
	}
	return dt, nil
}

// IsValidVolume returns whether id is a valid volume ID.
func IsValidVolume(id string) bool {
	return validVolume.MatchString(id)
}

// VolumeMachine returns the machine component of the volume
// tag, and a boolean indicating whether or not there is a
// machine component.
func VolumeMachine(tag VolumeTag) (MachineTag, bool) {
	id := tag.Id()
	pos := strings.LastIndex(id, "/")
	if pos == -1 {
		return MachineTag{}, false
	}
	return NewMachineTag(id[:pos]), true
}

func tagFromVolumeId(id string) (VolumeTag, bool) {
	if !IsValidVolume(id) {
		return VolumeTag{}, false
	}
	id = strings.Replace(id, "/", "-", -1)
	return VolumeTag{id}, true
}

func volumeTagSuffixToId(s string) string {
	return strings.Replace(s, "-", "/", -1)
}
