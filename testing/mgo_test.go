// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	stdtesting "testing"

	"labix.org/v2/mgo/bson"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
)

type mgoSuite struct {
	testing.BaseSuite
	testing.MgoSuite
}

var _ = gc.Suite(&mgoSuite{})

func TestMgoSuite(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func (s *mgoSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *mgoSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *mgoSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
}

func (s *mgoSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}

func (s *mgoSuite) TestResetWhenUnauthorized(c *gc.C) {
	session := testing.MgoServer.MustDial()
	defer session.Close()
	err := session.DB("admin").AddUser("admin", "foo", false)
	if err != nil && err.Error() != "need to login" {
		c.Assert(err, gc.IsNil)
	}
	// The test will fail if the reset does not succeed
}

func (s *mgoSuite) TestStartAndClean(c *gc.C) {
	c.Assert(testing.MgoServer.Addr(), gc.Not(gc.Equals), "")

	session := testing.MgoServer.MustDial()
	defer session.Close()
	menu := session.DB("food").C("menu")
	err := menu.Insert(
		bson.D{{"spam", "lots"}},
		bson.D{{"eggs", "fried"}},
	)
	c.Assert(err, gc.IsNil)
	food := make([]map[string]string, 0)
	err = menu.Find(nil).All(&food)
	c.Assert(err, gc.IsNil)
	c.Assert(food, gc.HasLen, 2)
	c.Assert(food[0]["spam"], gc.Equals, "lots")
	c.Assert(food[1]["eggs"], gc.Equals, "fried")

	testing.MgoServer.Reset()
	morefood := make([]map[string]string, 0)
	err = menu.Find(nil).All(&morefood)
	c.Assert(err, gc.IsNil)
	c.Assert(morefood, gc.HasLen, 0)
}
