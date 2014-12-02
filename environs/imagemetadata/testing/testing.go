// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"io/ioutil"
	"path"
	"sort"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
)

// ParseMetadataFromDir loads ImageMetadata from the specified directory.
func ParseMetadataFromDir(c *gc.C, metadataDir string) []*imagemetadata.ImageMetadata {
	stor, err := filestorage.NewFileStorageReader(metadataDir)
	c.Assert(err, jc.ErrorIsNil)
	return ParseMetadataFromStorage(c, stor)
}

// ParseMetadataFromStorage loads ImageMetadata from the specified storage reader.
func ParseMetadataFromStorage(c *gc.C, stor storage.StorageReader) []*imagemetadata.ImageMetadata {
	source := storage.NewStorageSimpleStreamsDataSource("test storage reader", stor, "images")

	// Find the simplestreams index file.
	params := simplestreams.ValueParams{
		DataType:      "image-ids",
		ValueTemplate: imagemetadata.ImageMetadata{},
	}
	const requireSigned = false
	indexPath := simplestreams.UnsignedIndex("v1", 1)
	mirrorsPath := simplestreams.MirrorsPath("v1")
	indexRef, err := simplestreams.GetIndexWithFormat(
		source, indexPath, "index:1.0", mirrorsPath, requireSigned, simplestreams.CloudSpec{}, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(indexRef.Indexes, gc.HasLen, 1)

	imageIndexMetadata := indexRef.Indexes["com.ubuntu.cloud:custom"]
	c.Assert(imageIndexMetadata, gc.NotNil)

	// Read the products file contents.
	r, err := stor.Get(path.Join("images", imageIndexMetadata.ProductsFilePath))
	defer r.Close()
	c.Assert(err, jc.ErrorIsNil)
	data, err := ioutil.ReadAll(r)
	c.Assert(err, jc.ErrorIsNil)

	// Parse the products file metadata.
	url, err := source.URL(imageIndexMetadata.ProductsFilePath)
	c.Assert(err, jc.ErrorIsNil)
	cloudMetadata, err := simplestreams.ParseCloudMetadata(data, "products:1.0", url, imagemetadata.ImageMetadata{})
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(imageIndexMetadata.ProductIds, gc.DeepEquals, expectedProductIds.SortedValues())

	imageMetadata := make([]*imagemetadata.ImageMetadata, len(imageMetadataMap))
	for i, key := range imageVersions.SortedValues() {
		imageMetadata[i] = imageMetadataMap[key]
	}
	return imageMetadata
}
