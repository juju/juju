// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

// HostingURL defines the location which identifies offered services endpoints..
type HostingURL struct {
	Protocol string
	User     string
	Env      string
	Service  string
}

// IsValidURL checks if given string represents, i.e. can be parsed into, a valid Hosted URL.
func IsValidURL(url string) bool {
	// TODO(anastasiamac 2015-11-02) validate url
	return true
}
