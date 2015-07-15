// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata_test

import (
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	txntesting "github.com/juju/txn/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/testing"
)

type cloudImageMetadataSuite struct {
	testing.BaseSuite

	mongo     *gitjujutesting.MgoInstance
	session   *mgo.Session
	txnRunner jujutxn.Runner

	storage            cloudimagemetadata.Storage
	metadataCollection *mgo.Collection
}

var _ = gc.Suite(&cloudImageMetadataSuite{})

func (s *cloudImageMetadataSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mongo = &gitjujutesting.MgoInstance{}
	s.mongo.Start(nil)

	var err error
	s.session, err = s.mongo.Dial()
	c.Assert(err, jc.ErrorIsNil)
	catalogue := s.session.DB("catalogue")
	s.metadataCollection = catalogue.C("cloudimagemetadata")

	s.txnRunner = jujutxn.NewRunner(jujutxn.RunnerParams{Database: catalogue})
	s.storage = cloudimagemetadata.NewStorage("my-uuid", s.metadataCollection, s.txnRunner)
}

func (s *cloudImageMetadataSuite) TearDownTest(c *gc.C) {
	s.session.Close()
	s.mongo.DestroyWithLog()
	s.BaseSuite.TearDownTest(c)
}

func (s *cloudImageMetadataSuite) TestSaveMetadata(c *gc.C) {
	s.assertSaveMetadataWithDefaults(c, "stream", "series", "arch")
}

func (s *cloudImageMetadataSuite) TestSaveMetadataUpdates(c *gc.C) {
	s.assertSaveMetadataWithDefaults(c, "stream", "series", "arch")
	s.assertSaveMetadata(c, "stream", "region-test", "series",
		"arch", "virtual-type-test",
		"root-storage-type-test", "root-storage-size-test")
}

func (s *cloudImageMetadataSuite) assertSaveMetadataWithDefaults(c *gc.C, stream, series, arch string) {
	s.assertSaveMetadata(c, stream, "region", series, arch, "virtType", "rootType", "rootSize")
}

func (s *cloudImageMetadataSuite) assertSaveMetadata(c *gc.C, stream, region, series, arch, virtType, rootStorageType, rootStorageSize string) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          stream,
		Region:          region,
		Series:          series,
		Arch:            arch,
		VirtualType:     virtType,
		RootStorageType: rootStorageType,
		RootStorageSize: rootStorageSize}

	added := cloudimagemetadata.Metadata{attrs, "1"}
	err := s.storage.SaveMetadata(added)
	c.Assert(err, jc.ErrorIsNil)

	s.assertMetadata(c, attrs, added)
}

func (s *cloudImageMetadataSuite) TestAllMetadata(c *gc.C) {
	metadata, err := s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 0)

	m := cloudimagemetadata.Metadata{
		cloudimagemetadata.MetadataAttributes{
			Stream:          "stream",
			Region:          "region",
			Series:          "series",
			Arch:            "arch",
			VirtualType:     "virtualType",
			RootStorageType: "rootStorageType",
			RootStorageSize: "rootStorageSize"},
		"1",
	}

	s.addMetadataDoc(c, m)

	metadata, err = s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 1)
	expected := []cloudimagemetadata.Metadata{m}
	c.Assert(metadata, jc.SameContents, expected)

	m.Series = "series2"
	s.addMetadataDoc(c, m)

	metadata, err = s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 2)
	expected = append(expected, m)
	c.Assert(metadata, jc.SameContents, expected)
}

func (s *cloudImageMetadataSuite) TestFindMetadata(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region",
		Series:          "series",
		Arch:            "arch",
		VirtualType:     "virtualType",
		RootStorageType: "rootStorageType",
		RootStorageSize: "rootStorageSize"}

	m := cloudimagemetadata.Metadata{attrs, "1"}

	_, err := s.storage.FindMetadata(attrs)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.addMetadataDoc(c, m)
	expected := []cloudimagemetadata.Metadata{m}
	s.assertMetadata(c, attrs, expected...)

	attrs.Stream = "another_stream"
	m = cloudimagemetadata.Metadata{attrs, "2"}
	s.addMetadataDoc(c, m)

	expected = append(expected, m)
	// Should find both
	s.assertMetadata(c, cloudimagemetadata.MetadataAttributes{Region: "region"}, expected...)
}

func (s *cloudImageMetadataSuite) TestSaveMetadataNoUpdates(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream: "stream",
		Series: "series",
		Arch:   "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, "1"}
	metadata1 := cloudimagemetadata.Metadata{attrs, "1"}

	err := s.storage.SaveMetadata(metadata0)
	c.Assert(err, jc.ErrorIsNil)

	err = s.storage.SaveMetadata(metadata1)
	c.Assert(err, gc.ErrorMatches, ".*no changes were made.*")

	s.assertMetadata(c, attrs, metadata0)
}

func (s *cloudImageMetadataSuite) TestSaveMetadataConcurrent(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream: "stream",
		Series: "series",
		Arch:   "arch",
	}
	metadata0 := cloudimagemetadata.Metadata{attrs, "1"}
	metadata1 := cloudimagemetadata.Metadata{attrs, "2"}

	addMetadata := func() {
		err := s.storage.SaveMetadata(metadata0)
		c.Assert(err, jc.ErrorIsNil)
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, addMetadata).Check()

	err := s.storage.SaveMetadata(metadata1)
	c.Assert(err, jc.ErrorIsNil)

	s.assertMetadata(c, attrs, metadata1)
}

func (s *cloudImageMetadataSuite) addMetadataDoc(c *gc.C, m cloudimagemetadata.Metadata) {
	doc := createTestDoc(m)
	err := s.metadataCollection.Insert(&doc)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cloudImageMetadataSuite) assertMetadata(c *gc.C, criteria cloudimagemetadata.MetadataAttributes, expected ...cloudimagemetadata.Metadata) {
	var docs []testMetadataDoc
	searchCriteria := cloudimagemetadata.BuildSearchClauses(criteria)
	c.Logf("looking for cloud image metadata with id %v", criteria)
	err := s.metadataCollection.Find(searchCriteria).All(&docs)
	c.Assert(err, jc.ErrorIsNil)

	metadata := make([]cloudimagemetadata.Metadata, len(docs))
	for i, m := range docs {
		metadata[i] = cloudimagemetadata.Metadata{
			cloudimagemetadata.MetadataAttributes{
				Stream:          m.Stream,
				Region:          m.Region,
				Series:          m.Series,
				Arch:            m.Arch,
				VirtualType:     m.VirtualType,
				RootStorageType: m.RootStorageType,
				RootStorageSize: m.RootStorageSize,
			}, m.ImageId}
	}
	c.Assert(metadata, jc.SameContents, expected)
}

type testMetadataDoc struct {
	Id              string `bson:"_id"`
	Stream          string `bson:"stream"`
	Region          string `bson:"region`
	Series          string `bson:"series"`
	Arch            string `bson:"arch"`
	ImageId         string `bson:"image_id"`
	VirtualType     string `bson:"virtual_type,omitempty"`
	RootStorageType string `bson:"root_storage_type,omitempty"`
	RootStorageSize string `bson:"root_storage_size,omitempty"`
}

func createTestDoc(m cloudimagemetadata.Metadata) testMetadataDoc {
	key := cloudimagemetadata.BuildKey(&m)
	return testMetadataDoc{
		Id:              key,
		Stream:          m.Stream,
		Region:          m.Region,
		Series:          m.Series,
		Arch:            m.Arch,
		VirtualType:     m.VirtualType,
		RootStorageType: m.RootStorageType,
		RootStorageSize: m.RootStorageSize,
		ImageId:         m.ImageId,
	}
}

type errorTransactionRunner struct {
	jujutxn.Runner
}

func (errorTransactionRunner) Run(transactions jujutxn.TransactionSource) error {
	return errors.New("Run fails")
}
