// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package types

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
)

type modelSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&modelSuite{})

func (s *modelSuite) TestParity(c *gc.C) {
	// Ensure that we have parity with the model types in core package.
	c.Check(IAAS.String(), gc.Equals, model.IAAS.String())
	c.Check(CAAS.String(), gc.Equals, model.CAAS.String())
}
