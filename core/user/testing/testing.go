// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreuser "github.com/juju/juju/core/user"
)

// GenUserUUID can be used in testing for generating a user uuid that is
// checked for subsequent errors using the test suits go check instance.
func GenUserUUID(c *gc.C) coreuser.UUID {
	uuid, err := coreuser.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}
