// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package arch

import (
	"regexp"
	"runtime"
	"strings"
)

// The following constants define the machine architectures supported by Juju.
const (
	AMD64 = "amd64"
	I386  = "i386"
	ARM   = "armhf"
	ARM64 = "arm64"
	PPC64 = "ppc64"
)

// AllSupportedArches records the machine architectures recognised by Juju.
var AllSupportedArches = []string{
	AMD64,
	I386,
	ARM,
	ARM64,
	PPC64,
}

// Info records the information regarding each architecture recognised by Juju.
var Info = map[string]ArchInfo{
	AMD64: {64},
	I386:  {32},
	ARM:   {32},
	ARM64: {64},
	PPC64: {64},
}

// ArchInfo is a struct containing information about a supported architecture.
type ArchInfo struct {
	// WordSize is the architecture's word size, in bits.
	WordSize int
}

// archREs maps regular expressions for matching
// `uname -m` to architectures recognised by Juju.
var archREs = []struct {
	*regexp.Regexp
	arch string
}{
	{regexp.MustCompile("amd64|x86_64"), AMD64},
	{regexp.MustCompile("i?[3-9]86"), I386},
	{regexp.MustCompile("(arm$)|(armv.*)"), ARM},
	{regexp.MustCompile("aarch64"), ARM64},
	{regexp.MustCompile("ppc64el|ppc64le"), PPC64},
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

// IsSupportedArch returns true if arch is one supported by Juju.
func IsSupportedArch(arch string) bool {
	for _, a := range AllSupportedArches {
		if a == arch {
			return true
		}
	}
	return false
}
