// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

import (
	"regexp"
)

const (
	ServiceSnippet = "([a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*)"
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
