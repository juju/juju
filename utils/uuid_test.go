// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/utils"
)

type uuidSuite struct{}

var _ = Suite(uuidSuite{})

func (uuidSuite) TestUUID(c *C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, IsNil)
	uuidCopy := uuid.Copy()
	uuidRaw := uuid.Raw()
	uuidStr := uuid.String()
	c.Assert(uuidRaw, HasLen, 16)
	c.Assert(uuidStr, checkers.Satisfies, utils.IsValidUUIDString)
	uuid[0] = 0x00
	uuidCopy[0] = 0xFF
	c.Assert(uuid, Not(DeepEquals), uuidCopy)
	uuidRaw[0] = 0xFF
	c.Assert(uuid, Not(DeepEquals), uuidRaw)
	nextUUID, err := utils.NewUUID()
	c.Assert(err, IsNil)
	c.Assert(uuid, Not(DeepEquals), nextUUID)
}

func (uuidSuite) TestIsValidUUIDFailsWhenNotValid(c *C) {
	c.Assert(utils.IsValidUUIDString("blah"), Equals, false)
}

func (uuidSuite) TestUUIDFromString(c *C) {
	_, err := utils.UUIDFromString("blah")
	c.Assert(err, ErrorMatches, `invalid UUID: "blah"`)
	validUUID := "9f484882-2f18-4fd2-967d-db9663db7bea"
	uuid, err := utils.UUIDFromString(validUUID)
	c.Assert(err, IsNil)
	c.Assert(uuid.String(), Equals, validUUID)
}
