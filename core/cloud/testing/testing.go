// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corecloud "github.com/juju/juju/core/cloud"
)

// GenModelUUID can be used in testing for generating a model uuid that is
// checked for subsequent errors using the test suits go check instance.
func GenCloudID(c *gc.C) corecloud.ID {
	uuid, err := corecloud.NewID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}
