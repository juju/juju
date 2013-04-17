package openstack

import (
	"bufio"
	"fmt"
	"launchpad.net/juju-core/environs"
)

// instanceConstraint constrains the possible instances that may be chosen by the Openstack provider.
// It adds to the standard instance constraints by allowing for an optional default flavor to be
// specified which is used if no instance types match the supplied contraints.
type instanceConstraint struct {
	environs.InstanceConstraint
	defaultFlavor string
}

// findInstanceSpec returns an image and instance type satisfying the constraint.
// The instance type comes from querying the flavors supported by the deployment.
func findInstanceSpec(e *environ, ic *instanceConstraint) (*environs.InstanceSpec, error) {
	// first construct the available instance types from the supported flavors.
	nova := e.nova()
	flavors, err := nova.ListFlavorsDetail()
	if err != nil {
		return nil, err
	}
	var defaultInstanceType *environs.InstanceType
	allInstanceTypes := []environs.InstanceType{}
	for _, flavor := range flavors {
		allInstanceTypes = append(allInstanceTypes, environs.InstanceType{
			Id:       flavor.Id,
			Name:     flavor.Name,
			Arches:   ic.Arches,
			Mem:      uint64(flavor.RAM),
			CpuCores: uint64(flavor.VCPUs),
		})
		if ic.defaultFlavor == "" || flavor.Name == ic.defaultFlavor {
			defaultInstanceType = &environs.InstanceType{
				Id:       flavor.Id,
				Name:     flavor.Name,
				Arches:   ic.Arches,
				Mem:      uint64(flavor.RAM),
				CpuCores: uint64(flavor.VCPUs),
			}
		}
	}
	if len(allInstanceTypes) == 0 {
		return nil, environs.NotFoundError{fmt.Errorf("no such flavor %s", ic.defaultFlavor)}
	}
	availableTypes, err := environs.GetInstanceTypes(ic.Region, ic.Constraints, allInstanceTypes, nil)
	// if no matching instance types are found, use the default if one is specified.
	if err != nil {
		if defaultInstanceType == nil {
				return nil, err
		}
		availableTypes = []environs.InstanceType{*defaultInstanceType}
	}

	// look first in the control bucket and then the public bucket to find the release files containing the
	// metadata for available images.
	var spec *environs.InstanceSpec
	releasesFile := fmt.Sprintf("series-image-metadata/%s/server/released.current.txt", ic.Series)
	r, err := e.Storage().Get(releasesFile)
	if err != nil {
		r, err = e.PublicStorage().Get(releasesFile)
	}
	if err == nil {
		defer r.Close()
		br := bufio.NewReader(r)
		spec, err = environs.FindInstanceSpec(br, &ic.InstanceConstraint, availableTypes)
	}
	// if no matching image is found for whatever reason, use the default if one is specified.
	if err != nil {
		imageId := e.ecfg().defaultImageId()
		if imageId == "" {
			return nil, fmt.Errorf("unable to find image for series/arch/region %s/%s/%s and no default specified.",
				ic.Series, ic.Arches[0], ic.Region)
		}
		spec = &environs.InstanceSpec{
			availableTypes[0].Id, availableTypes[0].Name,
			environs.Image{imageId, ic.Arches[0], false},
		}
	}
	return spec, nil
}
