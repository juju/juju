// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&connInfoSuite{})

type connInfoSuite struct {
	testing.BaseSuite
}

func (s *connInfoSuite) TestDBConnInfoValidateOkay(c *gc.C) {
	connInfo := &backups.DBConnInfo{"a", "b", "c"}
	err := connInfo.Validate()

	c.Check(err, gc.IsNil)
}

func (s *connInfoSuite) TestDBConnInfoCheckMissingAddress(c *gc.C) {
	connInfo := &backups.DBConnInfo{"", "b", "c"}
	err := connInfo.Validate()

	c.Check(err, gc.ErrorMatches, "missing address")
}

func (s *connInfoSuite) TestDBConnInfoCheckMissingUsername(c *gc.C) {
	connInfo := &backups.DBConnInfo{"a", "", "c"}
	err := connInfo.Validate()

	c.Check(err, gc.ErrorMatches, "missing username")
}

func (s *connInfoSuite) TestDBConnInfoCheckMissingPassword(c *gc.C) {
	connInfo := &backups.DBConnInfo{"a", "b", ""}
	err := connInfo.Validate()

	c.Check(err, gc.ErrorMatches, "missing password")
}

func (s *connInfoSuite) TestNewMongoConnInfoOkay(c *gc.C) {
	tag, err := names.ParseTag("machine-0")
	c.Assert(err, gc.IsNil)
	mgoInfo := mongo.MongoInfo{
		Info: mongo.Info{
			Addrs: []string{"localhost:8080"},
		},
		Tag:      tag,
		Password: "eggs",
	}

	connInfo := backups.NewMongoConnInfo(&mgoInfo)
	err = connInfo.Validate()
	c.Assert(err, gc.IsNil)

	c.Check(connInfo.Address, gc.Equals, "localhost:8080")
	c.Check(connInfo.Username, gc.Equals, "machine-0")
	c.Check(connInfo.Password, gc.Equals, "eggs")
}

func (s *connInfoSuite) TestNewMongoConnInfoMissingTag(c *gc.C) {
	mgoInfo := mongo.MongoInfo{
		Info: mongo.Info{
			Addrs: []string{"localhost:8080"},
		},
		Password: "eggs",
	}

	connInfo := backups.NewMongoConnInfo(&mgoInfo)
	err := connInfo.Validate()

	c.Check(err, gc.ErrorMatches, "missing username")
}
