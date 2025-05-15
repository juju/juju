// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"io"
	"path"
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/internal/testhelpers"
)

// PatchOfficialDataSources is used by tests.
// We replace one of the urls with the supplied value
// and prevent the other from being used.
func PatchOfficialDataSources(s *testhelpers.CleanupSuite, url string) {
	s.PatchValue(&imagemetadata.DefaultUbuntuBaseURL, url)
}

// ParseMetadataFromDir loads ImageMetadata from the specified directory.
func ParseMetadataFromDir(c *tc.C, metadataDir string) []*imagemetadata.ImageMetadata {
	stor, err := filestorage.NewFileStorageReader(metadataDir)
	c.Assert(err, tc.ErrorIsNil)
	return ParseMetadataFromStorage(c, stor)
}

// ParseIndexMetadataFromStorage loads Indices from the specified storage reader.
func ParseIndexMetadataFromStorage(c *tc.C, stor storage.StorageReader) (*simplestreams.IndexMetadata, simplestreams.DataSource) {
	source := storage.NewStorageSimpleStreamsDataSource("test storage reader", stor, "images", simplestreams.DEFAULT_CLOUD_DATA, false)

	// Find the simplestreams index file.
	params := simplestreams.ValueParams{
		DataType:      "image-ids",
		ValueTemplate: imagemetadata.ImageMetadata{},
	}
	const requireSigned = false
	indexPath := simplestreams.UnsignedIndex("v1", 1)
	mirrorsPath := simplestreams.MirrorsPath("v1")

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	indexRef, err := ss.GetIndexWithFormat(
		c.Context(),
		source, indexPath, "index:1.0", mirrorsPath, requireSigned, simplestreams.CloudSpec{}, params)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(indexRef.Indexes, tc.HasLen, 1)

	imageIndexMetadata := indexRef.Indexes["com.ubuntu.cloud:custom"]
	c.Assert(imageIndexMetadata, tc.NotNil)
	return imageIndexMetadata, source
}

// ParseMetadataFromStorage loads ImageMetadata from the specified storage reader.
func ParseMetadataFromStorage(c *tc.C, stor storage.StorageReader) []*imagemetadata.ImageMetadata {
	imageIndexMetadata, source := ParseIndexMetadataFromStorage(c, stor)
	c.Assert(imageIndexMetadata, tc.NotNil)

	// Read the products file contents.
	r, err := stor.Get(path.Join("images", imageIndexMetadata.ProductsFilePath))
	defer func() { _ = r.Close() }()
	c.Assert(err, tc.ErrorIsNil)
	data, err := io.ReadAll(r)
	c.Assert(err, tc.ErrorIsNil)

	// Parse the products file metadata.
	url, err := source.URL(imageIndexMetadata.ProductsFilePath)
	c.Assert(err, tc.ErrorIsNil)
	cloudMetadata, err := simplestreams.ParseCloudMetadata(data, "products:1.0", url, imagemetadata.ImageMetadata{})
	c.Assert(err, tc.ErrorIsNil)

	// Collate the metadata.
	imageMetadataMap := make(map[string]*imagemetadata.ImageMetadata)
	expectedProductIds, imageVersions := make(set.Strings), make(set.Strings)
	for _, mc := range cloudMetadata.Products {
		for _, items := range mc.Items {
			for key, item := range items.Items {
				imageMetadata := item.(*imagemetadata.ImageMetadata)
				imageMetadataMap[key] = imageMetadata
				imageVersions.Add(key)
				productId := fmt.Sprintf("com.ubuntu.cloud:server:%s:%s", mc.Version, imageMetadata.Arch)
				expectedProductIds.Add(productId)
			}
		}
	}

	// Make sure index's product IDs are all represented in the products metadata.
	sort.Strings(imageIndexMetadata.ProductIds)
	c.Assert(imageIndexMetadata.ProductIds, tc.DeepEquals, expectedProductIds.SortedValues())

	imageMetadata := make([]*imagemetadata.ImageMetadata, len(imageMetadataMap))
	for i, key := range imageVersions.SortedValues() {
		imageMetadata[i] = imageMetadataMap[key]
	}
	return imageMetadata
}
