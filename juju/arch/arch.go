// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package arch

import (
	"fmt"
	"regexp"
	"strings"

	"launchpad.net/juju-core/utils"
)

// The following constants define the machine architectures supported by Juju.
const (
	AMD64 = "amd64"
	I386  = "i386"
	ARM   = "arm"
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

// archREs maps regular expressions for matching
// `uname -m` to architectures recognised by Juju.
var archREs = []struct {
	*regexp.Regexp
	arch string
}{
	{regexp.MustCompile("amd64|x86_64"), AMD64},
	{regexp.MustCompile("i[3-9]86"), I386},
	{regexp.MustCompile("armv.*"), ARM},
	{regexp.MustCompile("aarch64"), ARM64},
	{regexp.MustCompile("ppc64el|ppc64le"), PPC64},
}

// Override for testing.
var HostArch = hostArch

// HostArch returns the Juju architecture of the machine on which it is run.
func hostArch() (string, error) {
	rawArch, err := utils.RunCommand("uname", "-m")
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
