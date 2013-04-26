package openstack

import (
	"fmt"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
)

// instanceSpec specifies a particular kind of instance.
type instanceSpec struct {
	flavorId string
	imageId  string
	tools    *state.Tools
}

func findInstanceSpec(e *environ, possibleTools tools.List) (*instanceSpec, error) {
	// TODO(wallyworld/fwereade) - we want to search for an image based on the
	// series, possible arches, and region, as http://cloud-images.ubuntu.com
	// allows for ec2, but there's nothing to support that yet. Instead, we
	// require that the user configure a default-image-id, and assume/require
	// that it be a precise-amd64 image, on the basis that (1) most charms are
	// precise and (2) we need an amd64 image because we don't currently supply
	// tools for other arches.
	// Thus, we require at this point that tools be available for precise/amd64.
	imageId := e.ecfg().defaultImageId()
	if imageId == "" {
		return nil, fmt.Errorf("no default-image-id set")
	}

	// TODO(wallyworld/fwereade) - we should be using constraints, but we're not;
	// we require that the user enter an default-instance-type appropriate to her
	// cloud.
	flavorName := e.ecfg().defaultInstanceType()
	if flavorName == "" {
		return nil, fmt.Errorf("no default-instance-type set")
	}
	log.Warningf("environs/openstack: ignoring constraints, using default-instance-type flavor %q", flavorName)
	flavors, err := e.nova().ListFlavors()
	if err != nil {
		return nil, err
	}
	var flavorId string
	for _, flavor := range flavors {
		if flavor.Name == flavorName {
			flavorId = flavor.Id
			break
		}
	}
	if flavorId == "" {
		return nil, &environs.NotFoundError{fmt.Errorf("flavor %q not found", flavorName)}
	}

	log.Warningf("environs/openstack: forcing precise/amd64 tools for image %q", imageId)
	possibleTools, err = possibleTools.Match(tools.Filter{
		Arch:   "amd64",
		Series: "precise",
	})
	if err != nil {
		return nil, &environs.NotFoundError{err}
	}
	tools := possibleTools[0]
	return &instanceSpec{
		imageId:  imageId,
		flavorId: flavorId,
		tools:    tools,
	}, nil
}
