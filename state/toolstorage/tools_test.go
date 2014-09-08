// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolstorage_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"strings"
	stdtesting "testing"

	"github.com/juju/blobstore"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/toolstorage"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

var _ = gc.Suite(&ToolsSuite{})

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type ToolsSuite struct {
	testing.BaseSuite
	mongo              *gitjujutesting.MgoInstance
	session            *mgo.Session
	storage            toolstorage.Storage
	managedStorage     blobstore.ManagedStorage
	metadataCollection *mgo.Collection
}

func (s *ToolsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mongo = &gitjujutesting.MgoInstance{}
	s.mongo.Start(nil)

	var err error
	s.session, err = s.mongo.Dial()
	c.Assert(err, gc.IsNil)
	rs := blobstore.NewGridFS("blobstore", "my-uuid", s.session)
	catalogue := s.session.DB("catalogue")
	s.managedStorage = blobstore.NewManagedStorage(catalogue, rs)
	s.metadataCollection = catalogue.C("toolsmetadata")
	txnRunner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: catalogue})
	s.storage = toolstorage.NewStorage("my-uuid", s.managedStorage, s.metadataCollection, txnRunner)
}

func (s *ToolsSuite) TearDownTest(c *gc.C) {
	s.session.Close()
	s.mongo.DestroyWithLog()
	s.BaseSuite.TearDownTest(c)
}

func (s *ToolsSuite) TestAddTools(c *gc.C) {
	s.testAddTools(c, "some-tools")
}

func (s *ToolsSuite) TestAddToolsReplaces(c *gc.C) {
	s.testAddTools(c, "abc")
	s.testAddTools(c, "def")
}

func (s *ToolsSuite) testAddTools(c *gc.C, content string) {
	var r io.Reader = bytes.NewReader([]byte(content))
	addedMetadata := toolstorage.Metadata{
		Version: version.Current,
		Size:    int64(len(content)),
		SHA256:  "hash(" + content + ")",
	}
	err := s.storage.AddTools(r, addedMetadata)
	c.Assert(err, gc.IsNil)

	metadata, rc, err := s.storage.Tools(version.Current)
	c.Assert(err, gc.IsNil)
	c.Assert(r, gc.NotNil)
	defer rc.Close()
	c.Assert(metadata, gc.Equals, addedMetadata)

	data, err := ioutil.ReadAll(rc)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, content)
}

func bumpVersion(v version.Binary) version.Binary {
	v.Build++
	return v
}

func (s *ToolsSuite) TestAllMetadata(c *gc.C) {
	metadata, err := s.storage.AllMetadata()
	c.Assert(err, gc.IsNil)
	c.Assert(metadata, gc.HasLen, 0)

	s.addMetadataDoc(c, version.Current, 3, "hash(abc)", "path")
	metadata, err = s.storage.AllMetadata()
	c.Assert(err, gc.IsNil)
	c.Assert(metadata, gc.HasLen, 1)
	expected := []toolstorage.Metadata{{
		Version: version.Current,
		Size:    3,
		SHA256:  "hash(abc)",
	}}
	c.Assert(metadata, jc.SameContents, expected)

	alias := bumpVersion(version.Current)
	s.addMetadataDoc(c, alias, 3, "hash(abc)", "path")

	metadata, err = s.storage.AllMetadata()
	c.Assert(err, gc.IsNil)
	c.Assert(metadata, gc.HasLen, 2)
	expected = append(expected, toolstorage.Metadata{
		Version: alias,
		Size:    3,
		SHA256:  "hash(abc)",
	})
	c.Assert(metadata, jc.SameContents, expected)
}

func (s *ToolsSuite) TestMetadata(c *gc.C) {
	metadata, err := s.storage.Metadata(version.Current)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.addMetadataDoc(c, version.Current, 3, "hash(abc)", "path")
	metadata, err = s.storage.Metadata(version.Current)
	c.Assert(err, gc.IsNil)
	c.Assert(metadata, gc.Equals, toolstorage.Metadata{
		Version: version.Current,
		Size:    3,
		SHA256:  "hash(abc)",
	})
}

func (s *ToolsSuite) TestTools(c *gc.C) {
	_, _, err := s.storage.Tools(version.Current)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `.* tools metadata not found`)

	s.addMetadataDoc(c, version.Current, 3, "hash(abc)", "path")
	_, _, err = s.storage.Tools(version.Current)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `resource at path "environs/my-uuid/path" not found`)

	err = s.managedStorage.PutForEnvironment("my-uuid", "path", strings.NewReader("blah"), 4)
	c.Assert(err, gc.IsNil)

	metadata, r, err := s.storage.Tools(version.Current)
	c.Assert(err, gc.IsNil)
	defer r.Close()
	c.Assert(metadata, gc.Equals, toolstorage.Metadata{
		Version: version.Current,
		Size:    3,
		SHA256:  "hash(abc)",
	})

	data, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "blah")
}

func (s *ToolsSuite) addMetadataDoc(c *gc.C, v version.Binary, size int64, hash, path string) {
	doc := struct {
		Id      string         `bson:"_id"`
		Version version.Binary `bson:"version"`
		Size    int64          `bson:"size"`
		SHA256  string         `bson:"sha256,omitempty"`
		Path    string         `bson:"path"`
	}{
		Id:      v.String(),
		Version: v,
		Size:    size,
		SHA256:  hash,
		Path:    path,
	}
	err := s.metadataCollection.Insert(&doc)
	c.Assert(err, gc.IsNil)
}
