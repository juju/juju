// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"launchpad.net/gwacl"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
)

const defaultMem = 1 * gwacl.GB

var (
	roleSizeCost = gwacl.RoleSizeCost
)

// If you specify no constraints at all, you're going to get the smallest
// instance type available.  In practice that one's a bit small.  So unless
// the constraints are deliberately set lower, this gives you a set of
// baseline constraints that are just slightly more ambitious than that.
func defaultToBaselineSpec(constraint constraints.Value) constraints.Value {
	result := constraint
	if !result.HasInstanceType() && result.Mem == nil {
		var value uint64 = defaultMem
		result.Mem = &value
	}
	return result
}

// selectMachineType returns the Azure machine type that best matches the
// supplied instanceConstraint.
func selectMachineType(env *azureEnviron, cons constraints.Value) (*instances.InstanceType, error) {
	instanceTypes, err := listInstanceTypes(env)
	if err != nil {
		return nil, err
	}
	region := env.getSnapshot().ecfg.location()
	instanceTypes, err = instances.MatchingInstanceTypes(instanceTypes, region, cons)
	if err != nil {
		return nil, err
	}
	return &instanceTypes[0], nil
}

// getEndpoint returns the simplestreams endpoint to use for the given Azure
// location (e.g. West Europe or China North).
func getEndpoint(location string) string {
	// Simplestreams uses the management-API endpoint for the image, not
	// the base managent API URL.
	return gwacl.GetEndpoint(location).ManagementAPI()
}

// As long as this code only supports the default simplestreams
// database, which is always signed, there is no point in accepting
// unsigned metadata.
//
// For tests, however, unsigned data is more convenient.  They can override
// this setting.
var signedImageDataOnly = true

// findMatchingImages queries simplestreams for OS images that match the given
// requirements.
//
// If it finds no matching images, that's an error.
func findMatchingImages(e *azureEnviron, location, series string, arches []string) ([]*imagemetadata.ImageMetadata, error) {
	endpoint := getEndpoint(location)
	constraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{location, endpoint},
		Series:    []string{series},
		Arches:    arches,
		Stream:    e.Config().ImageStream(),
	})
	sources, err := environs.ImageMetadataSources(e)
	if err != nil {
		return nil, err
	}
	images, _, err := imagemetadata.Fetch(sources, constraint, signedImageDataOnly)
	if len(images) == 0 || errors.IsNotFound(err) {
		return nil, fmt.Errorf("no OS images found for location %q, series %q, architectures %q (and endpoint: %q)", location, series, arches, endpoint)
	} else if err != nil {
		return nil, err
	}
	return images, nil
}

// newInstanceType creates an InstanceType based on a gwacl.RoleSize.
func newInstanceType(roleSize gwacl.RoleSize, region string) (instances.InstanceType, error) {
	cost, err := roleSizeCost(region, roleSize.Name)
	if err != nil {
		return instances.InstanceType{}, err
	}

	vtype := "Hyper-V"
	return instances.InstanceType{
		Id:       roleSize.Name,
		Name:     roleSize.Name,
		CpuCores: roleSize.CpuCores,
		Mem:      roleSize.Mem,
		RootDisk: roleSize.OSDiskSpace,
		Cost:     cost,
		VirtType: &vtype,
		// tags are not currently supported by azure
	}, nil
}

// listInstanceTypes describes the available instance types based on a
// description in gwacl's terms.
func listInstanceTypes(env *azureEnviron) ([]instances.InstanceType, error) {
	region := env.getSnapshot().ecfg.location()
	arches, err := env.SupportedArchitectures()
	if err != nil {
		return nil, err
	}
	types := make([]instances.InstanceType, 0, len(gwacl.RoleSizes))
	for _, roleSize := range gwacl.RoleSizes {
		// TODO(axw) 2014-06-23 #1324666
		// Support basic instance types. We need to not default
		// to them as they do not support load balancing.
		if strings.HasPrefix(roleSize.Name, "Basic_") {
			continue
		}
		instanceType, err := newInstanceType(roleSize, region)
		if err != nil {
			return nil, err
		}
		types = append(types, instanceType)
	}
	for i := range types {
		types[i].Arches = arches
	}
	return types, nil
}

// findInstanceSpec returns the InstanceSpec that best satisfies the supplied
// InstanceConstraint.
func findInstanceSpec(env *azureEnviron, constraint *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	constraint.Constraints = defaultToBaselineSpec(constraint.Constraints)
	imageData, err := findMatchingImages(env, constraint.Region, constraint.Series, constraint.Arches)
	if err != nil {
		return nil, err
	}
	images := instances.ImageMetadataToImages(imageData)
	instanceTypes, err := listInstanceTypes(env)
	if err != nil {
		return nil, err
	}
	return instances.FindInstanceSpec(images, constraint, instanceTypes)
}
