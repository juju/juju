// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/utils/series"

	apimetadata "github.com/juju/juju/api/imagemetadata"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/juju"
)

// FindImageMetadata looks for image metadata in state.
// If none are found, we fall back on original image search in simple streams.
func FindImageMetadata(env environs.Environ, imageConstraint *imagemetadata.ImageConstraint, signedOnly bool) ([]*imagemetadata.ImageMetadata, *simplestreams.ResolveInfo, error) {
	stateMetadata, stateInfo, err := imageMetadataFromState(env, imageConstraint, signedOnly)
	if err != nil && !errors.IsNotFound(err) {
		return nil, nil, errors.Trace(err)
	}

	// No need to look in data sources if found in state?
	if len(stateMetadata) != 0 {
		return stateMetadata, stateInfo, nil
	}

	// If none are found, fall back to original simple stream impl.
	// Currently, an image metadata worker picks up this metadata periodically (daily),
	// and stores it in state. So potentially, this collection could be different
	// to what is in state.
	dsMetadata, dsInfo, err := imageMetadataFromDataSources(env, imageConstraint, signedOnly)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, nil, errors.Trace(err)
		}
	}

	// If still none found, complain
	if len(dsMetadata) == 0 {
		return nil, nil, errors.NotFoundf("image metadata for series %v, arch %v", imageConstraint.Series, imageConstraint.Arches)
	}

	return dsMetadata, dsInfo, nil
}

var metadataAPI = func(env environs.Environ) (*apimetadata.Client, error) {
	api, err := juju.NewAPIFromName(env.Config().Name())
	if err != nil {
		return nil, errors.Annotate(err, "could not connect to api")
	}
	return apimetadata.NewClient(api), nil
}

func imageMetadataFromState(env environs.Environ, ic *imagemetadata.ImageConstraint, signedOnly bool) ([]*imagemetadata.ImageMetadata, *simplestreams.ResolveInfo, error) {
	metadataAPI, err := metadataAPI(env)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	stored, err := metadataAPI.List(ic.Stream, ic.Region, ic.Series, ic.Arches, "", "")
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not list image metadata from state server")
	}

	// Convert to common format.
	images := make([]*imagemetadata.ImageMetadata, len(stored))
	for i, one := range stored {
		m := &imagemetadata.ImageMetadata{
			Storage:    one.RootStorageType,
			Id:         one.ImageId,
			VirtType:   one.VirtType,
			Arch:       one.Arch,
			RegionName: one.Region,
			Stream:     one.Stream,
		}
		m.Version, _ = series.SeriesVersion(one.Series)
		images[i] = m
	}

	info := &simplestreams.ResolveInfo{}
	info.Source = "state server"
	// This is currently ignored for image metadata that is stored in state
	// since when stored, both signed and unsigned metadata are written,
	// but whether it was signed or not is not.
	info.Signed = signedOnly
	return images, info, nil
}

// imageMetadataFromDataSources finds image metadata using
// existing data sources.
func imageMetadataFromDataSources(env environs.Environ, constraint *imagemetadata.ImageConstraint, signedOnly bool) ([]*imagemetadata.ImageMetadata, *simplestreams.ResolveInfo, error) {
	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, nil, err
	}
	return imagemetadata.Fetch(sources, constraint, signedOnly)
}
