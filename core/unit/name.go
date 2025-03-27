// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"fmt"
	"regexp"

	"github.com/juju/juju/internal/errors"
)

const (
	// applicationSnippet is a non-compiled regexp that can be composed with
	// other snippets to form a valid application regexp.
	//
	// Application names a series lower case alpha-numeric strings, which can be
	// broken up with hyphens. The first character must be a letter. Each segment
	// must contain at least one letter.
	applicationPattern = "(?:[a-z][a-z0-9]*(?:-[a-z0-9]*[a-z][a-z0-9]*)*)"

	// numberPattern is a non-compiled regexp that can be composed with other
	// snippets for validating small number sequences.
	//
	// Numbers are a series of digits, with no leading zeros unless the number
	// is exactly 0.
	numberPattern = "(?:0|[1-9][0-9]*)"

	// unitPattern is a non-compiled regexp for a valid unit name.
	unitPattern = "(" + applicationPattern + ")/(" + numberPattern + ")"
)

const (
	InvalidUnitName = errors.ConstError("invalid unit name")
)

var validUnit = regexp.MustCompile("^" + unitPattern + "$")

// Name represents a units name, used as a human-readable unique identifier.
type Name string

// NewName returns a new Name. If the name is invalid, an InvalidUnitName error
// will be returned.
func NewName(name string) (Name, error) {
	n := Name(name)
	return n, n.Validate()
}

// NewNameFromParts returns a new Name from the application and number parts. If
// the name is invalid, an InvalidUnitName error will be returned.
func NewNameFromParts(applicationName string, number int) (Name, error) {
	return NewName(fmt.Sprintf("%s/%d", applicationName, number))
}

// String returns the Name as a string.
func (n Name) String() string {
	return string(n)
}

// Validate returns an error if the Name is invalid. The returned error is an
// InvalidUnitName error.
func (n Name) Validate() error {
	if !validUnit.MatchString(n.String()) {
		return errors.Errorf(": %q", InvalidUnitName, n)
	}
	return nil
}

// Application returns the name of the application that the unit is
// associated with. The name must be valid.
func (n Name) Application() string {
	s := validUnit.FindStringSubmatch(n.String())
	if s == nil {
		// Should never happen.
		return ""
	}
	return s[1]
}
