// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"testing"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/tc"

	internalerrors "github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
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

// rebootDispatchSuite tests the error-dispatch logic in MachineAgent.Run that
// decides whether to call executeRebootOrShutdown. The switch uses errors.Is
// which correctly traverses Unwrap() chains; the regression was that the old
// errors.Cause()-based switch silently dropped the reboot when the error was
// wrapped by internal/errors.Capture (as it is when it travels through
// core/watcher.NotifyWorker).
type rebootDispatchSuite struct{}

func TestRebootDispatchSuite(t *testing.T) {
	tc.Run(t, &rebootDispatchSuite{})
}

// TestRebootMachineIdentifiedWhenBare confirms that a bare ErrRebootMachine
// matches the errors.Is check used in the Run() switch.
func (s *rebootDispatchSuite) TestRebootMachineIdentifiedWhenBare(c *tc.C) {
	err := internalworker.ErrRebootMachine
	c.Check(isRebootError(err), tc.IsTrue)
	c.Check(isShutdownError(err), tc.IsFalse)
}

// TestShutdownMachineIdentifiedWhenBare confirms that a bare ErrShutdownMachine
// matches the errors.Is check used in the Run() switch.
func (s *rebootDispatchSuite) TestShutdownMachineIdentifiedWhenBare(c *tc.C) {
	err := internalworker.ErrShutdownMachine
	c.Check(isShutdownError(err), tc.IsTrue)
	c.Check(isRebootError(err), tc.IsFalse)
}

// TestRebootMachineIdentifiedWhenCaptured is the regression test: when
// ErrRebootMachine has been wrapped by internal/errors.Capture (as happens
// when it travels through core/watcher.NotifyWorker), errors.Is must still
// identify it.  The old errors.Cause()-based switch did not traverse Unwrap()
// chains and therefore missed the wrapped sentinel, causing the reboot to be
// silently dropped.
func (s *rebootDispatchSuite) TestRebootMachineIdentifiedWhenCaptured(c *tc.C) {
	wrapped := internalerrors.Capture(internalworker.ErrRebootMachine)
	c.Check(isRebootError(wrapped), tc.IsTrue)
}

// TestShutdownMachineIdentifiedWhenCaptured mirrors the above for
// ErrShutdownMachine.
func (s *rebootDispatchSuite) TestShutdownMachineIdentifiedWhenCaptured(c *tc.C) {
	wrapped := internalerrors.Capture(internalworker.ErrShutdownMachine)
	c.Check(isShutdownError(wrapped), tc.IsTrue)
}

// TestOtherErrorNotMisidentified ensures that an unrelated error is not
// dispatched to either reboot or shutdown.
func (s *rebootDispatchSuite) TestOtherErrorNotMisidentified(c *tc.C) {
	wrapped := internalerrors.Capture(internalworker.ErrTerminateAgent)
	c.Check(isRebootError(wrapped), tc.IsFalse)
	c.Check(isShutdownError(wrapped), tc.IsFalse)
}

// isRebootError mirrors the first case of the switch in MachineAgent.Run.
func isRebootError(err error) bool {
	return jujuerrors.Is(err, internalworker.ErrRebootMachine)
}

// isShutdownError mirrors the second case of the switch in MachineAgent.Run.
func isShutdownError(err error) bool {
	return jujuerrors.Is(err, internalworker.ErrShutdownMachine)
}
