// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"testing"

	"github.com/juju/tc"
)

type MachineSuite struct {
}

func TestMachineSuite(t *testing.T) {
	tc.Run(t, &MachineSuite{})
}

func (s *MachineSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:

- Test parsing invalid machine-id and controller-id
- Test parsing valid machine-id and controller-id
- Test ensure that the stderr is a lumberjack logger - essentially that the machine log can be rotated
- Test that the lumberjack logger is not used when the logToStdErr flag is set
- Test that the agent can run then stopped
- Test that the agent can be upgraded (goes the upgrade request flow: stop, upgrade, start)
- Test that no upgrade is required when the agent is already at the latest version
- Test that the agent sets the tools version for manage-model
- Test that the agent sets the tools version for host units
- Test that machine agent runs the disk manager worker (this could be generalised for all known workers)
- Test that certificate DNS names are updated when the agent starts
- Test that certificate DNS names are updated when the agent starts with an invalid private key
- Test that the agent ignores addresses when ignore-machine-addresses is set to true
- Test that the agent does not ignore addresses when ignore-machine-addresses is set to false
- Test that the agent ignores addresses when running in a container
- Test that all machine workers are started
`)
}

func (s *MachineSuite) TestIntegrationStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Testing that the controller runs the cleaner worker by removing an application and watching it's unit disapear.
  This is a very silly test.
- Testing that the controller runs the instance poller by doing a great song and dance to add tools, deploy units of an
  application etc using the dummy provider. It then checks that the deployed machine's addresses are updated by said
  poller. This is also a very silly test.
- Test that the audit log is written to the correct location with the correct calls.
- Test that the hosted model workers are started, with the correct set of workers.
- Test that the hosted model handles the case where the cloud credential is invalid.
- Test that the hosted model handles the case where the cloud credential is deleted.
- Test that the hosted model workers are started, when the cloud credential becomes valid.
- Test that the migrating model workers are started, with the correct set of workers.
- Test that the dying model workers are cleaned up, when the model is destroyed.
- Test that machine agent symlinks are created in the correct location.
- Test juju-exec symlink is created in the correct location.
- Test controller model workers are started, with the correct set of workers.
- Test model workers respect the singular responsibility flag, by claiming the lease for the model and checking that
  the correct set of workers are started.
`)
}
