// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/environs/simplestreams"
)

// ValidateImageMetadata attempts to load image metadata for the specified cloud attributes and stream
// and returns any image ids found, or an error if the metadata could not be loaded.
func ValidateImageMetadata(ctx context.Context, fetcher SimplestreamsFetcher, params *simplestreams.MetadataLookupParams) ([]string, *simplestreams.ResolveInfo, error) {
	if params.Release == "" {
		return nil, nil, fmt.Errorf("required parameter series not specified")
	}
	if params.Region == "" {
		return nil, nil, fmt.Errorf("required parameter region not specified")
	}
	if params.Endpoint == "" {
		return nil, nil, fmt.Errorf("required parameter endpoint not specified")
	}
	if len(params.Sources) == 0 {
		return nil, nil, fmt.Errorf("required parameter sources not specified")
	}
	imageConstraint, err := NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{
			Region:   params.Region,
			Endpoint: params.Endpoint,
		},
		Releases: []string{params.Release},
		Arches:   params.Architectures,
		Stream:   params.Stream,
	})
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	matchingImages, resolveInfo, err := Fetch(ctx, fetcher, params.Sources, imageConstraint)
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
