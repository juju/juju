// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package version implements version parsing.
package version

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/mgo.v2/bson"
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
	Series string
	Arch   string
}

// String returns the string representation of the binary version.
func (b Binary) String() string {
	return fmt.Sprintf("%v-%s-%s", b.Number, b.Series, b.Arch)
}

// GetBSON implements bson.Getter.
func (b Binary) GetBSON() (interface{}, error) {
	return b.String(), nil
}

// SetBSON implements bson.Setter.
func (b *Binary) SetBSON(raw bson.Raw) error {
	var s string
	err := raw.Unmarshal(&s)
	if err != nil {
		return err
	}
	v, err := ParseBinary(s)
	if err != nil {
		return err
	}
	*b = v
	return nil
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

var (
	binaryPat = regexp.MustCompile(`^(\d{1,9})\.(\d{1,9})(?:\.|-([a-z]+))(\d{1,9})(\.\d{1,9})?-([^-]+)-([^-]+)$`)
	numberPat = regexp.MustCompile(`^(\d{1,9})\.(\d{1,9})(?:\.|-([a-z]+))(\d{1,9})(\.\d{1,9})?$`)
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
	m := binaryPat.FindStringSubmatch(s)
	if m == nil {
		return Binary{}, fmt.Errorf("invalid binary version %q", s)
	}
	var b Binary
	b.Major = atoi(m[1])
	b.Minor = atoi(m[2])
	b.Tag = m[3]
	b.Patch = atoi(m[4])
	if m[5] != "" {
		b.Build = atoi(m[5][1:])
	}
	b.Series = m[6]
	b.Arch = m[7]
	return b, nil
}

// Parse parses the version, which is of the form 1.2.3
// giving the major, minor and release versions
// respectively.
func Parse(s string) (Number, error) {
	m := numberPat.FindStringSubmatch(s)
	if m == nil {
		return Number{}, fmt.Errorf("invalid version %q", s)
	}
	var n Number
	n.Major = atoi(m[1])
	n.Minor = atoi(m[2])
	n.Tag = m[3]
	n.Patch = atoi(m[4])
	if m[5] != "" {
		n.Build = atoi(m[5][1:])
	}
	return n, nil
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

// GetBSON implements bson.Getter.
func (n Number) GetBSON() (interface{}, error) {
	return n.String(), nil
}

// SetBSON implements bson.Setter.
func (n *Number) SetBSON(raw bson.Raw) error {
	var s string
	err := raw.Unmarshal(&s)
	if err != nil {
		return err
	}
	v, err := Parse(s)
	if err != nil {
		return err
	}
	*n = v
	return nil
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
