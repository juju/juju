// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata_test

import (
	"fmt"

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
	s.assertSearchCriteriaBuilt(c, cloudimagemetadata.MetadataFilter{}, nil)
}

func (s *funcMetadataSuite) TestSearchCriteriaWithStream(c *gc.C) {
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataFilter{Stream: "stream-value"},
		bson.D{{"stream", "stream-value"}})
}

func (s *funcMetadataSuite) TestSearchCriteriaWithRegion(c *gc.C) {
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataFilter{Region: "region-value"},
		bson.D{{"region", "region-value"}})
}

func (s *funcMetadataSuite) TestSearchCriteriaWithSeries(c *gc.C) {
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataFilter{Series: []string{"series-value"}},
		bson.D{{"series", bson.D{{"$in", []string{"series-value"}}}}})
}

func (s *funcMetadataSuite) TestSearchCriteriaWithArch(c *gc.C) {
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataFilter{Arches: []string{"arch-value"}},
		bson.D{{"arch", bson.D{{"$in", []string{"arch-value"}}}}})
}

func (s *funcMetadataSuite) TestSearchCriteriaWithVirtualType(c *gc.C) {
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataFilter{VirtualType: "vtype-value"},
		bson.D{{"virtual_type", "vtype-value"}})
}

func (s *funcMetadataSuite) TestSearchCriteriaWithStorageType(c *gc.C) {
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataFilter{RootStorageType: "rootstorage-value"},
		bson.D{{"root_storage_type", "rootstorage-value"}})
}

func (s *funcMetadataSuite) TestSearchCriteriaAll(c *gc.C) {
	// There should not be any size mentioned in criteria.
	s.assertSearchCriteriaBuilt(c,
		cloudimagemetadata.MetadataFilter{
			RootStorageType: "rootstorage-value",
			Series:          []string{"series-value", "series-value-two"},
			Stream:          "stream-value",
			Region:          "region-value",
			Arches:          []string{"arch-value", "arch-value-two"},
			VirtualType:     "vtype-value",
		},
		// This is in order in which it is built.
		bson.D{
			{"stream", "stream-value"},
			{"region", "region-value"},
			{"series", bson.D{{"$in", []string{"series-value", "series-value-two"}}}},
			{"arch", bson.D{{"$in", []string{"arch-value", "arch-value-two"}}}},
			{"virtual_type", "vtype-value"},
			{"root_storage_type", "rootstorage-value"},
		})
}

func (s *funcMetadataSuite) assertSearchCriteriaBuilt(c *gc.C,
	criteria cloudimagemetadata.MetadataFilter,
	expected bson.D,
) {
	clause := cloudimagemetadata.BuildSearchClauses(criteria)
	c.Assert(fmt.Sprintf("%s", clause), jc.DeepEquals, fmt.Sprintf("%s", expected))
}
