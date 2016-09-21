// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type CloudImageMetadataSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&CloudImageMetadataSerializationSuite{})

func (s *CloudImageMetadataSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "cloudimagemetadata"
	s.sliceName = "cloudimagemetadata"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importCloudImageMetadata(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["cloudimagemetadata"] = []interface{}{}
	}
}

func (s *CloudImageMetadataSerializationSuite) TestNewCloudImageMetadata(c *gc.C) {
	storageSize := uint64(3)
	args := CloudImageMetadataArgs{
		Stream:          "stream",
		Region:          "region-test",
		Version:         "14.04",
		Series:          "trusty",
		Arch:            "arch",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test",
		RootStorageSize: &storageSize,
		Source:          "test",
		Priority:        0,
		ImageId:         "foo",
		DateCreated:     0,
	}
	metadata := newCloudImageMetadata(args)
	c.Check(metadata.Stream(), gc.Equals, args.Stream)
	c.Check(metadata.Region(), gc.Equals, args.Region)
	c.Check(metadata.Version(), gc.Equals, args.Version)
	c.Check(metadata.Series(), gc.Equals, args.Series)
	c.Check(metadata.Arch(), gc.Equals, args.Arch)
	c.Check(metadata.VirtType(), gc.Equals, args.VirtType)
	c.Check(metadata.RootStorageType(), gc.Equals, args.RootStorageType)
	value, ok := metadata.RootStorageSize()
	c.Check(ok, jc.IsTrue)
	c.Check(value, gc.Equals, *args.RootStorageSize)
	c.Check(metadata.Source(), gc.Equals, args.Source)
	c.Check(metadata.Priority(), gc.Equals, args.Priority)
	c.Check(metadata.ImageId(), gc.Equals, args.ImageId)
	c.Check(metadata.DateCreated(), gc.Equals, args.DateCreated)
}

func (s *CloudImageMetadataSerializationSuite) TestParsingSerializedData(c *gc.C) {
	storageSize := uint64(3)
	initial := cloudimagemetadataset{
		Version: 1,
		CloudImageMetadata_: []*cloudimagemetadata{
			newCloudImageMetadata(CloudImageMetadataArgs{
				Stream:          "stream",
				Region:          "region-test",
				Version:         "14.04",
				Series:          "trusty",
				Arch:            "arch",
				VirtType:        "virtType-test",
				RootStorageType: "rootStorageType-test",
				RootStorageSize: &storageSize,
				Source:          "test",
				Priority:        0,
				ImageId:         "foo",
				DateCreated:     0,
			}),
			newCloudImageMetadata(CloudImageMetadataArgs{
				Stream:  "stream",
				Region:  "region-test",
				Version: "14.04",
			}),
		},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	metadata, err := importCloudImageMetadata(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(metadata, jc.DeepEquals, initial.CloudImageMetadata_)
}
