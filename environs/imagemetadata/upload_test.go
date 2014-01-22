// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/imagemetadata/testing"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

var _ = gc.Suite(&uploadSuite{})

type uploadSuite struct {
	testbase.LoggingSuite
}

func createImageMetadata(c *gc.C) (sourceDir string, destDir string, destStor storage.Storage, metadata *imagemetadata.ImageMetadata) {
	destDir = c.MkDir()
	var err error
	destStor, err = filestorage.NewFileStorageWriter(destDir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)

	// Generate some metadata.
	sourceDir = c.MkDir()
	im := []*imagemetadata.ImageMetadata{
		{
			Id:      "1234",
			Arch:    "amd64",
			Version: "13.04",
		},
	}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	im[0].RegionName = cloudSpec.Region
	im[0].Endpoint = cloudSpec.Endpoint
	var sourceStor storage.Storage
	sourceStor, err = filestorage.NewFileStorageWriter(sourceDir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
	err = imagemetadata.MergeAndWriteMetadata("raring", im, cloudSpec, sourceStor)
	c.Assert(err, gc.IsNil)
	return sourceDir, destDir, destStor, im[0]
}

func (s *uploadSuite) TestUpload(c *gc.C) {
	// Create some metadata.
	sourceDir, destDir, destStor, im := createImageMetadata(c)

	// Ensure it can be uploaded.
	err := imagemetadata.UploadImageMetadata(destStor, sourceDir)
	c.Assert(err, gc.IsNil)
	metadata := testing.ParseMetadataFromDir(c, destDir)
	c.Assert(metadata, gc.HasLen, 1)
	c.Assert(im, gc.DeepEquals, metadata[0])
}

func (s *uploadSuite) TestUploadErrors(c *gc.C) {
	destDir := c.MkDir()
	destStor, err := filestorage.NewFileStorageWriter(destDir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
	err = imagemetadata.UploadImageMetadata(destStor, "foo")
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *uploadSuite) TestUploadIgnoresNonJsonFiles(c *gc.C) {
	// Create some metadata.
	sourceDir, destDir, destStor, _ := createImageMetadata(c)

	// Add an extra file.
	sourceMetadataPath := filepath.Join(sourceDir, storage.BaseImagesPath, simplestreams.StreamsDir)
	err := ioutil.WriteFile(filepath.Join(sourceMetadataPath, "foo.txt"), []byte("hello"), 0644)
	c.Assert(err, gc.IsNil)

	// Upload the metadata.
	err = imagemetadata.UploadImageMetadata(destStor, sourceDir)
	c.Assert(err, gc.IsNil)

	// Check only json files are uploaded.
	destMetadataPath := filepath.Join(destDir, storage.BaseImagesPath, simplestreams.StreamsDir)
	files, err := ioutil.ReadDir(destMetadataPath)
	c.Assert(err, gc.IsNil)
	c.Assert(files, gc.HasLen, 2)
	for _, f := range files {
		fileName := f.Name()
		c.Assert(strings.HasSuffix(fileName, simplestreams.UnsignedSuffix), jc.IsTrue)
	}
}
