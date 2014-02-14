// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"bytes"
	"fmt"
	"path/filepath"
	"time"

	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/errors"
)

// MergeAndWriteMetadata reads the existing metadata from storage (if any),
// and merges it with supplied metadata, writing the resulting metadata is written to storage.
func MergeAndWriteMetadata(series string, metadata []*ImageMetadata, cloudSpec *simplestreams.CloudSpec,
	metadataStore storage.Storage) error {

	existingMetadata, err := readMetadata(metadataStore)
	if err != nil {
		return err
	}
	seriesVersion, err := simplestreams.SeriesVersion(series)
	if err != nil {
		return err
	}
	toWrite, allCloudSpec := mergeMetadata(seriesVersion, cloudSpec, metadata, existingMetadata)
	return writeMetadata(toWrite, allCloudSpec, metadataStore)
}

// readMetadata reads the image metadata from metadataStore.
func readMetadata(metadataStore storage.Storage) ([]*ImageMetadata, error) {
	// Read any existing metadata so we can merge the new tools metadata with what's there.
	dataSource := storage.NewStorageSimpleStreamsDataSource(metadataStore, storage.BaseImagesPath)
	imageConstraint := NewImageConstraint(simplestreams.LookupParams{})
	existingMetadata, err := Fetch(
		[]simplestreams.DataSource{dataSource}, simplestreams.DefaultIndexPath, imageConstraint, false)
	if err != nil && !errors.IsNotFoundError(err) {
		return nil, err
	}
	return existingMetadata, nil
}

func mapKey(im *ImageMetadata) string {
	return fmt.Sprintf("%s-%s", im.productId(), im.RegionName)
}

// mergeMetadata merges the newMetadata into existingMetadata, overwriting existing matching image records.
func mergeMetadata(seriesVersion string, cloudSpec *simplestreams.CloudSpec, newMetadata,
	existingMetadata []*ImageMetadata) ([]*ImageMetadata, []simplestreams.CloudSpec) {

	var toWrite = make([]*ImageMetadata, len(newMetadata))
	imageIds := make(map[string]bool)
	for i, im := range newMetadata {
		newRecord := *im
		newRecord.Version = seriesVersion
		newRecord.RegionName = cloudSpec.Region
		newRecord.Endpoint = cloudSpec.Endpoint
		toWrite[i] = &newRecord
		imageIds[mapKey(&newRecord)] = true
	}
	regions := make(map[string]bool)
	var allCloudSpecs = []simplestreams.CloudSpec{*cloudSpec}
	for _, im := range existingMetadata {
		if _, ok := imageIds[mapKey(im)]; ok {
			continue
		}
		toWrite = append(toWrite, im)
		if _, ok := regions[im.RegionName]; ok {
			continue
		}
		regions[im.RegionName] = true
		existingCloudSpec := simplestreams.CloudSpec{im.RegionName, im.Endpoint}
		allCloudSpecs = append(allCloudSpecs, existingCloudSpec)
	}
	return toWrite, allCloudSpecs
}

type MetadataFile struct {
	Path string
	Data []byte
}

// writeMetadata generates some basic simplestreams metadata using the specified cloud and image details and writes
// it to the supplied store.
func writeMetadata(metadata []*ImageMetadata, cloudSpec []simplestreams.CloudSpec,
	metadataStore storage.Storage) error {

	index, products, err := MarshalImageMetadataJSON(metadata, cloudSpec, time.Now())
	if err != nil {
		return err
	}
	metadataInfo := []MetadataFile{
		{simplestreams.UnsignedIndex, index},
		{ProductMetadataPath, products},
	}
	for _, md := range metadataInfo {
		err = metadataStore.Put(
			filepath.Join(storage.BaseImagesPath, md.Path), bytes.NewReader(md.Data), int64(len(md.Data)))
		if err != nil {
			return err
		}
	}
	return nil
}
