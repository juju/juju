// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"regexp"
	"strings"
)

const FilesystemTagKind = "filesystem"

// Filesystems may be bound to a machine or unit, meaning that the filesystem cannot
// exist without that machine or unit. We encode this in the tag.
var validFilesystem = regexp.MustCompile("^((" + MachineSnippet + "|" + UnitSnippet + ")/)?" + NumberSnippet + "$")

type FilesystemTag struct {
	id string
}

func (t FilesystemTag) String() string { return t.Kind() + "-" + t.id }
func (t FilesystemTag) Kind() string   { return FilesystemTagKind }
func (t FilesystemTag) Id() string     { return filesystemOrVolumeTagSuffixToId(t.id) }

// NewFilesystemTag returns the tag for the filesystem with the given name.
// It will panic if the given filesystem name is not valid.
func NewFilesystemTag(id string) FilesystemTag {
	tag, ok := tagFromFilesystemId(id)
	if !ok {
		panic(fmt.Sprintf("%q is not a valid filesystem id", id))
	}
	return tag
}

// ParseFilesystemTag parses a filesystem tag string.
func ParseFilesystemTag(filesystemTag string) (FilesystemTag, error) {
	tag, err := ParseTag(filesystemTag)
	if err != nil {
		return FilesystemTag{}, err
	}
	fstag, ok := tag.(FilesystemTag)
	if !ok {
		return FilesystemTag{}, invalidTagError(filesystemTag, FilesystemTagKind)
	}
	return fstag, nil
}

// IsValidFilesystem returns whether id is a valid filesystem id.
func IsValidFilesystem(id string) bool {
	return validFilesystem.MatchString(id)
}

// FilesystemMachine returns the machine component of the filesystem
// tag, and a boolean indicating whether or not there is a
// machine component.
func FilesystemMachine(tag FilesystemTag) (MachineTag, bool) {
	id := tag.Id()
	pos := strings.LastIndex(id, "/")
	if pos == -1 {
		return MachineTag{}, false
	}
	id = id[:pos]
	if !IsValidMachine(id) {
		return MachineTag{}, false
	}
	return NewMachineTag(id), true
}

// FilesystemUnit returns the unit component of the filesystem
// tag, and a boolean indicating whether or not there is a
// unit component.
func FilesystemUnit(tag FilesystemTag) (UnitTag, bool) {
	id := tag.Id()
	pos := strings.LastIndex(id, "/")
	if pos == -1 {
		return UnitTag{}, false
	}
	id = id[:pos]
	if !IsValidUnit(id) {
		return UnitTag{}, false
	}
	return NewUnitTag(id[:pos]), true
}

func tagFromFilesystemId(id string) (FilesystemTag, bool) {
	if !IsValidFilesystem(id) {
		return FilesystemTag{}, false
	}
	id = strings.Replace(id, "/", "-", -1)
	return FilesystemTag{id}, true
}

var validMachineSuffix = regexp.MustCompile("^(" + MachineSnippet + "-).*")

func filesystemOrVolumeTagSuffixToId(s string) string {
	if validMachineSuffix.MatchString(s) {
		return strings.Replace(s, "-", "/", -1)
	}
	// Replace only the last 2 "-" with "/", as it is valid for unit
	// names to contain hyphens
	for x := 0; x < 2; x++ {
		if i := strings.LastIndex(s, "-"); i > 0 {
			s = s[:i] + "/" + s[i+1:]
		}
	}
	return s
}
