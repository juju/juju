// The version package implements version parsing.
// It also acts as guardian of the current client Juju version number.
package version

import (
	"fmt"
	"regexp"
	"strconv"
)

var Current = MustParse("0.0.0")

// Version represents a juju version. When bugs are
// fixed the patch number is incremented; when new features are added
// the minor number is incremented and patch is reset; and when
// compatibility is broken the major version is incremented and minor
// and patch are reset.  If any of the numbers is odd it
// indicates that the release is still in development.
type Version struct {
	Major int
	Minor int
	Patch int
}

var versionPat = regexp.MustCompile(`^(\d{1,9})\.(\d{1,9})\.(\d{1,9})$`)

// MustParse parses a version and panics if it does
// not parse correctly.
func MustParse(s string) Version {
	v, err := Parse(s)
	if err != nil {
		panic(fmt.Errorf("version: cannot parse %q: %v", s, err))
	}
	return v
}

// Parse parses the version, which is of the form 1.2.3
// giving the major, minor and release versions
// respectively.
func Parse(s string) (Version, error) {
	m := versionPat.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("invalid version %q", s)
	}
	var v Version
	v.Major = atoi(m[1])
	v.Minor = atoi(m[2])
	v.Patch = atoi(m[3])
	return v, nil
}

// atoi is the same as strconv.Atoi but assumes that
// the string has been verified to be a valid integer.
func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return n
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Less returns whether v is semantically earlier in the
// version sequence than w.
func (v Version) Less(w Version) bool {
	switch {
	case v.Major != w.Major:
		return v.Major < w.Major
	case v.Minor != w.Minor:
		return v.Minor < w.Minor
	case v.Patch != w.Patch:
		return v.Patch < w.Patch
	}
	return false
}

func isOdd(x int) bool {
	return x%2 != 0
}

// IsDev returns whether the version represents a development
// version. A version with an odd-numbered major, minor
// or patch version is considered to be a development version.
func (v Version) IsDev() bool {
	return isOdd(v.Major) || isOdd(v.Minor) || isOdd(v.Patch)
}
