// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/juju/network"
	"github.com/juju/utils/set"
)

var dashPrefix = regexp.MustCompile("^-*")
var dashSuffix = regexp.MustCompile("-*$")
var multipleDashes = regexp.MustCompile("--+")

// ConvertSpaceName returns a string derived from name that does not
// already exist in existing. It does not modify existing.
func ConvertSpaceName(name string, existing set.Strings) string {
	// First lower case and replace spaces with dashes.
	name = strings.Replace(name, " ", "-", -1)
	name = strings.ToLower(name)
	// Replace any character that isn't in the set "-", "a-z", "0-9".
	name = network.SpaceInvalidChars.ReplaceAllString(name, "")
	// Get rid of any dashes at the start as that isn't valid.
	name = dashPrefix.ReplaceAllString(name, "")
	// And any at the end.
	name = dashSuffix.ReplaceAllString(name, "")
	// Repleace multiple dashes with a single dash.
	name = multipleDashes.ReplaceAllString(name, "-")
	// Special case of when the space name was only dashes or invalid
	// characters!
	if name == "" {
		name = "empty"
	}
	// If this name is in use add a numerical suffix.
	if existing.Contains(name) {
		counter := 2
		for existing.Contains(name + fmt.Sprintf("-%d", counter)) {
			counter += 1
		}
		name = name + fmt.Sprintf("-%d", counter)
	}
	return name
}
