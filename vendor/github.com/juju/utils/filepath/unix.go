// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package filepath

import (
	"strings"
)

// A substantial portion of this code comes from the Go stdlib code.

const (
	UnixSeparator     = '/' // OS-specific path separator
	UnixListSeparator = ':' // OS-specific path list separator
)

// UnixRenderer is a Renderer implementation for most flavors of Unix.
type UnixRenderer struct{}

// Base implements Renderer.
func (ur UnixRenderer) Base(path string) string {
	return Base(UnixSeparator, ur.VolumeName, path)
}

// Clean implements Renderer.
func (ur UnixRenderer) Clean(path string) string {
	return Clean(UnixSeparator, ur.VolumeName, path)
}

// Dir implements Renderer.
func (ur UnixRenderer) Dir(path string) string {
	return Dir(UnixSeparator, ur.VolumeName, path)
}

// Ext implements Renderer.
func (UnixRenderer) Ext(path string) string {
	return Ext(UnixSeparator, path)
}

// FromSlash implements Renderer.
func (UnixRenderer) FromSlash(path string) string {
	return FromSlash(UnixSeparator, path)
}

// IsAbs implements Renderer.
func (UnixRenderer) IsAbs(path string) bool {
	return strings.HasPrefix(path, string(UnixSeparator))
}

// Join implements Renderer.
func (ur UnixRenderer) Join(path ...string) string {
	return Join(UnixSeparator, ur.VolumeName, path...)
}

// Match implements Renderer.
func (UnixRenderer) Match(pattern, name string) (matched bool, err error) {
	return Match(UnixSeparator, pattern, name)
}

// Split implements Renderer.
func (ur UnixRenderer) Split(path string) (dir, file string) {
	return Split(UnixSeparator, ur.VolumeName, path)
}

// SplitList implements Renderer.
func (UnixRenderer) SplitList(path string) []string {
	if path == "" {
		return []string{}
	}
	return strings.Split(path, string(UnixListSeparator))
}

// ToSlash implements Renderer.
func (UnixRenderer) ToSlash(path string) string {
	return ToSlash(UnixSeparator, path)
}

// VolumeName implements Renderer.
func (UnixRenderer) VolumeName(path string) string {
	return ""
}

// NormCase implements Renderer.
func (UnixRenderer) NormCase(path string) string {
	return path
}

// SplitSuffix implements Renderer.
func (UnixRenderer) SplitSuffix(path string) (string, string) {
	return splitSuffix(path)
}
