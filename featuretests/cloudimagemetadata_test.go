// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/state/testing"
)

type cloudImageMetadataSuite struct {
	testing.StateSuite
}

func (s *cloudImageMetadataSuite) TestSaveAndFindMetadata(c *gc.C) {
	metadata, err := s.State.CloudImageMetadataStorage.FindMetadata(cloudimagemetadata.MetadataAttributes{})
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, "matching cloud image metadata not found")
	c.Assert(metadata, gc.HasLen, 0)

	// check db too
	conn := s.State.MongoSession()
	coll, closer := mongo.CollectionFromName(conn.DB("juju"), "cloudimagemetadata")
	defer closer()

	before, err := coll.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(before == 0, jc.IsTrue)

	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region",
		Series:          "series",
		Arch:            "arch",
		VirtualType:     "virtType",
		RootStorageType: "rootStorageType"}

	m := cloudimagemetadata.Metadata{attrs, "1"}
	err = s.State.CloudImageMetadataStorage.SaveMetadata(m)
	c.Assert(err, jc.ErrorIsNil)

	added, err := s.State.CloudImageMetadataStorage.FindMetadata(attrs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(added, jc.DeepEquals,
		map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata{
			m.Source: {m}})

	// make sure it's in db too
	after, err := coll.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(after == 1, jc.IsTrue)
}
