// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/set"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/blobstore.v2"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/tools"
)

type tooler interface {
	lifer
	AgentTools() (*tools.Tools, error)
	SetAgentVersion(v version.Binary) error
	Refresh() error
}

func testAgentTools(c *gc.C, obj tooler, agent string) {
	// object starts with zero'd tools.
	t, err := obj.AgentTools()
	c.Assert(t, gc.IsNil)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	err = obj.SetAgentVersion(version.Binary{})
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("cannot set agent version for %s: empty series or arch", agent))

	v2 := version.MustParseBinary("7.8.9-quantal-amd64")
	err = obj.SetAgentVersion(v2)
	c.Assert(err, jc.ErrorIsNil)
	t3, err := obj.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(t3.Version, gc.DeepEquals, v2)
	err = obj.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	t3, err = obj.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(t3.Version, gc.DeepEquals, v2)

	testWhenDying(c, obj, noErr, deadErr, func() error {
		return obj.SetAgentVersion(v2)
	})
}

type binaryStorageSuite struct {
	ConnSuite
}

var _ = gc.Suite(&binaryStorageSuite{})

type storageOpener func() (binarystorage.StorageCloser, error)

func (s *binaryStorageSuite) TestToolsStorage(c *gc.C) {
	s.testStorage(c, "toolsmetadata", s.State.ToolsStorage)
}

func (s *binaryStorageSuite) TestToolsStorageParams(c *gc.C) {
	s.testStorageParams(c, "toolsmetadata", s.State.ToolsStorage)
}

func (s *binaryStorageSuite) TestGUIArchiveStorage(c *gc.C) {
	s.testStorage(c, "guimetadata", s.State.GUIStorage)
}

func (s *binaryStorageSuite) TestGUIArchiveStorageParams(c *gc.C) {
	s.testStorageParams(c, "guimetadata", s.State.GUIStorage)
}

func (s *binaryStorageSuite) testStorage(c *gc.C, collName string, openStorage storageOpener) {
	session := s.State.MongoSession()
	collectionNames, err := session.DB("juju").CollectionNames()
	c.Assert(err, jc.ErrorIsNil)
	nameSet := set.NewStrings(collectionNames...)
	c.Assert(nameSet.Contains(collName), jc.IsFalse)

	storage, err := openStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := storage.Close()
		c.Assert(err, jc.ErrorIsNil)
	}()

	err = storage.Add(strings.NewReader(""), binarystorage.Metadata{})
	c.Assert(err, jc.ErrorIsNil)

	collectionNames, err = session.DB("juju").CollectionNames()
	c.Assert(err, jc.ErrorIsNil)
	nameSet = set.NewStrings(collectionNames...)
	c.Assert(nameSet.Contains(collName), jc.IsTrue)
}

func (s *binaryStorageSuite) testStorageParams(c *gc.C, collName string, openStorage storageOpener) {
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	var called bool
	s.PatchValue(state.BinarystorageNew, func(
		modelUUID string,
		managedStorage blobstore.ManagedStorage,
		metadataCollection *mgo.Collection,
		runner jujutxn.Runner,
	) binarystorage.Storage {
		called = true
		c.Assert(modelUUID, gc.Equals, env.UUID())
		c.Assert(managedStorage, gc.NotNil)
		c.Assert(metadataCollection.Name, gc.Equals, collName)
		c.Assert(runner, gc.NotNil)
		return nil
	})

	storage, err := openStorage()
	c.Assert(err, jc.ErrorIsNil)
	storage.Close()
	c.Assert(called, jc.IsTrue)
}
