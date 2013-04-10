package trivial_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/trivial"
)

type uuidSuite struct{}

var _ = Suite(uuidSuite{})

func (uuidSuite) TestUUID(c *C) {
	uuid, err := trivial.NewUUID()
	c.Assert(err, IsNil)
	uuidCopy := uuid.Copy()
	uuidRaw := uuid.Raw()
	uuidStr := uuid.String()
	c.Assert(uuidRaw, HasLen, 16)
	c.Assert(uuidStr, Matches, trivial.ValidUUIDString)
	uuid[0] = 0x00
	uuidCopy[0] = 0xFF
	c.Assert(uuid, Not(DeepEquals), uuidCopy)
	uuidRaw[0] = 0xFF
	c.Assert(uuid, Not(DeepEquals), uuidRaw)
	nextUUID, err := trivial.NewUUID()
	c.Assert(err, IsNil)
	c.Assert(uuid, Not(DeepEquals), nextUUID)
}
