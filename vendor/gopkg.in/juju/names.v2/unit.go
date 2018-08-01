// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"hash/crc32"
	"regexp"
	"strconv"
	"strings"
)

const UnitTagKind = "unit"

// minShortenedLength defines minimum size of shortened unit tag, other things depend
// on that value so change it carefully.
const minShortenedLength = 21

// UnitSnippet defines the regexp for a valid Unit Id.
const UnitSnippet = "(" + ApplicationSnippet + ")/" + NumberSnippet

var validUnit = regexp.MustCompile("^" + UnitSnippet + "$")

type UnitTag struct {
	name string
}

func (t UnitTag) String() string { return t.Kind() + "-" + t.name }
func (t UnitTag) Kind() string   { return UnitTagKind }
func (t UnitTag) Id() string     { return unitTagSuffixToId(t.name) }

// Number returns the unit number from the tag, effectively the NumberSnippet from the
// validUnit regular expression.
func (t UnitTag) Number() int {
	if i := strings.LastIndex(t.name, "-"); i > 0 {
		num, _ := strconv.Atoi(t.name[i+1:])
		return num
	}
	return 0
}

// NewUnitTag returns the tag for the unit with the given name.
// It will panic if the given unit name is not valid.
func NewUnitTag(unitName string) UnitTag {
	tag, ok := tagFromUnitName(unitName)
	if !ok {
		panic(fmt.Sprintf("%q is not a valid unit name", unitName))
	}
	return tag
}

// ParseUnitTag parses a unit tag string.
func ParseUnitTag(unitTag string) (UnitTag, error) {
	tag, err := ParseTag(unitTag)
	if err != nil {
		return UnitTag{}, err
	}
	ut, ok := tag.(UnitTag)
	if !ok {
		return UnitTag{}, invalidTagError(unitTag, UnitTagKind)
	}
	return ut, nil
}

// IsValidUnit returns whether name is a valid unit name.
func IsValidUnit(name string) bool {
	return validUnit.MatchString(name)
}

// UnitApplication returns the name of the application that the unit is
// associated with. It returns an error if unitName is not a valid unit name.
func UnitApplication(unitName string) (string, error) {
	s := validUnit.FindStringSubmatch(unitName)
	if s == nil {
		return "", fmt.Errorf("%q is not a valid unit name", unitName)
	}
	return s[1], nil
}

func tagFromUnitName(unitName string) (UnitTag, bool) {
	// Replace only the last "/" with "-".
	i := strings.LastIndex(unitName, "/")
	if i <= 0 || !IsValidUnit(unitName) {
		return UnitTag{}, false
	}
	unitName = unitName[:i] + "-" + unitName[i+1:]
	return UnitTag{name: unitName}, true
}

func unitTagSuffixToId(s string) string {
	// Replace only the last "-" with "/", as it is valid for application
	// names to contain hyphens.
	if i := strings.LastIndex(s, "-"); i > 0 {
		s = s[:i] + "/" + s[i+1:]
	}
	return s
}

// ShortenedString returns the length-limited string for the tag.
// It can be used in places where there are strict length requirements, e.g. for
// a service name. It uses a hash so the resulting name should be unique.
// It will panic if maxLength is less than minShortenedLength.
func (t UnitTag) ShortenedString(maxLength int) (string, error) {
	if maxLength < minShortenedLength {
		return "", fmt.Errorf("max length must be at least %d, not %d", minShortenedLength, maxLength)
	}
	i := strings.LastIndex(t.name, "-")
	if i <= 0 {
		return "", fmt.Errorf("invalid tag %s", t.name)
	}
	// To keep unit 'name' the same on all units we reserve 4 chars for ID.
	name, id := t.name[:i], t.name[i+1:]
	idLen := len(id)
	if idLen < 4 {
		idLen = 4
	}
	var hashString string
	// 8 for hash, 2 for two dashes
	maxNameLength := maxLength - idLen - len(UnitTagKind) - 8 - 2
	if len(name) > maxNameLength {
		hash := crc32.Checksum([]byte(name), crc32.IEEETable)
		hashString = fmt.Sprintf("%0.8x", hash)
		name = name[:maxNameLength]
	}
	return "unit-" + name + hashString + "-" + id, nil
}
