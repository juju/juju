// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"bytes"
	"fmt"
	"path"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/version"
)

// IndexStoragePath returns the storage path for the image metadata index file.
func IndexStoragePath() string {
	return path.Join(storage.BaseImagesPath, simplestreams.UnsignedIndex(currentStreamsVersion, IndexFileVersion))
}

// ProductMetadataStoragePath returns the storage path for the image metadata products file.
func ProductMetadataStoragePath() string {
	return path.Join(storage.BaseImagesPath, ProductMetadataPath)
}

// MergeAndWriteMetadata reads the existing metadata from storage (if any),
// and merges it with supplied metadata, writing the resulting metadata is written to storage.
func MergeAndWriteMetadata(series string, metadata []*ImageMetadata, cloudSpec *simplestreams.CloudSpec,
	metadataStore storage.Storage) error {

	existingMetadata, err := readMetadata(metadataStore)
	if err != nil {
		return err
	}
	seriesVersion, err := version.SeriesVersion(series)
	if err != nil {
		return err
	}
	toWrite, allCloudSpec := mergeMetadata(seriesVersion, cloudSpec, metadata, existingMetadata)
	return writeMetadata(toWrite, allCloudSpec, metadataStore)
}

// readMetadata reads the image metadata from metadataStore.
func readMetadata(metadataStore storage.Storage) ([]*ImageMetadata, error) {
	// Read any existing metadata so we can merge the new tools metadata with what's there.
	dataSource := storage.NewStorageSimpleStreamsDataSource("existing metadata", metadataStore, storage.BaseImagesPath)
	imageConstraint := NewImageConstraint(simplestreams.LookupParams{})
	existingMetadata, _, err := Fetch([]simplestreams.DataSource{dataSource}, imageConstraint, false)
	if err != nil && !errors.IsNotFound(err) {
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
		existingCloudSpec := simplestreams.CloudSpec{
			Region:   im.RegionName,
			Endpoint: im.Endpoint,
		}
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
		{IndexStoragePath(), index},
		{ProductMetadataStoragePath(), products},
	}
	for _, md := range metadataInfo {
		err = metadataStore.Put(md.Path, bytes.NewReader(md.Data), int64(len(md.Data)))
		if err != nil {
			return err
		}
	}
	return nil
}
