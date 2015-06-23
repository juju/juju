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
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/testing"
)

type funcMetadataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&funcMetadataSuite{})

var keyTestData = []struct {
	about       string
	stream      string
	expectedKey string
}{{
	`non empty stream`,
	"stream",
	"stream",
}, {
	"empty stream",
	"",
	"released",
}}

func (s *funcMetadataSuite) TestCreateMetadataKey(c *gc.C) {
	for i, t := range keyTestData {
		c.Logf("%d: %v", i, t.about)
		key := cloudimagemetadata.StreamKey(t.stream)
		c.Assert(key, gc.Equals, t.expectedKey)
	}
}

var searchTestData = []struct {
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
	`two search criteria supplied`,
	cloudimagemetadata.MetadataAttributes{RootStorageType: "rootstorage-value", Series: "series-value"},
	bson.D{{"root_storage_type", "rootstorage-value"}, {"series", "series-value"}},
}, {
	`all serach criteria supplied`,
	cloudimagemetadata.MetadataAttributes{
		RootStorageType: "rootstorage-value",
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
	},
}}

func (s *funcMetadataSuite) TestSearchCriteria(c *gc.C) {
	for i, t := range searchTestData {
		c.Logf("%d: %v", i, t.about)
		clause := cloudimagemetadata.SearchClauses(t.criteria)
		c.Assert(clause, jc.SameContents, t.expected)
	}
}

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
		"arch", "virtual-type-test", "root-storage-type-test")
}

func (s *cloudImageMetadataSuite) assertSaveMetadataWithDefaults(c *gc.C, stream, series, arch string) {
	s.assertSaveMetadata(c, stream, "region", series, arch, "virtType", "rootType")
}

func (s *cloudImageMetadataSuite) assertSaveMetadata(c *gc.C, stream, region, series, arch, virtType, rootStorageType string) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          stream,
		Region:          region,
		Series:          series,
		Arch:            arch,
		VirtualType:     virtType,
		RootStorageType: rootStorageType}

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
			RootStorageType: "rootStorageType"},
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
		RootStorageType: "rootStorageType"}

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

func (s *cloudImageMetadataSuite) TestSaveMetadataDuplicate(c *gc.C) {
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream: "stream",
		Series: "series",
		Arch:   "arch"}
	metadata := cloudimagemetadata.Metadata{attrs, "1"}

	for i := 0; i < 2; i++ {
		err := s.storage.SaveMetadata(metadata)
		c.Assert(err, jc.ErrorIsNil)
		s.assertMetadata(c, attrs, metadata)
	}
	all, err := s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 1)
	expected := []cloudimagemetadata.Metadata{metadata}
	c.Assert(all, jc.SameContents, expected)

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
	searchCriteria := cloudimagemetadata.SearchClauses(criteria)
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
}

func createTestDoc(m cloudimagemetadata.Metadata) testMetadataDoc {
	key := cloudimagemetadata.Key(&m)
	return testMetadataDoc{
		Id:              key,
		Stream:          m.Stream,
		Region:          m.Region,
		Series:          m.Series,
		Arch:            m.Arch,
		VirtualType:     m.VirtualType,
		RootStorageType: m.RootStorageType,
		ImageId:         m.ImageId,
	}
}

type errorTransactionRunner struct {
	jujutxn.Runner
}

func (errorTransactionRunner) Run(transactions jujutxn.TransactionSource) error {
	return errors.New("Run fails")
}
