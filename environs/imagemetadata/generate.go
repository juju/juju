// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"bytes"
	"time"

	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
)

func WriteMetadata(series string, im *ImageMetadata, cloudSpec *simplestreams.CloudSpec, metadataStore storage.Storage) error {
	metadataInfo, err := generateMetadata(series, im, cloudSpec)
	if err != nil {
		return err
	}
	for _, md := range metadataInfo {
		err = metadataStore.Put(md.Path, bytes.NewReader(md.Data), int64(len(md.Data)))
		if err != nil {
			return err
		}
	}
	return nil
}

type MetadataFile struct {
	Path string
	Data []byte
}

// generateMetadata generates some basic simplestreams metadata using the specified cloud and image details.
func generateMetadata(series string, im *ImageMetadata, cloudSpec *simplestreams.CloudSpec) ([]MetadataFile, error) {
	metadata := &ImageMetadata{
		Id:         im.Id,
		Arch:       im.Arch,
		Release:    series,
		RegionName: cloudSpec.Region,
		Endpoint:   cloudSpec.Endpoint,
	}

	index, products, err := MarshalImageMetadataJSON([]*ImageMetadata{metadata}, cloudSpec, time.Now())
	if err != nil {
		return nil, err
	}
	objects := []MetadataFile{
		{simplestreams.UnsignedIndex, index},
		{ProductMetadataPath, products},
	}
	return objects, nil
}
