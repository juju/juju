// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package arch

import (
	"regexp"
	"runtime"
	"strings"

	"github.com/juju/collections/set"
)

const (
	// DefaultArchitecture represents the default architecture we expect to use
	// if none is present.
	DefaultArchitecture = AMD64
)

// Arch represents a platform architecture.
type Arch = string

// Arches defines a list of arches to compare against.
type Arches struct {
	set set.Strings
}

// AllArches creates a series of arches to compare against.
func AllArches() Arches {
	return Arches{
		set: set.NewStrings(AllSupportedArches...),
	}
}

// Contains checks to see if a given arch is found with in the list.
func (a Arches) Contains(arch Arch) bool {
	return a.set.Contains(arch)
}

// StringList returns an ordered list of strings.
// ArchAll will always be the front of the slice to show importance of the enum
// value.
func (a Arches) StringList() []string {
	return a.set.SortedValues()
}

func (a Arches) String() string {
	return strings.Join(a.set.SortedValues(), ",")
}

// The following constants define the machine architectures supported by Juju.
const (
	AMD64   = "amd64"
	ARM64   = "arm64"
	PPC64EL = "ppc64el"
	S390X   = "s390x"
	RISCV64 = "riscv64"
)

// AllSupportedArches records the machine architectures recognised by Juju.
var AllSupportedArches = []string{
	AMD64,
	ARM64,
	PPC64EL,
	S390X,
	RISCV64,
}

// UnsupportedArches records the machine architectures not supported by Juju.
// Note: don't make const to prevent referencing it.
var UnsupportedArches = []string{
	"i386", "armhf", "ppc",
}

// archREs maps regular expressions for matching
// `uname -m` to architectures recognised by Juju.
var archREs = []struct {
	*regexp.Regexp
	arch string
}{
	{Regexp: regexp.MustCompile("amd64|x86_64"), arch: AMD64},
	{Regexp: regexp.MustCompile("aarch64"), arch: ARM64},
	{Regexp: regexp.MustCompile("ppc64|ppc64el|ppc64le"), arch: PPC64EL},
	{Regexp: regexp.MustCompile("s390x"), arch: S390X},
	{Regexp: regexp.MustCompile("riscv64|risc$|risc-[vV]64"), arch: RISCV64},
}

// Override for testing.
var HostArch = hostArch

// hostArch returns the Juju architecture of the machine on which it is run.
func hostArch() string {
	return NormaliseArch(runtime.GOARCH)
}

// NormaliseArch returns the Juju architecture corresponding to a machine's
// reported architecture. The Juju architecture is used to filter simple
// streams lookup of tools and images.
func NormaliseArch(rawArch string) string {
	rawArch = strings.TrimSpace(rawArch)
	for _, re := range archREs {
		if re.Match([]byte(rawArch)) {
			return re.arch
		}
	}
	return rawArch
}

// ToGoArch converts to a GOARCH compatible string.
func ToGoArch(arch Arch) string {
	if arch == PPC64EL {
		return "ppc64le"
	}
	return string(arch)
}

// IsSupportedArch returns true if arch is one supported by Juju.
func IsSupportedArch(arch string) bool {
	for _, a := range AllSupportedArches {
		if a == arch {
			return true
		}
	}
	return false
}
