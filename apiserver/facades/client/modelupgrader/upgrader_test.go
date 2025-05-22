// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type modelUpgradeSuite struct {
	testhelpers.IsolationSuite
}

func TestModelUpgradeSuite(t *stdtesting.T) {
	tc.Run(t, &modelUpgradeSuite{})
}

func (*modelUpgradeSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Upgrade a model that both as an invalid tag and also does not match the model uuid of the current facade scope.
- Upgrading a model when you don't have permission to.
- Upgrading a model when the block checker says that it is not allowed.
- Happy path test of upgrading the controller model. Including dry run.
- Upgrading a model when the controller that it lives in is dying.
- Upgrade validation checks fail for the controller model.
- Upgrading a model to a different version that doesn't match the controller.
- Upgrading a model to the same version as the controller.
- Upgrade a model and fail validation checks.
- Upgrade a model in the above cases with dry run as well.
- Test upgrading past controller version fails.
- Test default decisions around version for upgrades to a model and also arch for caas.
`)
}

func (s *modelUpgradeSuite) SetUpTest(c *tc.C) {
}
