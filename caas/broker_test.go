// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/internal/testing"
)

type brokerSuite struct {
	testing.BaseSuite
}

func TestBrokerSuite(t *stdtesting.T) {
	tc.Run(t, &brokerSuite{})
}

func (s *brokerSuite) TestDeploymentTypeValidation(c *tc.C) {

	validTypes := []caas.DeploymentType{
		caas.DeploymentStateful,
		caas.DeploymentStateless,
		caas.DeploymentDaemon,
		caas.DeploymentType(""), // TODO(caas): change deployment to mandatory.
	}
	for _, t := range validTypes {
		c.Check(t.Validate(), tc.ErrorIsNil)
	}

	c.Assert(caas.DeploymentType("bad type").Validate(), tc.ErrorIs, errors.NotSupported)
}
