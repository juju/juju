// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package types

import (
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/core/model"
)

type modelSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&modelSuite{})

func (s *modelSuite) TestParity(c *tc.C) {
	// Ensure that we have parity with the model types in core package.
	c.Check(IAAS.String(), tc.Equals, model.IAAS.String())
	c.Check(CAAS.String(), tc.Equals, model.CAAS.String())
}
