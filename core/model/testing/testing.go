// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
)

// GenModelUUID can be used in testing for generating a model uuid that is
// checked for subsequent errors using the test suits go check instance.
func GenModelUUID(c *gc.C) coremodel.UUID {
	uuid, err := coremodel.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}
