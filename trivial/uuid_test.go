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
	c.Assert(trivial.IsValidUUIDString(uuidStr), Equals, true)
	uuid[0] = 0x00
	uuidCopy[0] = 0xFF
	c.Assert(uuid, Not(DeepEquals), uuidCopy)
	uuidRaw[0] = 0xFF
	c.Assert(uuid, Not(DeepEquals), uuidRaw)
	nextUUID, err := trivial.NewUUID()
	c.Assert(err, IsNil)
	c.Assert(uuid, Not(DeepEquals), nextUUID)
}

func (uuidSuite) TestIsValidUUIDFailsWhenNotValid(c *C) {
	c.Assert(trivial.IsValidUUIDString("blah"), Equals, false)
}

func (uuidSuite) TestUUIDFromString(c *C) {
	_, err := trivial.UUIDFromString("blah")
	c.Assert(err, ErrorMatches, `invalid UUID: "blah"`)
	validUUID := "9f484882-2f18-4fd2-967d-db9663db7bea"
	uuid, err := trivial.UUIDFromString(validUUID)
	c.Assert(err, IsNil)
	c.Assert(uuid.String(), Equals, validUUID)
}
