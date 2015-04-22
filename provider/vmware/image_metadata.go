// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vmware

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
)

type OvfFileMetadata struct {
	Url      string
	Arch     string `json:"arch"`
	Size     int    `json:"size"`
	Path     string `json:"path"`
	FileType string `json:"ftype"`
	Sha256   string `json:"sha256"`
	Md5      string `json:"md5"`
}

func init() {
	simplestreams.RegisterStructTags(OvfFileMetadata{})
}

var findImageMetadata = func(env *environ, args environs.StartInstanceParams) (*OvfFileMetadata, error) {
	arches := args.Tools.Arches()
	series := args.Tools.OneSeries()
	ic := &imagemetadata.ImageConstraint{
		simplestreams.LookupParams{
			Series: []string{series},
			Arches: arches,
		},
	}
	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, errors.Trace(err)
	}

	matchingImages, err := imageMetadataFetch(sources, ic)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(matchingImages) == 0 {
		return nil, errors.Errorf("no mathicng images found for given constraints: %v", ic)
	}

	return matchingImages[0], nil
}

func imageMetadataFetch(sources []simplestreams.DataSource, cons *imagemetadata.ImageConstraint) ([]*OvfFileMetadata, error) {
	params := simplestreams.GetMetadataParams{
		StreamsVersion:   imagemetadata.StreamsVersionV1,
		OnlySigned:       false,
		LookupConstraint: cons,
		ValueParams: simplestreams.ValueParams{
			DataType:      "image-downloads",
			FilterFunc:    appendMatchingFunc,
			ValueTemplate: OvfFileMetadata{},
		},
	}
	items, _, err := simplestreams.GetMetadata(sources, params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	metadata := make([]*OvfFileMetadata, len(items))
	for i, md := range items {
		metadata[i] = md.(*OvfFileMetadata)
	}
	return metadata, nil
}

func appendMatchingFunc(source simplestreams.DataSource, matchingImages []interface{},
	images map[string]interface{}, cons simplestreams.LookupConstraint) []interface{} {

	for _, val := range images {
		file := val.(*OvfFileMetadata)
		if file.FileType == "ovf" {
			//ignore error for url data source
			url, _ := source.URL(file.Path)
			file.Url = url
			matchingImages = append(matchingImages, file)
		}
	}
	return matchingImages
}
