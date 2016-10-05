// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2instancetypes

//go:generate go run process_cost_data.go -o generated.go index.json

import "github.com/juju/juju/environs/instances"

// RegionInstanceTypes returns the instance types for the named region.
func RegionInstanceTypes(region string) []instances.InstanceType {
	// NOTE(axw) at the time of writing, there is no cost
	// information for China (Beijing). For any regions
	// that we don't know about, we substitute us-east-1
	// and hope that they're equivalent.
	instanceTypes, ok := allInstanceTypes[region]
	if !ok {
		instanceTypes = allInstanceTypes["us-east-1"]
	}
	return instanceTypes
}
