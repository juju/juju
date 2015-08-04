// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

//  .
type SpaceListResults struct {
	Results []SpaceListResult
	Error   *Error
}

type SpaceListResult struct {
	Name    string
	Subnets []SubnetInfo
}

type SubnetInfo struct {
	CIDR       string
	Type       string
	ProviderId string
	Zones      []string
	Status     string
}
