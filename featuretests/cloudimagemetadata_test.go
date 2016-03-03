// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/imagemetadata"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/rpc"
)

type cloudImageMetadataSuite struct {
	testing.JujuConnSuite

	client *imagemetadata.Client
}

func (s *cloudImageMetadataSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.client = imagemetadata.NewClient(s.APIState)
	c.Assert(s.client, gc.NotNil)
}

func (s *cloudImageMetadataSuite) TearDownTest(c *gc.C) {
	s.client.ClientFacade.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *cloudImageMetadataSuite) TestSaveAndFindAndDeleteMetadata(c *gc.C) {
	metadata, err := s.client.List("", "", nil, nil, "", "")
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: "matching cloud image metadata not found",
		Code:    "not found",
	})
	c.Assert(metadata, gc.HasLen, 0)

	//	check db too
	conn := s.State.MongoSession()
	coll, closer := mongo.CollectionFromName(conn.DB("juju"), "cloudimagemetadata")
	defer closer()

	before, err := coll.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(before == 0, jc.IsTrue)

	imageId := "1"
	m := params.CloudImageMetadata{
		Source:          "custom",
		Stream:          "stream",
		Region:          "region",
		Series:          "trusty",
		Arch:            "arch",
		VirtType:        "virtType",
		RootStorageType: "rootStorageType",
		ImageId:         imageId,
	}

	err = s.client.Save([]params.CloudImageMetadata{m})
	c.Assert(err, jc.ErrorIsNil)

	added, err := s.client.List("", "", nil, nil, "", "")
	c.Assert(err, jc.ErrorIsNil)

	// m.Version would be deduced from m.Series
	m.Version = "14.04"
	c.Assert(added, jc.DeepEquals, []params.CloudImageMetadata{m})

	// make sure it's in db too
	after, err := coll.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(after == 1, jc.IsTrue)

	err = s.client.Delete(imageId)
	c.Assert(err, jc.ErrorIsNil)
	// make sure it's no longer in db too
	afterDelete, err := coll.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(afterDelete, gc.Equals, 0)
}
