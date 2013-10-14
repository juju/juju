// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"bytes"
	"time"

	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/errors"
)

func WriteMetadata(series string, metadata []*ImageMetadata, cloudSpec *simplestreams.CloudSpec, metadataStore storage.Storage) error {
	seriesVersion, err := simplestreams.SeriesVersion(series)
	if err != nil {
		return err
	}
	existingMetadata, err := readExistingMetadata(cloudSpec, metadataStore)
	if err != nil {
		return err
	}
	toWrite, err := mergeMetadata(seriesVersion, metadata, existingMetadata)
	metadataInfo, err := generateMetadata(series, toWrite, cloudSpec)
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

// readExistingMetadata reads the image metadata for cloudSpec from metadataStore.
func readExistingMetadata(cloudSpec *simplestreams.CloudSpec, metadataStore storage.Storage) ([]*ImageMetadata, error) {
	// Read any existing metadata so we can merge the new tools metadata with what's there.
	dataSource := storage.NewStorageSimpleStreamsDataSource(metadataStore, "")
	imageConstraint := NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: *cloudSpec,
	})
	existingMetadata, err := Fetch(
		[]simplestreams.DataSource{dataSource}, simplestreams.DefaultIndexPath, imageConstraint, false)
	if err != nil && !errors.IsNotFoundError(err) {
		return nil, err
	}
	return existingMetadata, nil
}

// mergeMetadata merges the newMetadata into existingMetadata, overwriting existing matching image records.
func mergeMetadata(seriesVersion string, newMetadata, existingMetadata []*ImageMetadata) ([]*ImageMetadata, error) {
	var toWrite = make([]*ImageMetadata, len(newMetadata))
	imageIds := make(map[string]bool)
	for i, im := range newMetadata {
		newRecord := *im
		newRecord.Version = seriesVersion
		toWrite[i] = &newRecord
		id, err := newRecord.productId()
		if err != nil {
			return nil, err
		}
		imageIds[id] = true
	}
	for _, im := range existingMetadata {
		id, err := im.productId()
		if err != nil {
			return nil, err
		}
		if _, ok := imageIds[id]; ok {
			continue
		}
		toWrite = append(toWrite, im)
	}
	return toWrite, nil
}

type MetadataFile struct {
	Path string
	Data []byte
}

// generateMetadata generates some basic simplestreams metadata using the specified cloud and image details.
func generateMetadata(series string, metadata []*ImageMetadata, cloudSpec *simplestreams.CloudSpec) ([]MetadataFile, error) {
	for _, im := range metadata {
		im.RegionName = cloudSpec.Region
		im.Endpoint = cloudSpec.Endpoint
	}
	index, products, err := MarshalImageMetadataJSON(metadata, cloudSpec, time.Now())
	if err != nil {
		return nil, err
	}
	objects := []MetadataFile{
		{simplestreams.UnsignedIndex, index},
		{ProductMetadataPath, products},
	}
	return objects, nil
}
