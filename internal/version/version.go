// Copyright 2025 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package version

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Number represents a version number.
type Number struct {
	Major int
	Minor int
	Tag   string
	Patch int
	Build int
}

// Zero is occasionally convenient and readable.
// Please don't change its value.
var Zero = Number{}

// Binary specifies a binary version of juju.v
type Binary struct {
	Number
	Release string
	Arch    string
}

// String returns the string representation of the binary version.
func (b Binary) String() string {
	return fmt.Sprintf("%v-%s-%s", b.Number, b.Release, b.Arch)
}

// MarshalJSON implements json.Marshaler.
func (b Binary) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (b *Binary) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, err := ParseBinary(s)
	if err != nil {
		return err
	}
	*b = v
	return nil
}

// MarshalYAML implements yaml.v2.Marshaller interface.
func (b Binary) MarshalYAML() (interface{}, error) {
	return b.String(), nil
}

// UnmarshalYAML implements the yaml.Unmarshaller interface.
func (b *Binary) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var vstr string
	err := unmarshal(&vstr)
	if err != nil {
		return err
	}
	v, err := ParseBinary(vstr)
	if err != nil {
		return err
	}
	*b = v
	return nil
}

const (
	// NumberRegex for matching version strings in the forms:
	// - 1.2
	// - 1.2.3
	// - 1.2.3.4
	// - 1.2-alpha3
	// - 1.2-alpha3.4
	NumberRegex = `(?P<major>\d{1,9})(\.((?P<minor>\d{1,9})((?:(-((?P<tag>[a-z]+)(?P<patchInTag>\d{1,9})?))|(\.(?P<patch>\d{1,9})))?)(\.(?P<build>\d{1,9}))?))?`
	// BinaryRegex for matching binary version strings in the form:
	// - 1.2-release-arch
	// - 1.2.3-release-arch
	// - 1.2.3.4-release-arch
	// - 1.2-alpha3-release-arch
	// - 1.2-alpha3.4-release-arch
	BinaryRegex = NumberRegex + `-(?P<release>[^-]+)-(?P<arch>[^-]+)`
)

var (
	binaryPat = regexp.MustCompile(`^` + BinaryRegex + `$`)
	numberPat = regexp.MustCompile(`^` + NumberRegex + `$`)
)

// MustParse parses a version and panics if it does
// not parse correctly.
func MustParse(s string) Number {
	v, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

// MustParseBinary parses a binary version and panics if it does
// not parse correctly.
func MustParseBinary(s string) Binary {
	b, err := ParseBinary(s)
	if err != nil {
		panic(err)
	}
	return b
}

// ParseBinary parses a binary version of the form "1.2.3-series-arch".
func ParseBinary(s string) (Binary, error) {
	groups := captureNamedGroups(s, binaryPat)
	n := parseVersion(groups, true)
	if n == nil {
		return Binary{}, fmt.Errorf("invalid binary version %q", s)
	}

	return Binary{
		Number:  *n,
		Release: groups["release"],
		Arch:    groups["arch"],
	}, nil
}

// Parse a version in strict mode. The following version patterns are accepted:
//
//	1.2.3       (major, minor, patch)
//	1.2-tag3    (major, minor, patch, tag)
//	1.2.3.4     (major, minor, patch, build)
//	1.2-tag3.4  (major, minor, patch, build)
//
// The ParseNonStrict function can be used instead to parse a wider range of
// version patterns (e.g. major only, major/minor etc.).
func Parse(s string) (Number, error) {
	groups := captureNamedGroups(s, numberPat)
	if n := parseVersion(groups, true); n != nil {
		return *n, nil
	}
	return Number{}, fmt.Errorf("invalid version %q", s)
}

// ParseNonStrict attempts to parse a version in non-strict mode. It supports
// the same patterns as Parse with the addition of some extra patterns that
// are not considered pure semantic version values.
//
// The following version patterns are accepted:
//
//	1           (major)
//	1.2         (major, minor)
//	1.2.3       (major, minor, patch)
//	1.2-tag     (major, minor, tag)
//	1.2-tag3    (major, minor, patch, tag)
//	1.2.3.4     (major, minor, patch, build)
//	1.2-tag3.4  (major, minor, patch, build)
func ParseNonStrict(s string) (Number, error) {
	groups := captureNamedGroups(s, numberPat)
	if n := parseVersion(groups, false); n != nil {
		return *n, nil
	}
	return Number{}, fmt.Errorf("invalid version %q", s)
}

func parseVersion(groups map[string]string, strict bool) *Number {
	var n Number

	// Major is always required
	major := groups["major"]
	if major == "" {
		return nil
	}
	n.Major = atoi(major)

	// Minor is only required in strict mode
	minor := groups["minor"]
	if minor == "" && strict {
		return nil
	} else if minor != "" {
		n.Minor = atoi(minor)
	}

	// Patch is only required in strict mode. However there can be two
	// possible patch groups depending on whether a tag is specified:
	// - "patch" captures a standalone patch version (e.g. 1.2.3)
	// - "patchInTag" captures a patch version as a suffix to tag (e.g. 1.2-tag3)
	patch := groups["patch"]
	if patch == "" {
		patch = groups["patchInTag"] // try the alternative
	}
	if patch == "" && strict {
		return nil
	} else if patch != "" {
		n.Patch = atoi(patch)
	}

	// Tag is always optional
	n.Tag = groups["tag"]

	// Build is always optional
	build := groups["build"]
	if build != "" {
		n.Build = atoi(build)
	}

	return &n
}

func captureNamedGroups(s string, re *regexp.Regexp) map[string]string {
	match := re.FindStringSubmatch(s)

	results := map[string]string{}
	groups := re.SubexpNames()
	for i, name := range match {
		results[groups[i]] = name
	}
	return results
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

// String returns the string representation of this Number.
func (n Number) String() string {
	var s string
	if n.Tag == "" {
		s = fmt.Sprintf("%d.%d.%d", n.Major, n.Minor, n.Patch)
	} else {
		s = fmt.Sprintf("%d.%d-%s%d", n.Major, n.Minor, n.Tag, n.Patch)
	}
	if n.Build > 0 {
		s += fmt.Sprintf(".%d", n.Build)
	}
	return s
}

// Compare returns -1, 0 or 1 depending on whether
// n is less than, equal to or greater than other.
// The comparison compares Major, then Minor, then Patch, then Build, using the first difference as
func (n Number) Compare(other Number) int {
	if n == other {
		return 0
	}
	less := false
	switch {
	case n.Major != other.Major:
		less = n.Major < other.Major
	case n.Minor != other.Minor:
		less = n.Minor < other.Minor
	case n.Tag != other.Tag:
		switch {
		case n.Tag == "":
			less = false
		case other.Tag == "":
			less = true
		default:
			less = n.Tag < other.Tag
		}
	case n.Patch != other.Patch:
		less = n.Patch < other.Patch
	case n.Build != other.Build:
		less = n.Build < other.Build
	}
	if less {
		return -1
	}
	return 1
}

// MarshalJSON implements json.Marshaler.
func (n Number) MarshalJSON() ([]byte, error) {
	return json.Marshal(n.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (n *Number) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, err := Parse(s)
	if err != nil {
		return err
	}
	*n = v
	return nil
}

// MarshalYAML implements yaml.v2.Marshaller interface
func (n Number) MarshalYAML() (interface{}, error) {
	return n.String(), nil
}

// UnmarshalYAML implements the yaml.Unmarshaller interface
func (n *Number) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var vstr string
	err := unmarshal(&vstr)
	if err != nil {
		return err
	}
	v, err := Parse(vstr)
	if err != nil {
		return err
	}
	*n = v
	return nil
}

// ToPatch returns back a semver Number (Major.Minor.Tag.Patch), without a build
// attached to the Number.
// In some scenarios it's prefable to not have the build number to identity a
// version and instead use a less qualified Number. Being less specific about
// exactness allows us to be more flexible about compatible with other versions.
func (n Number) ToPatch() Number {
	return Number{
		Major: n.Major,
		Minor: n.Minor,
		Patch: n.Patch,
		Tag:   n.Tag,
	}
}

// ParseMajorMinor takes an argument of the form "major.minor" and returns ints major and minor.
func ParseMajorMinor(vers string) (int, int, error) {
	parts := strings.Split(vers, ".")
	major, err := strconv.Atoi(parts[0])
	minor := -1
	if err != nil {
		return -1, -1, fmt.Errorf("invalid major version number %s: %v", parts[0], err)
	}
	if len(parts) == 2 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return -1, -1, fmt.Errorf("invalid minor version number %s: %v", parts[1], err)
		}
	} else if len(parts) > 2 {
		return -1, -1, fmt.Errorf("invalid major.minor version number %s", vers)
	}
	return major, minor, nil
}
