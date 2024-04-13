// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/provider/caas"
	"github.com/juju/juju/testing"
)

type brokerSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&brokerSuite{})

func (s *brokerSuite) TestDeploymentTypeValidation(c *gc.C) {

	validTypes := []caas.DeploymentType{
		caas.DeploymentStateful,
		caas.DeploymentStateless,
		caas.DeploymentDaemon,
		caas.DeploymentType(""), // TODO(caas): change deployment to mandatory.
	}
	for _, t := range validTypes {
		c.Check(t.Validate(), jc.ErrorIsNil)
	}

	c.Assert(caas.DeploymentType("bad type").Validate(), jc.ErrorIs, errors.NotSupported)
}
