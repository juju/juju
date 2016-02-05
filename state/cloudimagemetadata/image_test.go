// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata_test

import (
	"regexp"

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
	envName        = "test-model"
	collectionName = "test-collection"
)

func (s *cloudImageMetadataSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)

	db := s.MgoSuite.Session.DB("juju")

	s.access = NewTestMongo(db)
	s.storage = cloudimagemetadata.NewStorage(envName, collectionName, s.access)
}

func (s *cloudImageMetadataSuite) TestSaveMetadata(c *gc.C) {
	attrs1 := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region-test",
		Version:         "14.04",
		Series:          "trusty",
		Arch:            "arch",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test",
	}
	attrs2 := cloudimagemetadata.MetadataAttributes{
		Stream:  "chalk",
		Region:  "nether",
		Version: "12.04",
		Series:  "precise",
		Arch:    "amd64",
	}
	added := []cloudimagemetadata.Metadata{
		{attrs1, 0, "1"},
		{attrs2, 0, "2"},
	}
	s.assertRecordMetadata(c, added[0])
	s.assertRecordMetadata(c, added[1])
	s.assertMetadataRecorded(c, cloudimagemetadata.MetadataAttributes{}, added...)
}

func (s *cloudImageMetadataSuite) TestFindMetadataNotFound(c *gc.C) {
	s.assertNoMetadata(c)

	// insert something...
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region",
		Version:         "14.04",
		Series:          "trusty",
		Arch:            "arch",
		VirtType:        "virtType",
		RootStorageType: "rootStorageType"}
	m := cloudimagemetadata.Metadata{attrs, 0, "1"}
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
		VirtType:        attrs.VirtType,
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
		Version:         "14.04",
		Series:          "trusty",
		Arch:            "arch",
		VirtType:        "virtType",
		RootStorageType: "rootStorageType"}

	m := cloudimagemetadata.Metadata{attrs, 0, "1"}

	_, err := s.storage.FindMetadata(buildAttributesFilter(attrs))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.assertRecordMetadata(c, m)
	expected := []cloudimagemetadata.Metadata{m}
	s.assertMetadataRecorded(c, attrs, expected...)

	attrs.Stream = "another_stream"
	m = cloudimagemetadata.Metadata{attrs, 0, "2"}
	s.assertRecordMetadata(c, m)

	expected = append(expected, m)
	// Should find both
	s.assertMetadataRecorded(c, cloudimagemetadata.MetadataAttributes{Region: "region"}, expected...)
}

func (s *cloudImageMetadataSuite) TestSaveMetadataUpdateSameAttrsAndImages(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:  "stream",
		Version: "14.04",
		Series:  "trusty",
		Arch:    "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, 0, "1"}
	metadata1 := cloudimagemetadata.Metadata{attrs, 0, "1"}

	s.assertRecordMetadata(c, metadata0)
	s.assertRecordMetadata(c, metadata1)
	s.assertMetadataRecorded(c, attrs, metadata1)
}

func (s *cloudImageMetadataSuite) TestSaveMetadataUpdateSameAttrsDiffImages(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:  "stream",
		Version: "14.04",
		Series:  "trusty",
		Arch:    "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, 0, "1"}
	metadata1 := cloudimagemetadata.Metadata{attrs, 0, "12"}

	s.assertRecordMetadata(c, metadata0)
	s.assertMetadataRecorded(c, attrs, metadata0)
	s.assertRecordMetadata(c, metadata1)
	s.assertMetadataRecorded(c, attrs, metadata1)
	s.assertMetadataRecorded(c, cloudimagemetadata.MetadataAttributes{}, metadata1)
}

func (s *cloudImageMetadataSuite) TestSaveDiffMetadataConcurrentlyAndOrderByDateCreated(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:  "stream",
		Version: "14.04",
		Series:  "trusty",
		Arch:    "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, 0, "0"}
	metadata1 := cloudimagemetadata.Metadata{attrs, 0, "1"}
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
		Stream:  "stream",
		Version: "14.04",
		Series:  "trusty",
		Arch:    "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, 0, "0"}
	metadata1 := cloudimagemetadata.Metadata{attrs, 0, "1"}

	s.assertConcurrentSave(c,
		metadata0, // add this one
		metadata1, // overwrite it with this one
		metadata1, // verify only the last one is in the list
	)
}

func (s *cloudImageMetadataSuite) TestSaveSameMetadataSameImageConcurrently(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:  "stream",
		Version: "14.04",
		Series:  "trusty",
		Arch:    "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, 0, "0"}

	s.assertConcurrentSave(c,
		metadata0, // add this one
		metadata0, // add it again
		metadata0, // varify only one is in the list
	)
}

func (s *cloudImageMetadataSuite) TestSaveSameMetadataSameImageDiffSourceConcurrently(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:  "stream",
		Version: "14.04",
		Series:  "trusty",
		Arch:    "arch",
		Source:  "public",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, 0, "0"}

	attrs.Source = "custom"
	metadata1 := cloudimagemetadata.Metadata{attrs, 0, "0"}

	s.assertConcurrentSave(c,
		metadata0,
		metadata1,
		metadata0,
		metadata1,
	)
}

func (s *cloudImageMetadataSuite) TestSaveMetadataNoVersionPassed(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream: "stream",
		Series: "trusty",
		Arch:   "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, 0, "1"}
	s.assertRecordMetadata(c, metadata0)
}

func (s *cloudImageMetadataSuite) TestSaveMetadataNoSeriesPassed(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream: "stream",
		Arch:   "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, 0, "1"}
	err := s.storage.SaveMetadata([]cloudimagemetadata.Metadata{metadata0})
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`missing series: metadata for image 1 not valid`))
}

func (s *cloudImageMetadataSuite) TestSaveMetadataUnsupportedSeriesPassed(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream: "stream",
		Series: "blah",
		Arch:   "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, 0, "1"}
	err := s.storage.SaveMetadata([]cloudimagemetadata.Metadata{metadata0})
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`unknown version for series: "blah"`))
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
	err := s.storage.SaveMetadata([]cloudimagemetadata.Metadata{m})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cloudImageMetadataSuite) assertMetadataRecorded(c *gc.C, criteria cloudimagemetadata.MetadataAttributes, expected ...cloudimagemetadata.Metadata) {
	metadata, err := s.storage.FindMetadata(buildAttributesFilter(criteria))
	c.Assert(err, jc.ErrorIsNil)

	groups := make(map[string][]cloudimagemetadata.Metadata)
	for _, one := range expected {
		groups[one.Source] = append(groups[one.Source], one)
	}
	c.Assert(metadata, jc.DeepEquals, groups)
}

func (s *cloudImageMetadataSuite) TestSupportedArchitectures(c *gc.C) {
	stream := "stream"
	region := "region-test"

	arch1 := "arch"
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          stream,
		Region:          region,
		Version:         "14.04",
		Series:          "trusty",
		Arch:            arch1,
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test"}

	added := cloudimagemetadata.Metadata{attrs, 0, "1"}
	s.assertRecordMetadata(c, added)
	s.assertMetadataRecorded(c, attrs, added)

	addedNonUnique := cloudimagemetadata.Metadata{attrs, 0, "21"}
	s.assertRecordMetadata(c, addedNonUnique)
	s.assertMetadataRecorded(c, attrs, addedNonUnique)

	arch2 := "anotherArch"
	attrs.Arch = arch2
	added2 := cloudimagemetadata.Metadata{attrs, 0, "21"}
	s.assertRecordMetadata(c, added2)
	s.assertMetadataRecorded(c, attrs, added2)

	expected := []string{arch1, arch2}
	uniqueArches, err := s.storage.SupportedArchitectures(
		cloudimagemetadata.MetadataFilter{Stream: stream, Region: region})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uniqueArches, gc.DeepEquals, expected)
}

func (s *cloudImageMetadataSuite) TestSupportedArchitecturesUnmatchedStreams(c *gc.C) {
	stream := "stream"
	region := "region-test"

	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "new-stream",
		Region:          region,
		Version:         "14.04",
		Series:          "trusty",
		Arch:            "arch",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test"}

	added := cloudimagemetadata.Metadata{attrs, 0, "1"}
	s.assertRecordMetadata(c, added)
	s.assertMetadataRecorded(c, attrs, added)

	uniqueArches, err := s.storage.SupportedArchitectures(
		cloudimagemetadata.MetadataFilter{Stream: stream, Region: region})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uniqueArches, gc.DeepEquals, []string{})
}

func (s *cloudImageMetadataSuite) TestSupportedArchitecturesUnmatchedRegions(c *gc.C) {
	stream := "stream"
	region := "region-test"

	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          stream,
		Region:          "new-region",
		Version:         "14.04",
		Series:          "trusty",
		Arch:            "arch",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test"}

	added := cloudimagemetadata.Metadata{attrs, 0, "1"}
	s.assertRecordMetadata(c, added)
	s.assertMetadataRecorded(c, attrs, added)

	uniqueArches, err := s.storage.SupportedArchitectures(
		cloudimagemetadata.MetadataFilter{Stream: stream, Region: region})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uniqueArches, gc.DeepEquals, []string{})
}

func (s *cloudImageMetadataSuite) TestSupportedArchitecturesUnmatchedStreamsAndRegions(c *gc.C) {
	stream := "stream"
	region := "region-test"

	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "new-stream",
		Region:          "new-region",
		Version:         "14.04",
		Series:          "trusty",
		Arch:            "arch",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test"}

	added := cloudimagemetadata.Metadata{attrs, 0, "1"}
	s.assertRecordMetadata(c, added)
	s.assertMetadataRecorded(c, attrs, added)

	uniqueArches, err := s.storage.SupportedArchitectures(
		cloudimagemetadata.MetadataFilter{Stream: stream, Region: region})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uniqueArches, gc.DeepEquals, []string{})
}

func (s *cloudImageMetadataSuite) TestDeleteMetadata(c *gc.C) {
	imageId := "ok-to-delete"
	s.addTestImageMetadata(c, imageId)
	s.assertDeleteMetadata(c, imageId)
	s.assertNoMetadata(c)

	// calling delete on it again should be a no-op
	s.assertDeleteMetadata(c, imageId)
	// make sure log has "nothing to delete" message
	c.Assert(c.GetTestLog(), jc.Contains, "no metadata for image ID ok-to-delete to delete")
}

func (s *cloudImageMetadataSuite) TestDeleteDiffMetadataConcurrently(c *gc.C) {
	imageId := "ok-to-delete"
	s.addTestImageMetadata(c, imageId)

	diffImageId := "ok-to-delete-too"
	s.addTestImageMetadata(c, diffImageId)

	s.assertConcurrentDelete(c, imageId, diffImageId)
}

func (s *cloudImageMetadataSuite) TestDeleteSameMetadataConcurrently(c *gc.C) {
	imageId := "ok-to-delete"
	s.addTestImageMetadata(c, imageId)

	s.assertConcurrentDelete(c, imageId, imageId)
}

func (s *cloudImageMetadataSuite) assertConcurrentDelete(c *gc.C, imageId0, imageId1 string) {
	deleteMetadata := func() {
		s.assertDeleteMetadata(c, imageId0)
	}
	defer txntesting.SetBeforeHooks(c, s.access.runner, deleteMetadata).Check()
	s.assertDeleteMetadata(c, imageId1)
	s.assertNoMetadata(c)
}

func (s *cloudImageMetadataSuite) addTestImageMetadata(c *gc.C, imageId string) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region-test",
		Version:         "14.04",
		Series:          "trusty",
		Arch:            "arch",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test"}

	added := cloudimagemetadata.Metadata{attrs, 0, imageId}
	s.assertRecordMetadata(c, added)
	s.assertMetadataRecorded(c, attrs, added)
}

func (s *cloudImageMetadataSuite) assertDeleteMetadata(c *gc.C, imageId string) {
	err := s.storage.DeleteMetadata(imageId)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cloudImageMetadataSuite) assertNoMetadata(c *gc.C) {
	// No metadata should be in store.
	// So when looking for all and none is found, err.
	found, err := s.storage.FindMetadata(cloudimagemetadata.MetadataFilter{})
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, "matching cloud image metadata not found")
	c.Assert(found, gc.HasLen, 0)
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
