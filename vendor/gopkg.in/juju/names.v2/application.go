// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"regexp"
)

const ApplicationTagKind = "application"

const (
	ApplicationSnippet = "(?:[a-z][a-z0-9]*(?:-[a-z0-9]*[a-z][a-z0-9]*)*)"
	NumberSnippet      = "(?:0|[1-9][0-9]*)"
)

var validApplication = regexp.MustCompile("^" + ApplicationSnippet + "$")

// IsValidApplication returns whether name is a valid application name.
func IsValidApplication(name string) bool {
	return validApplication.MatchString(name)
}

type ApplicationTag struct {
	Name string
}

func (t ApplicationTag) String() string { return t.Kind() + "-" + t.Id() }
func (t ApplicationTag) Kind() string   { return ApplicationTagKind }
func (t ApplicationTag) Id() string     { return t.Name }

// NewApplicationTag returns the tag for the application with the given name.
func NewApplicationTag(applicationName string) ApplicationTag {
	return ApplicationTag{Name: applicationName}
}

// ParseApplicationTag parses a application tag string.
func ParseApplicationTag(applicationTag string) (ApplicationTag, error) {
	tag, err := ParseTag(applicationTag)
	if err != nil {
		return ApplicationTag{}, err
	}
	st, ok := tag.(ApplicationTag)
	if !ok {
		return ApplicationTag{}, invalidTagError(applicationTag, ApplicationTagKind)
	}
	return st, nil
}
