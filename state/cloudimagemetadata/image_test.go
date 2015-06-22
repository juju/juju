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

type keyMetadataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&keyMetadataSuite{})

var keyTestData = []struct {
	about       string
	series      string
	arch        string
	stream      string
	expectedKey string
}{{
	`non empty stream`,
	"series",
	"arch",
	"stream",
	"series-arch-stream",
}, {
	"empty stream",
	"series",
	"arch",
	"",
	"series-arch-released",
}}

func (s *cloudImageMetadataSuite) TestCreateMetadataKey(c *gc.C) {
	for i, t := range keyTestData {
		c.Logf("%d: %v", i, t.about)
		key := cloudimagemetadata.CreateKey(t.series, t.arch, t.stream)
		c.Assert(key, gc.Equals, t.expectedKey)
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

func (s *cloudImageMetadataSuite) TestAddMetadata(c *gc.C) {
	s.assertAddMetadataWithDefaults(c, "quantal", "amd64", "test")
}

func (s *cloudImageMetadataSuite) TestAddMetadataUpdates(c *gc.C) {
	s.assertAddMetadataWithDefaults(c, "quantal", "amd64", "test")
	s.assertAddMetadata(c, "quantal", "amd64", "test",
		"storage-test", "virtType-test", "regionAlias-test", "regionName-test", "endpoint-test",
	)
}

func (s *cloudImageMetadataSuite) assertAddMetadataWithDefaults(c *gc.C, series, arch, stream string) {
	s.assertAddMetadata(c, series, arch, stream,
		"storage", "virtType", "regionAlias", "regionName", "endpoint",
	)
}

func (s *cloudImageMetadataSuite) assertAddMetadata(c *gc.C, series, arch, stream, storage, virtType, regionAlias, regionName, endpoint string) {
	added := cloudimagemetadata.Metadata{
		Storage:     storage,
		VirtType:    virtType,
		Arch:        arch,
		Series:      series,
		RegionAlias: regionAlias,
		RegionName:  regionName,
		Endpoint:    endpoint,
		Stream:      stream,
	}
	err := s.storage.AddMetadata(added)
	c.Assert(err, jc.ErrorIsNil)

	s.assertMetadata(c, added)
}

func (s *cloudImageMetadataSuite) TestAllMetadata(c *gc.C) {
	metadata, err := s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 0)

	m := cloudimagemetadata.Metadata{
		Series:      "quantal",
		Arch:        "amd64",
		Stream:      "test",
		Storage:     "storage",
		VirtType:    "virtType",
		RegionAlias: "regionAlias",
		RegionName:  "regionName",
		Endpoint:    "endpoint",
	}
	s.addMetadataDoc(c,
		m.Series,
		m.Arch,
		m.Stream,
		m.Storage,
		m.VirtType,
		m.RegionAlias,
		m.RegionName,
		m.Endpoint,
	)

	metadata, err = s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 1)
	expected := []cloudimagemetadata.Metadata{m}
	c.Assert(metadata, jc.SameContents, expected)

	m.Arch = "my one"
	s.addMetadataDoc(c,
		m.Series,
		m.Arch,
		m.Stream,
		m.Storage,
		m.VirtType,
		m.RegionAlias,
		m.RegionName,
		m.Endpoint,
	)

	metadata, err = s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 2)
	expected = append(expected, m)
	c.Assert(metadata, jc.SameContents, expected)
}

func (s *cloudImageMetadataSuite) TestMetadata(c *gc.C) {
	m := cloudimagemetadata.Metadata{
		Series:      "quantal",
		Arch:        "amd64",
		Stream:      "test",
		Storage:     "storage",
		VirtType:    "virtType",
		RegionAlias: "regionAlias",
		RegionName:  "regionName",
		Endpoint:    "endpoint",
	}

	_, err := s.storage.Metadata(m.Series, m.Arch, m.Stream)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.addMetadataDoc(c,
		m.Series,
		m.Arch,
		m.Stream,
		m.Storage,
		m.VirtType,
		m.RegionAlias,
		m.RegionName,
		m.Endpoint,
	)
	s.assertMetadata(c, m)
}

func (s *cloudImageMetadataSuite) TestAddMetadataDuplicate(c *gc.C) {
	metadata := cloudimagemetadata.Metadata{
		Series: "quantal",
		Arch:   "amd64",
		Stream: "test",
	}
	for i := 0; i < 2; i++ {
		err := s.storage.AddMetadata(metadata)
		c.Assert(err, jc.ErrorIsNil)
		s.assertMetadata(c, metadata)
	}
	all, err := s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 1)
	expected := []cloudimagemetadata.Metadata{metadata}
	c.Assert(all, jc.SameContents, expected)

}

func (s *cloudImageMetadataSuite) TestAddMetadataConcurrent(c *gc.C) {
	metadata0 := cloudimagemetadata.Metadata{
		Series: "quantal",
		Arch:   "amd64",
		Stream: "test",
	}
	metadata1 := cloudimagemetadata.Metadata{
		Series: "quantal",
		Arch:   "amd64",
		Stream: "test2",
	}

	addMetadata := func() {
		err := s.storage.AddMetadata(metadata0)
		c.Assert(err, jc.ErrorIsNil)
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, addMetadata).Check()

	err := s.storage.AddMetadata(metadata1)
	c.Assert(err, jc.ErrorIsNil)

	s.assertMetadata(c, metadata1)
}

func (s *cloudImageMetadataSuite) addMetadataDoc(c *gc.C,
	series,
	arch,
	stream,
	storage,
	virtType,
	regionAlias,
	regionName,
	endpoint string,
) {
	doc := testMetadataDoc{
		Id:          cloudimagemetadata.CreateKey(series, arch, stream),
		Series:      series,
		Arch:        arch,
		Stream:      stream,
		Storage:     storage,
		VirtType:    virtType,
		RegionAlias: regionAlias,
		RegionName:  regionName,
		Endpoint:    endpoint,
	}
	err := s.metadataCollection.Insert(&doc)
	c.Assert(err, jc.ErrorIsNil)
}

type testMetadataDoc struct {
	Id          string `bson:"_id"`
	Series      string `bson:"series"`
	Arch        string `bson:"arch,omitempty"`
	Stream      string `bson:"stream,omitempty"`
	Storage     string `bson:"root_store,omitempty"`
	VirtType    string `bson:"virt,omitempty"`
	RegionAlias string `bson:"crsn,omitempty"`
	RegionName  string `bson:"region,omitempty"`
	Endpoint    string `bson:"endpoint,omitempty"`
}

func (s *cloudImageMetadataSuite) assertMetadata(c *gc.C, expected cloudimagemetadata.Metadata) {
	var m testMetadataDoc
	desiredID := cloudimagemetadata.CreateKey(expected.Series, expected.Arch, expected.Stream)
	c.Logf("looking for cloud image metadata with id %v", desiredID)
	err := s.metadataCollection.Find(bson.D{{"_id", desiredID}}).One(&m)
	c.Assert(err, jc.ErrorIsNil)

	metadata := cloudimagemetadata.Metadata{
		Series:      m.Series,
		Arch:        m.Arch,
		Stream:      m.Stream,
		Storage:     m.Storage,
		VirtType:    m.VirtType,
		RegionAlias: m.RegionAlias,
		RegionName:  m.RegionName,
		Endpoint:    m.Endpoint,
	}
	c.Assert(metadata, gc.DeepEquals, expected)
}

type errorTransactionRunner struct {
	jujutxn.Runner
}

func (errorTransactionRunner) Run(transactions jujutxn.TransactionSource) error {
	return errors.New("Run fails")
}
