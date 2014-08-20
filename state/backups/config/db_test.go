// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	"github.com/juju/names"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/backups/config"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&dbSuite{})

type dbSuite struct {
	testing.BaseSuite
}

func (s *sourcesSuite) TestDBConnInfoNewDBConnInfo(c *gc.C) {
	connInfo := config.NewDBConnInfo("a", "b", "c")

	c.Check(connInfo.Address(), gc.Equals, "a")
	c.Check(connInfo.Username(), gc.Equals, "b")
	c.Check(connInfo.Password(), gc.Equals, "c")
}

func (s *sourcesSuite) TestDBConnInfoUpdateFromMongoInfoOkay(c *gc.C) {
	tag, err := names.ParseTag("machine-0")
	c.Assert(err, gc.IsNil)
	mgoInfo := authentication.MongoInfo{
		Info: mongo.Info{
			Addrs: []string{"localhost:8080"},
		},
		Tag:      tag,
		Password: "eggs",
	}

	connInfo := config.NewDBConnInfo("a", "b", "c")
	connInfo.UpdateFromMongoInfo(&mgoInfo)
	addr, user, pw, err := connInfo.Check()
	c.Assert(err, gc.IsNil)

	c.Check(addr, gc.Equals, "localhost:8080")
	c.Check(user, gc.Equals, "machine-0")
	c.Check(pw, gc.Equals, "eggs")
}

func (s *sourcesSuite) TestDBConnInfoUpdateFromMongoInfoMissingTag(c *gc.C) {
	mgoInfo := authentication.MongoInfo{
		Info: mongo.Info{
			Addrs: []string{"localhost:8080"},
		},
		Password: "eggs",
	}

	connInfo := config.NewDBConnInfo("a", "b", "c")
	connInfo.UpdateFromMongoInfo(&mgoInfo)
	addr, user, pw, err := connInfo.Check()
	c.Assert(err, gc.IsNil)

	c.Check(addr, gc.Equals, "localhost:8080")
	c.Check(user, gc.Equals, "b")
	c.Check(pw, gc.Equals, "eggs")
}

func (s *sourcesSuite) TestDBConnInfoCheckOkay(c *gc.C) {
	connInfo := config.NewDBConnInfo("a", "b", "c")
	addr, user, pw, err := connInfo.Check()
	c.Assert(err, gc.IsNil)

	c.Check(addr, gc.Equals, "a")
	c.Check(user, gc.Equals, "b")
	c.Check(pw, gc.Equals, "c")
}

func (s *sourcesSuite) TestDBConnInfoCheckMissing(c *gc.C) {
	connInfo := config.NewDBConnInfo("a", "b", "")
	_, _, _, err := connInfo.Check()

	c.Check(err, gc.ErrorMatches, "missing password")
}
