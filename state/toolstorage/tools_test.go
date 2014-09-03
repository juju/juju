// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolstorage_test

import (
	"bytes"
	"io"
	"io/ioutil"
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
	mongo   *gitjujutesting.MgoInstance
	session *mgo.Session
	storage toolstorage.Storage
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
	managedStorage := blobstore.NewManagedStorage(catalogue, rs)
	metadataCollection := catalogue.C("toolsmetadata")
	txnRunner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: catalogue})
	s.storage = toolstorage.NewStorage("my-uuid", managedStorage, metadataCollection, txnRunner)
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

func (s *ToolsSuite) TestAddToolsAlias(c *gc.C) {
	s.testAddTools(c, "abc")
	alias := bumpVersion(version.Current)
	err := s.storage.AddToolsAlias(alias, version.Current)
	c.Assert(err, gc.IsNil)

	md1, r1, err := s.storage.Tools(version.Current)
	c.Assert(err, gc.IsNil)
	defer r1.Close()
	c.Assert(md1.Version, gc.Equals, version.Current)

	md2, r2, err := s.storage.Tools(alias)
	c.Assert(err, gc.IsNil)
	defer r2.Close()
	c.Assert(md2.Version, gc.Equals, alias)

	c.Assert(md1.Size, gc.Equals, md2.Size)
	c.Assert(md1.SHA256, gc.Equals, md2.SHA256)
	data1, err := ioutil.ReadAll(r1)
	c.Assert(err, gc.IsNil)
	data2, err := ioutil.ReadAll(r2)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data1), gc.Equals, string(data2))
}

func (s *ToolsSuite) TestAddToolsAliasDoesNotReplace(c *gc.C) {
	s.testAddTools(c, "abc")
	alias := bumpVersion(version.Current)
	err := s.storage.AddToolsAlias(alias, version.Current)
	c.Assert(err, gc.IsNil)
	err = s.storage.AddToolsAlias(alias, version.Current)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *ToolsSuite) TestAddToolsAliasNotExist(c *gc.C) {
	// try to alias a non-existent version
	alias := bumpVersion(version.Current)
	err := s.storage.AddToolsAlias(alias, version.Current)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, _, err = s.storage.Tools(alias)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ToolsSuite) TestAllMetadata(c *gc.C) {
	metadata, err := s.storage.AllMetadata()
	c.Assert(err, gc.IsNil)
	c.Assert(metadata, gc.HasLen, 0)

	s.testAddTools(c, "abc")
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
	err = s.storage.AddToolsAlias(alias, version.Current)
	c.Assert(err, gc.IsNil)

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

	s.testAddTools(c, "abc")
	metadata, err = s.storage.Metadata(version.Current)
	c.Assert(err, gc.IsNil)
	c.Assert(metadata, gc.Equals, toolstorage.Metadata{
		Version: version.Current,
		Size:    3,
		SHA256:  "hash(abc)",
	})
}
