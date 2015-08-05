// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// SpaceListResults holds the list of all available spaces.
type SpaceListResults struct {
	Results []SpaceListResult
	Error   *Error
}

// SpaceListResult holds the information about a single space and its
// associated subnets.
type SpaceListResult struct {
	Name    string
	Subnets []Subnet
}
