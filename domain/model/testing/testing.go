// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/internal/uuid"
)

// GenModelUUID can be used in testing for generating a model uuid that is
// checked for subsequent errors using the test suits go check instance.
func GenModelUUID(c *gc.C) model.UUID {
	uuid, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return model.UUID(uuid.String())
}
