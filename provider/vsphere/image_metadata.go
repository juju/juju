// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
)

/*
Vmware provider use "image-download" data type for simplestream. That's why we use custom implementation of imagemetadata.Fetch function.
We also use custom struct OvfFileMetadata that corresponds to the format used in "image-downloads" simplestream datatype.
Also we use custom append function to filter content of the stream and keep only items, that have ova FileType
*/

type OvaFileMetadata struct {
	URL      string
	Arch     string `json:"arch"`
	Size     int    `json:"size"`
	Path     string `json:"path"`
	FileType string `json:"ftype"`
	Sha256   string `json:"sha256"`
	Md5      string `json:"md5"`
}

func init() {
	simplestreams.RegisterStructTags(OvaFileMetadata{})
}

func findImageMetadata(ctx context.Context, env environs.Environ, arch string, series string) (*OvaFileMetadata, error) {
	vers, err := imagemetadata.ImageRelease(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ic := &imagemetadata.ImageConstraint{
		LookupParams: simplestreams.LookupParams{
			Releases: []string{vers},
			Arches:   []string{arch},
			Stream:   env.Config().ImageStream(),
		},
	}
	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	sources, err := environs.ImageMetadataSources(env, ss)
	if err != nil {
		return nil, errors.Trace(err)
	}

	matchingImages, err := imageMetadataFetch(ctx, sources, ic)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(matchingImages) == 0 {
		return nil, errors.Errorf("no matching images found for given constraints: %v", ic)
	}

	return matchingImages[0], nil
}

func imageMetadataFetch(ctx context.Context, sources []simplestreams.DataSource, cons *imagemetadata.ImageConstraint) ([]*OvaFileMetadata, error) {
	params := simplestreams.GetMetadataParams{
		StreamsVersion:   imagemetadata.StreamsVersionV1,
		LookupConstraint: cons,
		ValueParams: simplestreams.ValueParams{
			DataType:      "image-downloads",
			FilterFunc:    appendMatchingFunc,
			ValueTemplate: OvaFileMetadata{},
		},
	}
	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	items, _, err := ss.GetMetadata(ctx, sources, params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	metadata := make([]*OvaFileMetadata, len(items))
	for i, md := range items {
		metadata[i] = md.(*OvaFileMetadata)
	}
	return metadata, nil
}

func appendMatchingFunc(source simplestreams.DataSource, matchingImages []interface{},
	images map[string]interface{}, cons simplestreams.LookupConstraint) ([]interface{}, error) {

	for _, val := range images {
		file := val.(*OvaFileMetadata)
		if file.FileType == "ova" {
			//ignore error for url data source
			url, _ := source.URL(file.Path)
			file.URL = url
			matchingImages = append(matchingImages, file)
		}
	}
	return matchingImages, nil
}
