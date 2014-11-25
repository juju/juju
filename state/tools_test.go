// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"strings"

	"github.com/juju/blobstore"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/toolstorage"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type tooler interface {
	lifer
	AgentTools() (*tools.Tools, error)
	SetAgentVersion(v version.Binary) error
	Refresh() error
}

func newTools(vers, url string) *tools.Tools {
	return &tools.Tools{
		Version: version.MustParseBinary(vers),
		URL:     url,
		Size:    10,
		SHA256:  "1234",
	}
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

var _ = gc.Suite(&ToolsSuite{})

type ToolsSuite struct {
	ConnSuite
}

func (s *ToolsSuite) TestStorage(c *gc.C) {
	session := s.State.MongoSession()
	collectionNames, err := session.DB("juju").CollectionNames()
	c.Assert(err, jc.ErrorIsNil)
	nameSet := set.NewStrings(collectionNames...)
	c.Assert(nameSet.Contains("toolsmetadata"), jc.IsFalse)

	storage, err := s.State.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := storage.Close()
		c.Assert(err, jc.ErrorIsNil)
	}()

	err = storage.AddTools(strings.NewReader(""), toolstorage.Metadata{})
	c.Assert(err, jc.ErrorIsNil)

	collectionNames, err = session.DB("juju").CollectionNames()
	c.Assert(err, jc.ErrorIsNil)
	nameSet = set.NewStrings(collectionNames...)
	c.Assert(nameSet.Contains("toolsmetadata"), jc.IsTrue)
}

func (s *ToolsSuite) TestStorageParams(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)

	var called bool
	s.PatchValue(state.ToolstorageNewStorage, func(
		envUUID string,
		managedStorage blobstore.ManagedStorage,
		metadataCollection *mgo.Collection,
		runner jujutxn.Runner,
	) toolstorage.Storage {
		called = true
		c.Assert(envUUID, gc.Equals, env.UUID())
		c.Assert(managedStorage, gc.NotNil)
		c.Assert(metadataCollection.Name, gc.Equals, "toolsmetadata")
		c.Assert(runner, gc.NotNil)
		return nil
	})

	storage, err := s.State.ToolsStorage()
	c.Assert(err, jc.ErrorIsNil)
	storage.Close()
	c.Assert(called, jc.IsTrue)
}
