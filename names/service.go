// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	ServiceSnippet = "[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*"
	NumberSnippet  = "(0|[1-9][0-9]*)"
)

var validService = regexp.MustCompile("^" + ServiceSnippet + "$")

// IsService returns whether name is a valid service name.
func IsService(name string) bool {
	return validService.MatchString(name)
}

// ServiceTag returns the tag for the service with the given name.
func ServiceTag(serviceName string) string {
	return makeTag(ServiceTagKind, serviceName)
}

// ServiceFromUnitTag returns the service name for the given unit tag.
func ServiceFromUnitTag(tag string) string {
	_, name, err := ParseTag(tag, UnitTagKind)
	if err != nil {
		panic(fmt.Sprintf("%q is not a valid unit tag", tag))
	}
	// Strip only the last "/".
	i := strings.LastIndex(name, "/")
	if i <= 0 || !IsUnit(name) {
		panic(fmt.Sprintf("%q is not a valid unit tag", tag))
	}
	return name[:i]
}
