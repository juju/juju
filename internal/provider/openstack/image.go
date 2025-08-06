// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/go-goose/goose/v5/nova"
	"github.com/juju/errors"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
)

// FlavorFilter is an interface that can control which server flavors
// are acceptable.
type FlavorFilter interface {
	// AcceptFlavor returns true iff the given flavor is acceptable.
	AcceptFlavor(nova.FlavorDetail) bool
}

// FlavorFilterFunc is a function type that implements FlavorFilter.
type FlavorFilterFunc func(nova.FlavorDetail) bool

// AcceptFlavor is part of the FlavorFilter interface.
func (f FlavorFilterFunc) AcceptFlavor(d nova.FlavorDetail) bool {
	return f(d)
}

// AcceptAllFlavors is a function that returns true for any input,
// and can be assigned to a value of type FlavorFilterFunc.
func AcceptAllFlavors(nova.FlavorDetail) bool {
	return true
}

// findInstanceSpec returns an image and instance type satisfying the constraint.
// The instance type comes from querying the flavors supported by the deployment.
func findInstanceSpec(
	e *Environ,
	ic instances.InstanceConstraint,
	imageMetadata []*imagemetadata.ImageMetadata,
) (*instances.InstanceSpec, error) {
	// First construct all available instance types from the supported flavors.
	nova := e.nova()
	flavors, err := nova.ListFlavorsDetail()
	if err != nil {
		return nil, err
	}

	if ic.Constraints.HasRootDiskSource() && *ic.Constraints.RootDiskSource == "volume" {
		// When the root disk is a volume (i.e. cinder block volume)
		// we don't want to match on RootDisk size. If an instance requires
		// a very large root disk we don't want to select a larger instance type
		// to fit a disk that won't be local to the instance.
		ic.Constraints.RootDisk = nil
	}

	// Not all needed information is available in flavors,
	// for e.g. architectures or virtualisation types.
	// For these properties, we assume that all instance types support
	// all values.
	var allInstanceTypes []instances.InstanceType
	for _, flavor := range flavors {
		if !e.flavorFilter.AcceptFlavor(flavor) {
			continue
		}
		isSev := flavor.ExtraSpecs["hw:mem_encryption"] == "true"
		instanceType := instances.InstanceType{
			Id:       flavor.Id,
			Name:     flavor.Name,
			Arch:     ic.Arch,
			Mem:      uint64(flavor.RAM),
			CpuCores: uint64(flavor.VCPUs),
			RootDisk: uint64(flavor.Disk * 1024),
			IsSev:    isSev,
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
	spec, err := instances.FindInstanceSpec(images, &ic, allInstanceTypes)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// If instance constraints did not have a virtualisation type,
	// but image metadata did, we will have an instance type
	// with virtualisation type of an image.
	if !ic.Constraints.HasVirtType() && spec.Image.VirtType != "" {
		spec.InstanceType.VirtType = &spec.Image.VirtType
	}
	return spec, nil
}
