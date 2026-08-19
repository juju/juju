// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"bytes"
	"fmt"
	"path"
	"time"

	"github.com/juju/errors"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
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
func MergeAndWriteMetadata(fetcher SimplestreamsFetcher,
	base corebase.Base,
	metadata []*ImageMetadata,
	cloudSpec *simplestreams.CloudSpec,
	metadataStore storage.Storage) error {

	existingMetadata, err := readMetadata(fetcher, metadataStore)
	if err != nil {
		return err
	}
	toWrite, allCloudSpec := mergeMetadata(base, cloudSpec, metadata, existingMetadata)
	return writeMetadata(toWrite, allCloudSpec, metadataStore)
}

// readMetadata reads the image metadata from metadataStore.
func readMetadata(fetcher SimplestreamsFetcher, metadataStore storage.Storage) ([]*ImageMetadata, error) {
	// Read any existing metadata so we can merge the new tools metadata with what's there.
	dataSource := storage.NewStorageSimpleStreamsDataSource("existing metadata", metadataStore, storage.BaseImagesPath, simplestreams.EXISTING_CLOUD_DATA, false)
	imageConstraint, err := NewImageConstraint(simplestreams.LookupParams{})
	if err != nil {
		return nil, errors.Trace(err)
	}
	existingMetadata, _, err := Fetch(fetcher, []simplestreams.DataSource{dataSource}, imageConstraint)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	return existingMetadata, nil
}

// mapKey returns a key that uniquely identifies image metadata.
// The metadata for different images may have similar values
// for some parameters. This key ensures that truly distinct
// metadata is not overwritten by closely related ones.
// This key is similar to image metadata key built in state which combines
// parameter values rather than using image id to ensure record uniqueness.
func mapKey(im *ImageMetadata) string {
	return fmt.Sprintf("%s-%s-%s-%s", im.productId(), im.RegionName, im.VirtType, im.Storage)
}

// mergeMetadata merges the newMetadata into existingMetadata, overwriting existing matching image records.
func mergeMetadata(base corebase.Base, cloudSpec *simplestreams.CloudSpec, newMetadata,
	existingMetadata []*ImageMetadata) ([]*ImageMetadata, []simplestreams.CloudSpec) {

	regions := make(map[string]bool)
	var allCloudSpecs = []simplestreams.CloudSpec{}
	// Each metadata item defines its own cloud specification.
	// However, when we combine metadata items in the file, we do not want to
	// repeat common cloud specifications in index definition.
	// Since region name and endpoint have 1:1 correspondence,
	// only one distinct cloud specification for each region
	// is being collected.
	addDistinctCloudSpec := func(im *ImageMetadata) {
		if _, ok := regions[im.RegionName]; !ok {
			regions[im.RegionName] = true
			aCloudSpec := simplestreams.CloudSpec{
				Region:   im.RegionName,
				Endpoint: im.Endpoint,
			}
			allCloudSpecs = append(allCloudSpecs, aCloudSpec)
		}
	}

	var toWrite = make([]*ImageMetadata, len(newMetadata))
	imageIds := make(map[string]bool)
	for i, im := range newMetadata {
		newRecord := *im
		newRecord.Version = base.Channel.Track
		newRecord.RegionName = cloudSpec.Region
		newRecord.Endpoint = cloudSpec.Endpoint
		toWrite[i] = &newRecord
		imageIds[mapKey(&newRecord)] = true
		addDistinctCloudSpec(&newRecord)
	}
	for _, im := range existingMetadata {
		if _, ok := imageIds[mapKey(im)]; !ok {
			toWrite = append(toWrite, im)
			addDistinctCloudSpec(im)
		}
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

	// TODO(perrito666) 2016-05-02 lp:1558657
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
