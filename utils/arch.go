// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"regexp"
	"strings"
)

// The following constants define the machine architectures supported by Juju.
const (
	Arch_amd64 = "amd64"
	Arch_i386  = "i386"
	Arch_arm   = "arm"
	Arch_arm64 = "arm64"
	Arch_ppc64 = "ppc64"
)

// AllSupportedArches records the machine architectures recognised by Juju.
var AllSupportedArches = []string{
	Arch_amd64,
	Arch_i386,
	Arch_arm,
	Arch_arm64,
	Arch_ppc64,
}

// archREs maps regular expressions for matching
// `uname -m` to architectures recognised by Juju.
var archREs = []struct {
	*regexp.Regexp
	arch string
}{
	{regexp.MustCompile("amd64|x86_64"), Arch_amd64},
	{regexp.MustCompile("i[3-9]86"), Arch_i386},
	{regexp.MustCompile("armv.*"), Arch_arm},
	{regexp.MustCompile("aarch64"), Arch_arm64},
	{regexp.MustCompile("ppc64el|ppc64le"), Arch_ppc64},
}

// HostArch returns the Juju architecture of the machine on which it is run.
func HostArch() (string, error) {
	rawArch, err := RunCommand("uname", "-m")
	if err != nil {
		return "", err
	}
	return NormaliseArch(rawArch)
}

// NormaliseArch returns the Juju architecture corresponding to the
// output of `uname -m`.
func NormaliseArch(rawArch string) (string, error) {
	rawArch = strings.TrimSpace(rawArch)
	for _, re := range archREs {
		if re.Match([]byte(rawArch)) {
			return re.arch, nil
			break
		}
	}
	return "", fmt.Errorf("unrecognised architecture: %s", rawArch)
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
