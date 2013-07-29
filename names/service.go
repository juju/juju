// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

import (
	"regexp"
)

var validService = regexp.MustCompile("^" + ServiceSnippet + "$")

// IsServiceName returns whether name is a valid service name.
func IsServiceName(name string) bool {
	return validService.MatchString(name)
}

// ServiceTag returns the tag for the service with the given name.
func ServiceTag(serviceName string) string {
	return ServiceTagPrefix + serviceName
}
