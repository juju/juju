// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/mgo/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/imagestorage"
)

var _ = gc.Suite(&ImageSuite{})

type ImageSuite struct {
	ConnSuite
}

func (s *ImageSuite) TestStorage(c *gc.C) {
	session := s.State.MongoSession()
	collectionNames, err := session.DB("osimages").CollectionNames()
	c.Assert(err, gc.IsNil)
	nameSet := set.NewStrings(collectionNames...)
	c.Assert(nameSet.Contains("imagemetadata"), jc.IsFalse)

	storage := s.State.ImageStorage()
	err = storage.AddImage(strings.NewReader(""), &imagestorage.Metadata{})
	c.Assert(err, gc.IsNil)

	collectionNames, err = session.DB("osimages").CollectionNames()
	c.Assert(err, gc.IsNil)
	nameSet = set.NewStrings(collectionNames...)
	c.Assert(nameSet.Contains("imagemetadata"), jc.IsTrue)
}

func (s *ImageSuite) TestStorageParams(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, gc.IsNil)

	var called bool
	s.PatchValue(state.ImageStorageNewStorage, func(
		session *mgo.Session,
		modelUUID string,
	) imagestorage.Storage {
		called = true
		c.Assert(modelUUID, gc.Equals, model.UUID())
		c.Assert(session, gc.NotNil)
		return nil
	})

	s.State.ImageStorage()
	c.Assert(called, jc.IsTrue)
}
