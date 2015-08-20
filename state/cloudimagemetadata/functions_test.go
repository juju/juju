// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state/cloudimagemetadata"
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
	// There should be no size in criteria - criteria must be empty.
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataAttributes{RootStorageSize: 0},
		nil)
}

func (s *funcMetadataSuite) TestSearchCriteriaWithSource(c *gc.C) {
	// There should be no source in criteria - criteria must be empty.
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataAttributes{Source: cloudimagemetadata.SourceType("source")},
		nil)
}

func (s *funcMetadataSuite) TestSearchCriteriaAll(c *gc.C) {
	// There should not be any size mentioned in criteria.
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataAttributes{
			RootStorageType: "rootstorage-value",
			RootStorageSize: 0,
			Series:          "series-value",
			Stream:          "stream-value",
			Region:          "region-value",
			Arch:            "arch-value",
			VirtualType:     "vtype-value",
		},
		bson.D{
			{"root_storage_type", "rootstorage-value"},
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

func (s *funcMetadataSuite) TestAddOneToGroupEmpty(c *gc.C) {
	sourceType := cloudimagemetadata.SourceType("source")
	one := cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{
		Source: sourceType,
	}, ""}

	s.assertAddedToGroup(c, one,
		map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata{},
		map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata{
			sourceType: {one},
		})
}

func (s *funcMetadataSuite) TestAddOneToGroup(c *gc.C) {
	sourceType := cloudimagemetadata.SourceType("source")
	one := cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{
		Source: sourceType,
	}, ""}

	existingGroup := map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata{
		sourceType: {one},
	}

	two := cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{
		Source: sourceType,
	}, "22"}

	s.assertAddedToGroup(c, two,
		existingGroup,
		map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata{
			sourceType: {one, two},
		})
}

func (s *funcMetadataSuite) TestAddOneToGroupDiff(c *gc.C) {
	sourceType := cloudimagemetadata.SourceType("source")
	one := cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{
		Source: sourceType,
	}, ""}

	existingGroup := map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata{
		sourceType: {one},
	}

	anotherType := cloudimagemetadata.SourceType("type")
	two := cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{
		Source: anotherType,
	}, "22"}

	s.assertAddedToGroup(c, two,
		existingGroup,
		map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata{
			sourceType:  {one},
			anotherType: {two},
		})
}

func (s *funcMetadataSuite) assertAddedToGroup(c *gc.C,
	one cloudimagemetadata.Metadata,
	groups map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata,
	expected map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata,
) {
	cloudimagemetadata.AddOneToGroup(one, groups)
	c.Assert(groups, jc.DeepEquals, expected)
}
