// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
)

// findInstanceSpec returns an image and instance type satisfying the constraint.
// The instance type comes from querying the flavors supported by the deployment.
func findInstanceSpec(e *Environ, ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	// first construct all available instance types from the supported flavors.
	nova := e.nova()
	flavors, err := nova.ListFlavorsDetail()
	if err != nil {
		return nil, err
	}
	allInstanceTypes := []instances.InstanceType{}
	for _, flavor := range flavors {
		instanceType := instances.InstanceType{
			Id:       flavor.Id,
			Name:     flavor.Name,
			Arches:   ic.Arches,
			Mem:      uint64(flavor.RAM),
			CpuCores: uint64(flavor.VCPUs),
			RootDisk: uint64(flavor.Disk * 1024),
			// tags not currently supported on openstack
		}
		allInstanceTypes = append(allInstanceTypes, instanceType)
	}

	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{ic.Region, e.ecfg().authURL()},
		Series:    []string{ic.Series},
		Arches:    ic.Arches,
		Stream:    e.Config().ImageStream(),
	})
	sources, err := environs.ImageMetadataSources(e)
	if err != nil {
		return nil, err
	}
	// TODO (wallyworld): use an env parameter (default true) to mandate use of only signed image metadata.
	matchingImages, _, err := imagemetadata.Fetch(sources, imageConstraint, false)
	if err != nil {
		return nil, err
	}
	images := instances.ImageMetadataToImages(matchingImages)
	spec, err := instances.FindInstanceSpec(images, ic, allInstanceTypes)
	if err != nil {
		return nil, err
	}
	return spec, nil
}
