package testing_test

import (
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
	stdtesting "testing"
)

type mgoSuite struct{}

var _ = Suite(&mgoSuite{})

func TestMgoSuite(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func (s *mgoSuite) TestMgoStartAndClean(c *C) {
	c.Assert(testing.MgoAddr, Not(Equals), "")

	session := testing.MgoDial()
	menu := session.DB("food").C("menu")
	err := menu.Insert(
		bson.D{{"spam", "lots"}},
		bson.D{{"eggs", "fried"}},
	)
	c.Assert(err, IsNil)
	food := make([]map[string]string, 0)
	err = menu.Find(nil).All(&food)
	c.Assert(err, IsNil)
	c.Assert(food, HasLen, 2)
	c.Assert(food[0]["spam"], Equals, "lots")
	c.Assert(food[1]["eggs"], Equals, "fried")

	testing.MgoReset()
	morefood := make([]map[string]string, 0)
	err = menu.Find(nil).All(&morefood)
	c.Assert(err, IsNil)
	c.Assert(morefood, HasLen, 0)
}
