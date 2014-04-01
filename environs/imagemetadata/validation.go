// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"fmt"

	"launchpad.net/juju-core/environs/simplestreams"
)

// ValidateImageMetadata attempts to load image metadata for the specified cloud attributes and stream
// and returns any image ids found, or an error if the metadata could not be loaded.
func ValidateImageMetadata(params *simplestreams.MetadataLookupParams) ([]string, *simplestreams.ResolveInfo, error) {
	if params.Series == "" {
		return nil, nil, fmt.Errorf("required parameter series not specified")
	}
	if params.Region == "" {
		return nil, nil, fmt.Errorf("required parameter region not specified")
	}
	if params.Endpoint == "" {
		return nil, nil, fmt.Errorf("required parameter endpoint not specified")
	}
	if len(params.Architectures) == 0 {
		return nil, nil, fmt.Errorf("required parameter arches not specified")
	}
	if len(params.Sources) == 0 {
		return nil, nil, fmt.Errorf("required parameter sources not specified")
	}
	imageConstraint := NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{params.Region, params.Endpoint},
		Series:    []string{params.Series},
		Arches:    params.Architectures,
		Stream:    params.Stream,
	})
	matchingImages, resolveInfo, err := Fetch(params.Sources, simplestreams.DefaultIndexPath, imageConstraint, false)
	if err != nil {
		return nil, resolveInfo, err
	}
	if len(matchingImages) == 0 {
		return nil, resolveInfo, fmt.Errorf("no matching images found for constraint %+v", imageConstraint)
	}
	image_ids := make([]string, len(matchingImages))
	for i, im := range matchingImages {
		image_ids[i] = im.Id
	}
	return image_ids, resolveInfo, nil
}
