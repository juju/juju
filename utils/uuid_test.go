// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils"
)

type uuidSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&uuidSuite{})

func (*uuidSuite) TestUUID(c *gc.C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	uuidCopy := uuid.Copy()
	uuidRaw := uuid.Raw()
	uuidStr := uuid.String()
	c.Assert(uuidRaw, gc.HasLen, 16)
	c.Assert(uuidStr, jc.Satisfies, utils.IsValidUUIDString)
	uuid[0] = 0x00
	uuidCopy[0] = 0xFF
	c.Assert(uuid, gc.Not(gc.DeepEquals), uuidCopy)
	uuidRaw[0] = 0xFF
	c.Assert(uuid, gc.Not(gc.DeepEquals), uuidRaw)
	nextUUID, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	c.Assert(uuid, gc.Not(gc.DeepEquals), nextUUID)
}

func (*uuidSuite) TestIsValidUUIDFailsWhenNotValid(c *gc.C) {
	c.Assert(utils.IsValidUUIDString("blah"), gc.Equals, false)
}

func (*uuidSuite) TestUUIDFromString(c *gc.C) {
	_, err := utils.UUIDFromString("blah")
	c.Assert(err, gc.ErrorMatches, `invalid UUID: "blah"`)
	validUUID := "9f484882-2f18-4fd2-967d-db9663db7bea"
	uuid, err := utils.UUIDFromString(validUUID)
	c.Assert(err, gc.IsNil)
	c.Assert(uuid.String(), gc.Equals, validUUID)
}
