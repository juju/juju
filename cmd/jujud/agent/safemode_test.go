// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"net"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/internal/controllerruntimeconfig"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type safemodeSuite struct {
	testhelpers.IsolationSuite
}

func TestSafemodeSuite(t *testing.T) {
	tc.Run(t, &safemodeSuite{})
}

func (s *safemodeSuite) TestEnsuringControllerNotRunningPortUnset(c *tc.C) {
	err := ensuringControllerNotRunning(0)
	c.Check(err, tc.ErrorIsNil)
}

func (s *safemodeSuite) TestEnsuringControllerNotRunningDialTimeout(c *tc.C) {
	// Find an unused port that has no listener.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, tc.ErrorIsNil)
	port := l.Addr().(*net.TCPAddr).Port
	c.Assert(l.Close(), tc.ErrorIsNil)

	// Dial to a port with no listener should timeout and return nil.
	err = ensuringControllerNotRunning(port)
	c.Check(err, tc.ErrorIsNil)
}

func (s *safemodeSuite) TestEnsuringControllerNotRunningControllerActive(c *tc.C) {
	// Start a listener on a random port to simulate a running controller.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, tc.ErrorIsNil)
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	err = ensuringControllerNotRunning(port)
	c.Check(err, tc.ErrorIs, errors.AlreadyExists)
}

// mustNotBeCalledSafeModeFactory returns an error if it is ever
// invoked: the safe-mode Init path must not instantiate the safe-mode
// controller application.
func mustNotBeCalledSafeModeFactory(
	names.Tag,
	controllerruntimeconfig.ControllerRuntimeConfig,
) (*SafeModeControllerApplication, error) {
	return nil, errors.New("safe-mode controller agent factory must not be invoked")
}

type safeModeInitSuite struct {
	coretesting.BaseSuite
}

func TestSafeModeInitSuite(t *testing.T) {
	tc.Run(t, &safeModeInitSuite{})
}

func (s *safeModeInitSuite) newCommand(c *tc.C, dataDir string) cmd.Command {
	return NewSafeModeApplicationCommand(
		cmdtesting.Context(c),
		mustNotBeCalledSafeModeFactory,
		agentconf.NewAgentConf(dataDir),
	)
}

func (s *safeModeInitSuite) TestInitDerivesControllerIDFromRuntimeConf(c *tc.C) {
	dataDir := c.MkDir()
	writeRuntimeConf(c, dataDir, "0")

	command := s.newCommand(c, dataDir)
	// The data dir is passed via the real --data-dir flag, as in
	// production: registering the flag resets the configured value to
	// the compiled-in default.
	err := cmdtesting.InitCommand(command, []string{"--data-dir", dataDir})
	c.Assert(err, tc.ErrorIsNil)

	// The controller ID is no longer taken from the command line: the
	// derived agent tag must identify controller 0 from runtime.conf.
	safemodeCommand := command.(*safeModeApplicationCommand)
	c.Check(safemodeCommand.agentTag.Id(), tc.Equals, "0")
	c.Check(safemodeCommand.agentTag.String(), tc.Equals, "controller-0")
	c.Check(safemodeCommand.runtimeConfig.ControllerID, tc.Equals, "0")
}

func (s *safeModeInitSuite) TestInitMissingRuntimeConf(c *tc.C) {
	// With the --controller-id flag removed, a missing runtime.conf is
	// the primary input error: it must surface as the reader's
	// annotated error, naming the file it could not read.
	dataDir := c.MkDir()

	command := s.newCommand(c, dataDir)
	err := cmdtesting.InitCommand(command, []string{"--data-dir", dataDir})
	c.Assert(err, tc.ErrorMatches,
		`cannot read controller runtime config: reading controller runtime config ".*": open .*: no such file or directory`)
}

func (s *safeModeInitSuite) TestInitInvalidControllerID(c *tc.C) {
	dataDir := c.MkDir()
	writeRuntimeConf(c, dataDir, "bogus")

	command := s.newCommand(c, dataDir)
	// The reader validates the controller ID before the command derives
	// its agent tag, so a malformed runtime.conf must yield a
	// validation error rather than the panic that
	// names.NewControllerAgentTag raises for a non-numeric ID.
	err := cmdtesting.InitCommand(command, []string{"--data-dir", dataDir})
	c.Assert(err, tc.ErrorMatches,
		`cannot read controller runtime config: validating controller runtime config ".*": controller ID "bogus" not valid`)
}

func (s *safeModeInitSuite) TestInitControllerIDFlagRemoved(c *tc.C) {
	dataDir := c.MkDir()
	writeRuntimeConf(c, dataDir, "0")

	command := s.newCommand(c, dataDir)
	// The --controller-id flag was removed: the controller ID is read
	// from runtime.conf, so passing the flag must fail at parse time.
	err := cmdtesting.InitCommand(command, []string{"--controller-id", "0"})
	c.Assert(err, tc.ErrorMatches, `option provided but not defined: --controller-id`)
}
