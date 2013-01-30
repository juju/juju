package openstack

import (
	"fmt"
	"launchpad.net/juju-core/environs"
)

// instanceConstraint constrains the possible instances that may be
// chosen by the Openstack provider.
type instanceConstraint struct {
	series string // Ubuntu release name.
	arch   string
	region string
	flavor string
}

// instanceSpec specifies a particular kind of instance.
type instanceSpec struct {
	imageId  string
	flavorId string
}

func findInstanceSpec(e *environ, constraint *instanceConstraint) (*instanceSpec, error) {
	nova := e.nova()
	flavors, err := nova.ListFlavors()
	if err != nil {
		return nil, err
	}
	var flavorId string
	for _, flavor := range flavors {
		if flavor.Name == constraint.flavor {
			flavorId = flavor.Id
			break
		}
	}
	if flavorId == "" {
		return nil, environs.NotFoundError{fmt.Errorf("No such flavor %s", constraint.flavor)}
	}
	// TODO(wallyworld) - we want to search for an image based on the series, arch, region like for ec2 providers
	// and http://cloud-images.ubuntu.com but there's nothing to support that yet.
	// So we allow the user to configure a default image Id to use.
	imageId := e.ecfg().defaultImageId()
	if imageId == "" {
		return nil, fmt.Errorf("Unable to find image for series/arch/region %s/%s/%s and no default specified.",
			constraint.series, constraint.arch, constraint.region)
	}
	return &instanceSpec{
		imageId:  imageId,
		flavorId: flavorId,
	}, nil
}
