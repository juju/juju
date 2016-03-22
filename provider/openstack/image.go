// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
)

// findInstanceSpec returns an image and instance type satisfying the constraint.
// The instance type comes from querying the flavors supported by the deployment.
func findInstanceSpec(
	e *Environ,
	ic *instances.InstanceConstraint,
	imageMetadata []*imagemetadata.ImageMetadata,
) (*instances.InstanceSpec, error) {
	// First construct all available instance types from the supported flavors.
	nova := e.nova()
	flavors, err := nova.ListFlavorsDetail()
	if err != nil {
		return nil, err
	}
	// Not all needed information is available in flavors,
	// for e.g. architectures or virtualisation types.
	// For these properties, we assume that all instance types support
	// all values.
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
		if ic.Constraints.HasVirtType() {
			// Instance Type virtual type depends on the virtual type of the selected image, i.e.
			// picking an image with a virt type gives a machine with this virt type.
			instanceType.VirtType = ic.Constraints.VirtType
		}
		allInstanceTypes = append(allInstanceTypes, instanceType)
	}

	images := instances.ImageMetadataToImages(imageMetadata)
	spec, err := instances.FindInstanceSpec(images, ic, allInstanceTypes)
	if err != nil {
		return nil, err
	}

	// If instance constraints did not have a virtualisation type,
	// but image metadata did, we will have an instance type
	// with virtualisation type of an image.
	if !ic.Constraints.HasVirtType() && spec.Image.VirtType != "" {
		spec.InstanceType.VirtType = &spec.Image.VirtType
	}
	return spec, nil
}
