// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/model"
)

// GenModelUUID can be used in testing for generating a model uuid that is
// checked for subsequent errors using the test suits go check instance.
func GenModelUUID(c *gc.C) model.UUID {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return model.UUID(uuid.String())
}
