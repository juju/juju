// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"regexp"
	"strings"
)

const FilesystemTagKind = "filesystem"

// Filesystems may be bound to a machine, meaning that the filesystem cannot
// exist without that machine. We encode this in the tag.
var validFilesystem = regexp.MustCompile("^(" + MachineSnippet + "/)?" + NumberSnippet + "$")

type FilesystemTag struct {
	id string
}

func (t FilesystemTag) String() string { return t.Kind() + "-" + t.id }
func (t FilesystemTag) Kind() string   { return FilesystemTagKind }
func (t FilesystemTag) Id() string     { return filesystemTagSuffixToId(t.id) }

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
	return NewMachineTag(id[:pos]), true
}

func tagFromFilesystemId(id string) (FilesystemTag, bool) {
	if !IsValidFilesystem(id) {
		return FilesystemTag{}, false
	}
	id = strings.Replace(id, "/", "-", -1)
	return FilesystemTag{id}, true
}

func filesystemTagSuffixToId(s string) string {
	return strings.Replace(s, "-", "/", -1)
}
