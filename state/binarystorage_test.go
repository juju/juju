// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/blobstore.v2"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

type tooler interface {
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

	if le, ok := obj.(lifer); ok {
		testWhenDying(c, le, noErr, deadErr, func() error {
			return obj.SetAgentVersion(v2)
		})
	}
}

type binaryStorageSuite struct {
	ConnSuite

	controllerModelUUID string
	modelUUID           string
	st                  *state.State
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
}

type storageOpener func() (binarystorage.StorageCloser, error)

func (s *binaryStorageSuite) TestToolsStorage(c *gc.C) {
	s.testStorage(c, "toolsmetadata", s.State.ToolsStorage)
}

func (s *binaryStorageSuite) TestToolsStorageParamsControllerModel(c *gc.C) {
	s.testStorageParams(c, "toolsmetadata", []string{s.State.ModelUUID()}, s.State.ToolsStorage)
}

func (s *binaryStorageSuite) TestToolsStorageParamsHostedModel(c *gc.C) {
	s.testStorageParams(c, "toolsmetadata", []string{s.modelUUID, s.State.ModelUUID()}, s.st.ToolsStorage)
}

func (s *binaryStorageSuite) TestGUIArchiveStorage(c *gc.C) {
	s.testStorage(c, "guimetadata", s.State.GUIStorage)
}

func (s *binaryStorageSuite) TestGUIArchiveStorageParams(c *gc.C) {
	s.testStorageParams(c, "guimetadata", []string{s.controllerModelUUID}, s.st.GUIStorage)
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

func (s *binaryStorageSuite) testStorageParams(c *gc.C, collName string, uuids []string, openStorage storageOpener) {
	var uuidArgs []string
	s.PatchValue(state.BinarystorageNew, func(
		modelUUID string,
		managedStorage blobstore.ManagedStorage,
		metadataCollection mongo.Collection,
		runner jujutxn.Runner,
	) binarystorage.Storage {
		uuidArgs = append(uuidArgs, modelUUID)
		c.Assert(managedStorage, gc.NotNil)
		c.Assert(metadataCollection.Name(), gc.Equals, collName)
		c.Assert(runner, gc.NotNil)
		return nil
	})

	storage, err := openStorage()
	c.Assert(err, jc.ErrorIsNil)
	storage.Close()
	c.Assert(uuidArgs, jc.DeepEquals, uuids)
}

func (s *binaryStorageSuite) TestToolsStorageLayered(c *gc.C) {
	modelTools, err := s.st.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer modelTools.Close()

	controllerTools, err := s.State.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer controllerTools.Close()

	err = modelTools.Add(strings.NewReader("abc"), binarystorage.Metadata{Version: "1.0", Size: 3})
	c.Assert(err, jc.ErrorIsNil)
	err = controllerTools.Add(strings.NewReader("defg"), binarystorage.Metadata{Version: "1.0", Size: 4})
	c.Assert(err, jc.ErrorIsNil)
	err = controllerTools.Add(strings.NewReader("def"), binarystorage.Metadata{Version: "2.0", Size: 3})
	c.Assert(err, jc.ErrorIsNil)

	all, err := modelTools.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, jc.DeepEquals, []binarystorage.Metadata{
		{Version: "1.0", Size: 3},
		{Version: "2.0", Size: 3},
	})

	assertContents := func(v, contents string) {
		_, rc, err := modelTools.Open(v)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(rc, gc.NotNil)
		defer rc.Close()
		data, err := ioutil.ReadAll(rc)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(data), gc.Equals, contents)
	}
	assertContents("1.0", "abc")
	assertContents("2.0", "def")
}
