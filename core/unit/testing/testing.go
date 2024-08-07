// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreunit "github.com/juju/juju/core/unit"
)

// GenUnitID can be used in testing for generating a unit id that is checked
// for subsequent errors using the test suits go check instance.
func GenUnitID(c *gc.C) coreunit.ID {
	uuid, err := coreunit.NewID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}
