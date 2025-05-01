// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremachine "github.com/juju/juju/core/machine"
)

// GenUUID can be used in testing for generating a machine uuid that is
// checked for errors using the test suit's go check instance.
func GenUUID(c *gc.C) coremachine.UUID {
	uuid, err := coremachine.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}
