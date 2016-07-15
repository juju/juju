// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"regexp"
	"strings"
)

const UnitTagKind = "unit"

var validUnit = regexp.MustCompile("^(" + ApplicationSnippet + ")/" + NumberSnippet + "$")

type UnitTag struct {
	name string
}

func (t UnitTag) String() string { return t.Kind() + "-" + t.name }
func (t UnitTag) Kind() string   { return UnitTagKind }
func (t UnitTag) Id() string     { return unitTagSuffixToId(t.name) }

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
