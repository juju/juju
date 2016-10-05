// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2instancetypes

//go:generate go run process_cost_data.go -o generated.go index.json

import (
	"strings"

	"github.com/juju/juju/environs/instances"
)

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

// SupportsClassic reports whether the instance type with the given
// name can be run in EC2-Classic.
//
// At the time of writing, we know that the following instance type
// families support only VPC: C4, M4, P2, T2, X1. However, rather
// than hard-coding that list, we assume that any new instance type
// families support VPC only, and so we hard-code the inverse of the
// list at the time of writing.
//
// See:
//     http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-vpc.html#vpc-only-instance-types
func SupportsClassic(instanceType string) bool {
	parts := strings.SplitN(instanceType, ".", 2)
	if len(parts) < 2 {
		return false
	}
	switch strings.ToLower(parts[0]) {
	case
		"c1", "c3",
		"cc2",
		"cg1",
		"cr1",
		"d2",
		"g2",
		"hi1",
		"hs1",
		"i2",
		"m1", "m2", "m3",
		"r3",
		"t1":
		return true
	}
	return false
}
