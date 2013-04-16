package openstack

import (
	"fmt"
	"launchpad.net/juju-core/environs"
	"bufio"
)

// instanceConstraint constrains the possible instances that may be
// chosen by the Openstack provider.
type instanceConstraint struct {
	environs.InstanceConstraint
	flavor string
}

func findInstanceSpec(e *environ, constraint *instanceConstraint) (*environs.InstanceSpec, error) {
	nova := e.nova()
	flavors, err := nova.ListFlavorsDetail()
	if err != nil {
		return nil, err
	}
	var defaultInstanceType *environs.InstanceType
	allInstanceTypes := []environs.InstanceType{}
	for _, flavor := range flavors {
		allInstanceTypes = append(allInstanceTypes, environs.InstanceType{
				Id: flavor.Id,
				Name: flavor.Name,
				Arches: constraint.Arches,
				Mem: uint64(flavor.RAM),
				CpuCores: uint64(flavor.VCPUs),
				CpuPower: 100,
			})
		if constraint.flavor == "" || flavor.Name == constraint.flavor {
			defaultInstanceType = &environs.InstanceType{
				Id: flavor.Id,
				Name: flavor.Name,
				Arches: constraint.Arches,
				Mem: uint64(flavor.RAM),
				CpuCores: uint64(flavor.VCPUs),
				CpuPower: 100,
			}
		}
	}
	if len(allInstanceTypes) == 0 {
		return nil, environs.NotFoundError{fmt.Errorf("No such flavor %s", constraint.flavor)}
	}
	availableTypes, err := environs.GetInstanceTypes(constraint.Region, constraint.Constraints, allInstanceTypes, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot get instance types for %q: %v", constraint.Series, err)
	}
	if len(availableTypes) == 0 && defaultInstanceType != nil {
		availableTypes = []environs.InstanceType{*defaultInstanceType}
	}

	var spec *environs.InstanceSpec
	releasesFile := fmt.Sprintf("series-image-metadata/%s/server/released.current.txt", constraint.Series)
	r, err := e.Storage().Get(releasesFile)
	if err != nil {
		r, err = e.PublicStorage().Get(releasesFile)
	}
	if err == nil {
		defer r.Close()
		br := bufio.NewReader(r)
		spec, err = environs.FindInstanceSpec(br, &constraint.InstanceConstraint, availableTypes)
	}
	if err != nil {
		imageId := e.ecfg().defaultImageId()
		if imageId == "" {
			return nil, fmt.Errorf("Unable to find image for series/arch/region %s/%s/%s and no default specified.",
				constraint.Series, constraint.Arches[0], constraint.Region)
		}
		spec = &environs.InstanceSpec{
			availableTypes[0].Id, availableTypes[0].Name,
			environs.Image{imageId, constraint.Arches[0], false},
		}
	}
	return spec, nil
}
