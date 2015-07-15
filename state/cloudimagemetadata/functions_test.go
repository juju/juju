// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/testing"
)

type funcMetadataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&funcMetadataSuite{})

func (s *funcMetadataSuite) TestSearchCriteria(c *gc.C) {
	testData := []struct {
		about    string
		criteria cloudimagemetadata.MetadataAttributes
		expected bson.D
	}{{
		`empty criteria`,
		cloudimagemetadata.MetadataAttributes{},
		nil,
	}, {
		`only stream supplied`,
		cloudimagemetadata.MetadataAttributes{Stream: "stream-value"},
		bson.D{{"stream", "stream-value"}},
	}, {
		`only region supplied`,
		cloudimagemetadata.MetadataAttributes{Region: "region-value"},
		bson.D{{"region", "region-value"}},
	}, {
		`only series supplied`,
		cloudimagemetadata.MetadataAttributes{Series: "series-value"},
		bson.D{{"series", "series-value"}},
	}, {
		`only arch supplied`,
		cloudimagemetadata.MetadataAttributes{Arch: "arch-value"},
		bson.D{{"arch", "arch-value"}},
	}, {
		`only virtual type supplied`,
		cloudimagemetadata.MetadataAttributes{VirtualType: "vtype-value"},
		bson.D{{"virtual_type", "vtype-value"}},
	}, {
		`only root storage type supplied`,
		cloudimagemetadata.MetadataAttributes{RootStorageType: "rootstorage-value"},
		bson.D{{"root_storage_type", "rootstorage-value"}},
	}, {
		`only root storage size supplied`,
		cloudimagemetadata.MetadataAttributes{RootStorageSize: "rootstorage-value"},
		bson.D{{"root_storage_size", "rootstorage-value"}},
	}, {
		`two search criteria supplied`,
		cloudimagemetadata.MetadataAttributes{RootStorageType: "rootstorage-value", Series: "series-value"},
		bson.D{{"root_storage_type", "rootstorage-value"}, {"series", "series-value"}},
	}, {
		`all serach criteria supplied`,
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
		},
	}}

	for i, t := range testData {
		c.Logf("%d: %v", i, t.about)
		clause := cloudimagemetadata.BuildSearchClauses(t.criteria)
		c.Assert(clause, jc.SameContents, t.expected)
	}
}

func (s *funcMetadataSuite) TestSameMetadataAttributesFunc(c *gc.C) {
	testData := []struct {
		about    string
		a        cloudimagemetadata.MetadataAttributes
		b        cloudimagemetadata.MetadataAttributes
		expected bool
	}{{
		`both attributes empty`,
		cloudimagemetadata.MetadataAttributes{},
		cloudimagemetadata.MetadataAttributes{},
		true,
	}, {
		`one empty`,
		cloudimagemetadata.MetadataAttributes{},
		cloudimagemetadata.MetadataAttributes{Stream: "a"},
		false,
	}, {
		`attrs: stream same`,
		cloudimagemetadata.MetadataAttributes{Stream: "a"},
		cloudimagemetadata.MetadataAttributes{Stream: "a"},
		true,
	}, {
		`attrs: stream different`,
		cloudimagemetadata.MetadataAttributes{Stream: "a"},
		cloudimagemetadata.MetadataAttributes{Stream: "b"},
		false,
	}, {
		`attrs: region same`,
		cloudimagemetadata.MetadataAttributes{Region: "a"},
		cloudimagemetadata.MetadataAttributes{Region: "a"},
		true,
	}, {
		`attrs: Region different`,
		cloudimagemetadata.MetadataAttributes{Region: "a"},
		cloudimagemetadata.MetadataAttributes{Region: "b"},
		false,
	}, {
		`attrs: Arch same`,
		cloudimagemetadata.MetadataAttributes{Arch: "a"},
		cloudimagemetadata.MetadataAttributes{Arch: "a"},
		true,
	}, {
		`attrs: Arch different`,
		cloudimagemetadata.MetadataAttributes{Arch: "a"},
		cloudimagemetadata.MetadataAttributes{Arch: "b"},
		false,
	}, {
		`attrs: VirtualType same`,
		cloudimagemetadata.MetadataAttributes{VirtualType: "a"},
		cloudimagemetadata.MetadataAttributes{VirtualType: "a"},
		true,
	}, {
		`attrs: VirtualType different`,
		cloudimagemetadata.MetadataAttributes{VirtualType: "a"},
		cloudimagemetadata.MetadataAttributes{VirtualType: "b"},
		false,
	}, {
		`attrs: RootStorageType same`,
		cloudimagemetadata.MetadataAttributes{RootStorageType: "a"},
		cloudimagemetadata.MetadataAttributes{RootStorageType: "a"},
		true,
	}, {
		`attrs: RootStorageType different`,
		cloudimagemetadata.MetadataAttributes{RootStorageType: "a"},
		cloudimagemetadata.MetadataAttributes{RootStorageType: "b"},
		false,
	}, {
		`attrs: RootStorageSize same`,
		cloudimagemetadata.MetadataAttributes{RootStorageSize: "a"},
		cloudimagemetadata.MetadataAttributes{RootStorageSize: "a"},
		true,
	}, {
		`attrs: RootStorageSize different`,
		cloudimagemetadata.MetadataAttributes{RootStorageSize: "a"},
		cloudimagemetadata.MetadataAttributes{RootStorageSize: "b"},
		false,
	}, {
		`attrs: All same`,
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
		},
		true,
	}, {
		`attrs: VirtualType different`,
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
		},
		false,
	}}

	for i, t := range testData {
		c.Logf("%d: %v", i, t.about)
		same := cloudimagemetadata.AreSameAttributes(t.a, t.b)
		c.Assert(same, gc.Equals, t.expected)
	}
}

func (s *funcMetadataSuite) TestSameMetadataFunc(c *gc.C) {
	testData := []struct {
		about    string
		a        cloudimagemetadata.Metadata
		b        cloudimagemetadata.Metadata
		expected bool
	}{{
		`both empty`,
		cloudimagemetadata.Metadata{},
		cloudimagemetadata.Metadata{},
		true,
	}, {
		`one empty`,
		cloudimagemetadata.Metadata{},
		cloudimagemetadata.Metadata{ImageId: "a"},
		false,
	}, {
		`image same`,
		cloudimagemetadata.Metadata{ImageId: "a"},
		cloudimagemetadata.Metadata{ImageId: "a"},
		true,
	}, {
		`image different`,
		cloudimagemetadata.Metadata{ImageId: "a"},
		cloudimagemetadata.Metadata{ImageId: "b"},
		false,
	}, {
		`attrs: same, image same`,
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "a"}, "a"},
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "a"}, "a"},
		true,
	}, {
		`attrs different, image same`,
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "a"}, "a"},
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "b"}, "a"},
		false,
	}, {
		`attrs: same, image diff`,
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "a"}, "a"},
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "a"}, "b"},
		false,
	}, {
		`attrs different, image diff`,
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "a"}, "a"},
		cloudimagemetadata.Metadata{cloudimagemetadata.MetadataAttributes{Region: "b"}, "b"},
		false,
	}}

	for i, t := range testData {
		c.Logf("%d: %v", i, t.about)
		same := cloudimagemetadata.IsSameMetadata(t.a, t.b)
		c.Assert(same, gc.Equals, t.expected)
	}
}
