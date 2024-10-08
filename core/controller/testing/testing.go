// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corecontroller "github.com/juju/juju/core/controller"
)

// GenControllerUUID can be used in testing for generating a controller uuid
// that is checked for subsequent errors using the test suits go check instance.
func GenControllerUUID(c *gc.C) corecontroller.UUID {
	uuid, err := corecontroller.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}
