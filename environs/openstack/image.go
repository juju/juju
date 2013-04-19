package openstack

import (
	"bufio"
	"fmt"
	"launchpad.net/juju-core/environs"
)

// findInstanceSpec returns an image and instance type satisfying the constraint.
// The instance type comes from querying the flavors supported by the deployment.
func findInstanceSpec(e *environ, ic *environs.InstanceConstraint) (*environs.InstanceSpec, error) {
	// first construct the available instance types from the supported flavors.
	nova := e.nova()
	flavors, err := nova.ListFlavorsDetail()
	if err != nil {
		return nil, err
	}
	allInstanceTypes := []environs.InstanceType{}
	for _, flavor := range flavors {
		instanceType := environs.InstanceType{
			Id:       flavor.Id,
			Name:     flavor.Name,
			Arches:   ic.Arches,
			Mem:      uint64(flavor.RAM),
			CpuCores: uint64(flavor.VCPUs),
		}
		allInstanceTypes = append(allInstanceTypes, instanceType)
	}
	availableTypes, err := environs.GetInstanceTypes(ic, allInstanceTypes, nil)
	if err != nil {
		return nil, err
	}

	// look first in the control bucket and then the public bucket to find the release files containing the
	// metadata for available images.
	var spec *environs.InstanceSpec
	releasesFile := fmt.Sprintf("series-image-metadata/%s/server/released.current.txt", ic.Series)
	r, err := e.Storage().Get(releasesFile)
	if err != nil {
		r, err = e.PublicStorage().Get(releasesFile)
	}
	var br *bufio.Reader
	if err == nil {
		defer r.Close()
		br = bufio.NewReader(r)
	}
	spec, err = environs.FindInstanceSpec(br, ic, availableTypes)
	if err != nil {
		return nil, err
	}
	return spec, nil
}
