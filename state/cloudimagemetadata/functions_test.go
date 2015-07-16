// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/testing"
)

type funcMetadataSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&funcMetadataSuite{})

func (s *funcMetadataSuite) TestSearchEmptyCriteria(c *gc.C) {
	s.assertSearchCriteriaBuilt(c, cloudimagemetadata.MetadataAttributes{}, nil)
}

func (s *funcMetadataSuite) TestSearchCriteriaWithStream(c *gc.C) {
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataAttributes{Stream: "stream-value"},
		bson.D{{"stream", "stream-value"}})
}

func (s *funcMetadataSuite) TestSearchCriteriaWithRegion(c *gc.C) {
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataAttributes{Region: "region-value"},
		bson.D{{"region", "region-value"}})
}

func (s *funcMetadataSuite) TestSearchCriteriaWithSeries(c *gc.C) {
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataAttributes{Series: "series-value"},
		bson.D{{"series", "series-value"}})
}

func (s *funcMetadataSuite) TestSearchCriteriaWithArch(c *gc.C) {
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataAttributes{Arch: "arch-value"},
		bson.D{{"arch", "arch-value"}})
}

func (s *funcMetadataSuite) TestSearchCriteriaWithVirtualType(c *gc.C) {
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataAttributes{VirtualType: "vtype-value"},
		bson.D{{"virtual_type", "vtype-value"}})
}

func (s *funcMetadataSuite) TestSearchCriteriaWithStorageType(c *gc.C) {
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataAttributes{RootStorageType: "rootstorage-value"},
		bson.D{{"root_storage_type", "rootstorage-value"}})
}

func (s *funcMetadataSuite) TestSearchCriteriaWithStorageSize(c *gc.C) {
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataAttributes{RootStorageSize: "rootstorage-value"},
		bson.D{{"root_storage_size", "rootstorage-value"}})
}

func (s *funcMetadataSuite) TestSearchCriteriaAll(c *gc.C) {
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataAttributes{
			RootStorageType: "rootstorage-value",
			RootStorageSize: "rootstorage-value",
			Series:          "series-value",
			Stream:          "stream-value",
			Region:          "region-value",
			Arch:            "arch-value",
			VirtualType:     "vtype-value",
		},
		bson.D{
			{"root_storage_type", "rootstorage-value"},
			{"root_storage_size", "rootstorage-value"},
			{"series", "series-value"},
			{"stream", "stream-value"},
			{"region", "region-value"},
			{"arch", "arch-value"},
			{"virtual_type", "vtype-value"},
		})
}

func (s *funcMetadataSuite) assertSearchCriteriaBuilt(c *gc.C,
	criteria cloudimagemetadata.MetadataAttributes,
	expected bson.D,
) {
	clause := cloudimagemetadata.BuildSearchClauses(criteria)
	c.Assert(clause, jc.SameContents, expected)
}

func (s *funcMetadataSuite) TestAttribitesBothEmpty(c *gc.C) {
	s.assertSameAttributes(c,
		cloudimagemetadata.MetadataAttributes{},
		cloudimagemetadata.MetadataAttributes{})
}

func (s *funcMetadataSuite) TestAttribitesOneEmpty(c *gc.C) {
	s.assertDifferentAttributes(c,
		cloudimagemetadata.MetadataAttributes{},
		cloudimagemetadata.MetadataAttributes{Stream: "a"})
}

func (s *funcMetadataSuite) TestStreamSame(c *gc.C) {
	s.assertSameAttributes(c,
		cloudimagemetadata.MetadataAttributes{Stream: "a"},
		cloudimagemetadata.MetadataAttributes{Stream: "a"})
}

func (s *funcMetadataSuite) TestStreamDifferent(c *gc.C) {
	s.assertDifferentAttributes(c,
		cloudimagemetadata.MetadataAttributes{Stream: "a"},
		cloudimagemetadata.MetadataAttributes{Stream: "b"})
}

func (s *funcMetadataSuite) TestRegionSame(c *gc.C) {
	s.assertSameAttributes(c,
		cloudimagemetadata.MetadataAttributes{Region: "a"},
		cloudimagemetadata.MetadataAttributes{Region: "a"})
}

func (s *funcMetadataSuite) TestRegionDifferent(c *gc.C) {
	s.assertDifferentAttributes(c,
		cloudimagemetadata.MetadataAttributes{Region: "a"},
		cloudimagemetadata.MetadataAttributes{Region: "b"})
}

func (s *funcMetadataSuite) TestArchSame(c *gc.C) {
	s.assertSameAttributes(c,
		cloudimagemetadata.MetadataAttributes{Arch: "a"},
		cloudimagemetadata.MetadataAttributes{Arch: "a"})
}

func (s *funcMetadataSuite) TestArchDifferent(c *gc.C) {
	s.assertDifferentAttributes(c,
		cloudimagemetadata.MetadataAttributes{Arch: "a"},
		cloudimagemetadata.MetadataAttributes{Arch: "b"})
}

func (s *funcMetadataSuite) TestVirtualTypeSame(c *gc.C) {
	s.assertSameAttributes(c,
		cloudimagemetadata.MetadataAttributes{VirtualType: "a"},
		cloudimagemetadata.MetadataAttributes{VirtualType: "a"})
}

func (s *funcMetadataSuite) TestVirtualTypeDifferent(c *gc.C) {
	s.assertDifferentAttributes(c,
		cloudimagemetadata.MetadataAttributes{VirtualType: "a"},
		cloudimagemetadata.MetadataAttributes{VirtualType: "b"})
}

func (s *funcMetadataSuite) TestRootStorageTypeSame(c *gc.C) {
	s.assertSameAttributes(c,
		cloudimagemetadata.MetadataAttributes{RootStorageType: "a"},
		cloudimagemetadata.MetadataAttributes{RootStorageType: "a"})
}

func (s *funcMetadataSuite) TestRootStorageTypeDifferent(c *gc.C) {
	s.assertDifferentAttributes(c,
		cloudimagemetadata.MetadataAttributes{RootStorageType: "a"},
		cloudimagemetadata.MetadataAttributes{RootStorageType: "b"})
}

func (s *funcMetadataSuite) TestRootStorageSizeSame(c *gc.C) {
	s.assertSameAttributes(c,
		cloudimagemetadata.MetadataAttributes{RootStorageSize: "a"},
		cloudimagemetadata.MetadataAttributes{RootStorageSize: "a"})
}

func (s *funcMetadataSuite) TestRootStorageSizeDifferent(c *gc.C) {
	s.assertDifferentAttributes(c,
		cloudimagemetadata.MetadataAttributes{RootStorageSize: "a"},
		cloudimagemetadata.MetadataAttributes{RootStorageSize: "b"})
}

func (s *funcMetadataSuite) TestAllSame(c *gc.C) {
	s.assertSameAttributes(c,
		cloudimagemetadata.MetadataAttributes{
			RootStorageType: "a",
			RootStorageSize: "a",
			VirtualType:     "a",
			Arch:            "a",
			Region:          "a",
			Stream:          "a",
		},
		cloudimagemetadata.MetadataAttributes{
			RootStorageType: "a",
			RootStorageSize: "a",
			VirtualType:     "a",
			Arch:            "a",
			Region:          "a",
			Stream:          "a",
		})
}

func (s *funcMetadataSuite) TestOneDifferent(c *gc.C) {
	s.assertDifferentAttributes(c,
		cloudimagemetadata.MetadataAttributes{
			RootStorageType: "a",
			RootStorageSize: "a",
			VirtualType:     "a",
			Arch:            "a",
			Region:          "a",
			Stream:          "a",
		},
		cloudimagemetadata.MetadataAttributes{
			RootStorageType: "a",
			RootStorageSize: "b",
			VirtualType:     "a",
			Arch:            "a",
			Region:          "a",
			Stream:          "a",
		})
}

func (s *funcMetadataSuite) TestAllDifferent(c *gc.C) {
	s.assertDifferentAttributes(c,
		cloudimagemetadata.MetadataAttributes{
			RootStorageType: "a",
			RootStorageSize: "a",
			VirtualType:     "a",
			Arch:            "a",
			Region:          "a",
			Stream:          "a",
		},
		cloudimagemetadata.MetadataAttributes{
			RootStorageType: "b",
			RootStorageSize: "b",
			VirtualType:     "b",
			Arch:            "b",
			Region:          "b",
			Stream:          "b",
		})
}

func (s *funcMetadataSuite) assertSameAttributes(c *gc.C,
	a, b cloudimagemetadata.MetadataAttributes,
) {
	same := cloudimagemetadata.AreSameAttributes(a, b)
	c.Assert(same, jc.IsTrue)
}

func (s *funcMetadataSuite) assertDifferentAttributes(c *gc.C,
	a, b cloudimagemetadata.MetadataAttributes,
) {
	same := cloudimagemetadata.AreSameAttributes(a, b)
	c.Assert(same, jc.IsFalse)
}

func (s *funcMetadataSuite) TestEmptyMetadataSame(c *gc.C) {
	s.assertSameMetadata(c,
		cloudimagemetadata.Metadata{},
		cloudimagemetadata.Metadata{})
}

func (s *funcMetadataSuite) TestOneEmptyMetadataDifferent(c *gc.C) {
	s.assertDifferentMetadata(c,
		cloudimagemetadata.Metadata{},
		cloudimagemetadata.Metadata{ImageId: "a"})
}

func (s *funcMetadataSuite) TestMetadataImageSame(c *gc.C) {
	s.assertSameMetadata(c,
		cloudimagemetadata.Metadata{ImageId: "a"},
		cloudimagemetadata.Metadata{ImageId: "a"})
}

func (s *funcMetadataSuite) TestMetadataImageDifferent(c *gc.C) {
	s.assertDifferentMetadata(c,
		cloudimagemetadata.Metadata{ImageId: "a"},
		cloudimagemetadata.Metadata{ImageId: "b"})
}

func (s *funcMetadataSuite) TestAttributesAndImageSame(c *gc.C) {
	s.assertSameMetadata(c,
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "a"}, "a"},
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "a"}, "a"})
}

func (s *funcMetadataSuite) TestAttributesSameImageDifferent(c *gc.C) {
	s.assertDifferentMetadata(c,
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "a"}, "a"},
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "a"}, "b"})
}

func (s *funcMetadataSuite) TestAttributesDifferentImageSame(c *gc.C) {
	s.assertDifferentMetadata(c,
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "a"}, "a"},
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "b"}, "a"})
}

func (s *funcMetadataSuite) TestAttributesDifferentImageDifferent(c *gc.C) {
	s.assertDifferentMetadata(c,
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "a"}, "a"},
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "b"}, "b"})
}

func (s *funcMetadataSuite) assertSameMetadata(c *gc.C, a, b cloudimagemetadata.Metadata) {
	same := cloudimagemetadata.IsSameMetadata(a, b)
	c.Assert(same, jc.IsTrue)
}

func (s *funcMetadataSuite) assertDifferentMetadata(c *gc.C, a, b cloudimagemetadata.Metadata) {
	same := cloudimagemetadata.IsSameMetadata(a, b)
	c.Assert(same, jc.IsFalse)
}
