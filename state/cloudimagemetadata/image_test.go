// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/txn"
	txntesting "github.com/juju/txn/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/cloudimagemetadata"
)

type cloudImageMetadataSuite struct {
	testing.IsolatedMgoSuite

	access  *TestMongo
	storage cloudimagemetadata.Storage
}

var _ = gc.Suite(&cloudImageMetadataSuite{})

const (
	envName        = "test-env"
	collectionName = "test-collection"
)

func (s *cloudImageMetadataSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)

	db := s.MgoSuite.Session.DB("juju")

	s.access = NewTestMongo(db)
	s.storage = cloudimagemetadata.NewStorage(envName, collectionName, s.access)
}

func (s *cloudImageMetadataSuite) TestSaveMetadata(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region-test",
		Series:          "series",
		Arch:            "arch",
		VirtualType:     "virtType-test",
		RootStorageType: "rootStorageType-test"}

	added := cloudimagemetadata.Metadata{attrs, "1"}
	s.assertRecordMetadata(c, added)
	s.assertMetadataRecorded(c, attrs, added)

}

func (s *cloudImageMetadataSuite) TestFindMetadataNotFound(c *gc.C) {
	// No metadata is stored yet.
	// So when looking for all and none is found, err.
	found, err := s.storage.FindMetadata(cloudimagemetadata.MetadataFilter{})
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, "matching cloud image metadata not found")
	c.Assert(found, gc.HasLen, 0)

	// insert something...
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region",
		Series:          "series",
		Arch:            "arch",
		VirtualType:     "virtualType",
		RootStorageType: "rootStorageType"}
	m := cloudimagemetadata.Metadata{attrs, "1"}
	s.assertRecordMetadata(c, m)

	// ...but look for something else.
	none, err := s.storage.FindMetadata(cloudimagemetadata.MetadataFilter{
		Stream: "something else",
	})
	// Make sure that we are explicit that we could not find what we wanted.
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, "matching cloud image metadata not found")
	c.Assert(none, gc.HasLen, 0)
}

func buildAttributesFilter(attrs cloudimagemetadata.MetadataAttributes) cloudimagemetadata.MetadataFilter {
	filter := cloudimagemetadata.MetadataFilter{
		Stream:          attrs.Stream,
		Region:          attrs.Region,
		VirtualType:     attrs.VirtualType,
		RootStorageType: attrs.RootStorageType}
	if attrs.Series != "" {
		filter.Series = []string{attrs.Series}
	}
	if attrs.Arch != "" {
		filter.Arches = []string{attrs.Arch}
	}
	return filter
}

func (s *cloudImageMetadataSuite) TestFindMetadata(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region",
		Series:          "series",
		Arch:            "arch",
		VirtualType:     "virtualType",
		RootStorageType: "rootStorageType"}

	m := cloudimagemetadata.Metadata{attrs, "1"}

	_, err := s.storage.FindMetadata(buildAttributesFilter(attrs))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.assertRecordMetadata(c, m)
	expected := []cloudimagemetadata.Metadata{m}
	s.assertMetadataRecorded(c, attrs, expected...)

	attrs.Stream = "another_stream"
	m = cloudimagemetadata.Metadata{attrs, "2"}
	s.assertRecordMetadata(c, m)

	expected = append(expected, m)
	// Should find both
	s.assertMetadataRecorded(c, cloudimagemetadata.MetadataAttributes{Region: "region"}, expected...)
}

func (s *cloudImageMetadataSuite) TestSaveMetadataUpdateSameAttrsAndImages(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream: "stream",
		Series: "series",
		Arch:   "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, "1"}
	metadata1 := cloudimagemetadata.Metadata{attrs, "1"}

	s.assertRecordMetadata(c, metadata0)
	s.assertRecordMetadata(c, metadata1)
	s.assertMetadataRecorded(c, attrs, metadata1)
}

func (s *cloudImageMetadataSuite) TestSaveMetadataUpdateSameAttrsDiffImages(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream: "stream",
		Series: "series",
		Arch:   "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, "1"}
	metadata1 := cloudimagemetadata.Metadata{attrs, "12"}

	s.assertRecordMetadata(c, metadata0)
	s.assertMetadataRecorded(c, attrs, metadata0)
	s.assertRecordMetadata(c, metadata1)
	s.assertMetadataRecorded(c, attrs, metadata1)
	s.assertMetadataRecorded(c, cloudimagemetadata.MetadataAttributes{}, metadata1)
}

func (s *cloudImageMetadataSuite) TestSaveDiffMetadataConcurrentlyAndOrderByDateCreated(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream: "stream",
		Series: "series",
		Arch:   "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, "0"}
	metadata1 := cloudimagemetadata.Metadata{attrs, "1"}
	metadata1.Stream = "scream"

	s.assertConcurrentSave(c,
		metadata0, // add this one
		metadata1, // add this one
		// last added should be first as order is by date created
		metadata1, // verify it's in the list
		metadata0, // verify it's in the list
	)
}

func (s *cloudImageMetadataSuite) TestSaveSameMetadataDiffImageConcurrently(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream: "stream",
		Series: "series",
		Arch:   "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, "0"}
	metadata1 := cloudimagemetadata.Metadata{attrs, "1"}

	s.assertConcurrentSave(c,
		metadata0, // add this one
		metadata1, // overwrite it with this one
		metadata1, // verify only the last one is in the list
	)
}

func (s *cloudImageMetadataSuite) TestSaveSameMetadataSameImageConcurrently(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream: "stream",
		Series: "series",
		Arch:   "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, "0"}

	s.assertConcurrentSave(c,
		metadata0, // add this one
		metadata0, // add it again
		metadata0, // varify only one is in the list
	)
}

func (s *cloudImageMetadataSuite) TestSaveSameMetadataSameImageDiffSourceConcurrently(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream: "stream",
		Series: "series",
		Arch:   "arch",
		Source: cloudimagemetadata.Public,
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, "0"}

	attrs.Source = cloudimagemetadata.Custom
	metadata1 := cloudimagemetadata.Metadata{attrs, "0"}

	s.assertConcurrentSave(c,
		metadata0,
		metadata1,
		metadata0,
		metadata1,
	)
}

func (s *cloudImageMetadataSuite) assertConcurrentSave(c *gc.C, metadata0, metadata1 cloudimagemetadata.Metadata, expected ...cloudimagemetadata.Metadata) {
	addMetadata := func() {
		s.assertRecordMetadata(c, metadata0)
	}
	defer txntesting.SetBeforeHooks(c, s.access.runner, addMetadata).Check()
	s.assertRecordMetadata(c, metadata1)
	s.assertMetadataRecorded(c, cloudimagemetadata.MetadataAttributes{}, expected...)
}

func (s *cloudImageMetadataSuite) assertRecordMetadata(c *gc.C, m cloudimagemetadata.Metadata) {
	err := s.storage.SaveMetadata(m)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cloudImageMetadataSuite) assertMetadataRecorded(c *gc.C, criteria cloudimagemetadata.MetadataAttributes, expected ...cloudimagemetadata.Metadata) {
	metadata, err := s.storage.FindMetadata(buildAttributesFilter(criteria))
	c.Assert(err, jc.ErrorIsNil)

	groups := make(map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata)
	for _, one := range expected {
		groups[one.Source] = append(groups[one.Source], one)
	}
	c.Assert(metadata, jc.DeepEquals, groups)
}

type TestMongo struct {
	database *mgo.Database
	runner   txn.Runner
}

func NewTestMongo(database *mgo.Database) *TestMongo {
	return &TestMongo{
		database: database,
		runner: txn.NewRunner(txn.RunnerParams{
			Database: database,
		}),
	}
}

func (m *TestMongo) GetCollection(name string) (mongo.Collection, func()) {
	return mongo.CollectionFromName(m.database, name)
}

func (m *TestMongo) RunTransaction(getTxn txn.TransactionSource) error {
	return m.runner.Run(getTxn)
}
