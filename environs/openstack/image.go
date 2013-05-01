package openstack

import (
	"launchpad.net/juju-core/environs/instances"
)

// findInstanceSpec returns an image and instance type satisfying the constraint.
// The instance type comes from querying the flavors supported by the deployment.
func findInstanceSpec(e *environ, ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
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
			Cost:     uint64(flavor.RAM),
		}
		allInstanceTypes = append(allInstanceTypes, instanceType)
	}

	// look first in the control bucket and then the public bucket to find the release files containing the
	// metadata for available images. The format of the data in the files is found at
	// https://help.ubuntu.com/community/UEC/Images.
	var spec *instances.InstanceSpec
	releasesFile := "image-metadata/released.js"
	r, err := e.Storage().Get(releasesFile)
	if err != nil {
		r, err = e.PublicStorage().Get(releasesFile)
	}
	if err == nil {
		defer r.Close()
	}
	spec, err = instances.FindInstanceSpec(r, ic, allInstanceTypes)
	if err != nil {
		return nil, err
	}
	return spec, nil
}
