// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v3"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/testing"
)

type tooler interface {
	AgentTools() (*tools.Tools, error)
	SetAgentVersion(v version.Binary) error
	Refresh() error
}

func testAgentTools(c *gc.C, store objectstore.ObjectStore, obj tooler, agent string) {
	// object starts with zero'd tools.
	t, err := obj.AgentTools()
	c.Assert(t, gc.IsNil)
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = obj.SetAgentVersion(version.Binary{})
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("setting agent version for %s: empty series or arch", agent))

	v2 := version.MustParseBinary("7.8.9-ubuntu-amd64")
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

	if le, ok := obj.(lifer); ok {
		testWhenDying(c, store, le, noErr, deadErr, func() error {
			return obj.SetAgentVersion(v2)
		})
	}
}

type binaryStorageSuite struct {
	ConnSuite

	controllerModelUUID string
	modelUUID           string
	st                  *state.State
	store               objectstore.ObjectStore
}

var _ = gc.Suite(&binaryStorageSuite{})

func (s *binaryStorageSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	s.controllerModelUUID = s.State.ControllerModelUUID()

	// Create a new model and store its UUID.
	s.modelUUID = utils.MustNewUUID().String()
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": "new-model",
		"uuid": s.modelUUID,
	})
	var err error
	_, s.st, err = s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   names.NewLocalUserTag("test-admin"),
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) {
		s.st.Close()
	})

	s.store = state.NewObjectStore(c, s.st)
}

type storageOpener func(objectstore.ObjectStore) (binarystorage.StorageCloser, error)

func (s *binaryStorageSuite) TestToolsStorage(c *gc.C) {
	s.testStorage(c, "toolsmetadata", s.State.ToolsStorage)
}

func (s *binaryStorageSuite) TestToolsStorageParamsControllerModel(c *gc.C) {
	s.testStorageParams(c, "toolsmetadata", s.State.ToolsStorage)
}

func (s *binaryStorageSuite) TestToolsStorageParamsHostedModel(c *gc.C) {
	s.testStorageParams(c, "toolsmetadata", s.st.ToolsStorage)
}

func (s *binaryStorageSuite) testStorage(c *gc.C, collName string, openStorage storageOpener) {
	session := s.State.MongoSession()
	// if the collection didn't exist, we will create it on demand.
	err := session.DB("juju").C(collName).DropCollection()
	c.Assert(err, jc.ErrorIsNil)
	collectionNames, err := session.DB("juju").CollectionNames()
	c.Assert(err, jc.ErrorIsNil)
	nameSet := set.NewStrings(collectionNames...)
	c.Assert(nameSet.Contains(collName), jc.IsFalse)

	storage, err := openStorage(s.store)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := storage.Close()
		c.Assert(err, jc.ErrorIsNil)
	}()

	err = storage.Add(context.Background(), strings.NewReader(""), binarystorage.Metadata{})
	c.Assert(err, jc.ErrorIsNil)

	collectionNames, err = session.DB("juju").CollectionNames()
	c.Assert(err, jc.ErrorIsNil)
	nameSet = set.NewStrings(collectionNames...)
	c.Assert(nameSet.Contains(collName), jc.IsTrue)
}

func (s *binaryStorageSuite) testStorageParams(c *gc.C, collName string, openStorage storageOpener) {
	s.PatchValue(state.BinarystorageNew, func(
		managedStorage binarystorage.ManagedStorage,
		metadataCollection mongo.Collection,
		runner jujutxn.Runner,
	) binarystorage.Storage {
		c.Assert(managedStorage, gc.NotNil)
		c.Assert(metadataCollection.Name(), gc.Equals, collName)
		c.Assert(runner, gc.NotNil)
		return nil
	})

	storage, err := openStorage(s.store)
	c.Assert(err, jc.ErrorIsNil)
	storage.Close()
}
