// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"strings"

	"github.com/juju/blobstore"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/imagestorage"
)

var _ = gc.Suite(&ImageSuite{})

type ImageSuite struct {
	ConnSuite
}

func (s *ImageSuite) TestStorage(c *gc.C) {
	session := s.State.MongoSession()
	collectionNames, err := session.DB("juju").CollectionNames()
	c.Assert(err, gc.IsNil)
	nameSet := set.NewStrings(collectionNames...)
	c.Assert(nameSet.Contains("imagemetadata"), jc.IsFalse)

	storage, err := s.State.ImageStorage()
	c.Assert(err, gc.IsNil)
	defer func() {
		err := storage.Close()
		c.Assert(err, gc.IsNil)
	}()

	err = storage.AddImage(strings.NewReader(""), &imagestorage.Metadata{})
	c.Assert(err, gc.IsNil)

	collectionNames, err = session.DB("juju").CollectionNames()
	c.Assert(err, gc.IsNil)
	nameSet = set.NewStrings(collectionNames...)
	c.Assert(nameSet.Contains("imagemetadata"), jc.IsTrue)
}

func (s *ImageSuite) TestStorageParams(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)

	var called bool
	s.PatchValue(state.ImageStorageNewStorage, func(
		envUUID string,
		managedStorage blobstore.ManagedStorage,
		metadataCollection *mgo.Collection,
		runner jujutxn.Runner,
	) imagestorage.Storage {
		called = true
		c.Assert(envUUID, gc.Equals, env.UUID())
		c.Assert(managedStorage, gc.NotNil)
		c.Assert(metadataCollection.Name, gc.Equals, "imagemetadata")
		c.Assert(runner, gc.NotNil)
		return nil
	})

	storage, err := s.State.ImageStorage()
	c.Assert(err, gc.IsNil)
	storage.Close()
	c.Assert(called, jc.IsTrue)
}
