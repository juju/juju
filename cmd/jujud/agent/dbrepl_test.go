// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/internal/controllerruntimeconfig"
	coretesting "github.com/juju/juju/internal/testing"
)

// writeRuntimeConf writes a minimal but valid controller runtime
// configuration with the given controller ID into dir and returns the
// path it was written to. Every field checked by
// ControllerRuntimeConfig.Validate is populated, so the file passes the
// reader's validation. The safemode tests share this helper.
func writeRuntimeConf(c *tc.C, dir, controllerID string) string {
	conf := fmt.Sprintf(`
controller-id: %[1]q
controller-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d
controller-model-uuid: feedface-dead-beef-cafe-c0ffee000000
data-dir: %[2]s
log-dir: %[2]s
socket-dir: %[2]s
shared-agent-dir: %[2]s
api-port: 17070
agent-password: agent-password
ca-cert: ca-cert-pem
ca-private-key: ca-private-key-pem
controller-cert: controller-cert-pem
controller-private-key: controller-private-key-pem
`, controllerID, dir)

	path := filepath.Join(dir, controllerruntimeconfig.Filename)
	err := os.WriteFile(path, []byte(conf), 0o600)
	c.Assert(err, tc.ErrorIsNil)
	return path
}

// mustNotBeCalledDBReplFactory returns an error if it is ever invoked:
// the db-repl Init path must not instantiate the repl controller agent.
func mustNotBeCalledDBReplFactory(
	names.Tag,
	controllerruntimeconfig.ControllerRuntimeConfig,
) (*replControllerAgent, error) {
	return nil, errors.New("repl controller agent factory must not be invoked")
}

type dbreplInitSuite struct {
	coretesting.BaseSuite
}

func TestDBReplInitSuite(t *testing.T) {
	tc.Run(t, &dbreplInitSuite{})
}

func (s *dbreplInitSuite) newCommand(c *tc.C, dataDir string) cmd.Command {
	return NewDBReplAgentCommand(
		cmdtesting.Context(c),
		mustNotBeCalledDBReplFactory,
		agentconf.NewAgentConf(dataDir),
	)
}

func (s *dbreplInitSuite) TestInitDerivesControllerIDFromRuntimeConf(c *tc.C) {
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
	dbreplCommand := command.(*dbReplAgentCommand)
	c.Check(dbreplCommand.agentTag.Id(), tc.Equals, "0")
	c.Check(dbreplCommand.agentTag.String(), tc.Equals, "controller-0")
	c.Check(dbreplCommand.runtimeConfig.ControllerID, tc.Equals, "0")
}

func (s *dbreplInitSuite) TestInitMissingRuntimeConf(c *tc.C) {
	// With the --controller-id flag removed, a missing runtime.conf is
	// the primary input error: it must surface as the reader's
	// annotated error, naming the file it could not read.
	dataDir := c.MkDir()

	command := s.newCommand(c, dataDir)
	err := cmdtesting.InitCommand(command, []string{"--data-dir", dataDir})
	c.Assert(err, tc.ErrorMatches,
		`cannot read controller runtime configuration: reading controller runtime config ".*": open .*: no such file or directory`)
}

func (s *dbreplInitSuite) TestInitInvalidControllerID(c *tc.C) {
	dataDir := c.MkDir()
	writeRuntimeConf(c, dataDir, "bogus")

	command := s.newCommand(c, dataDir)
	// The reader validates the controller ID before the command derives
	// its agent tag, so a malformed runtime.conf must yield a
	// validation error rather than the panic that
	// names.NewControllerAgentTag raises for a non-numeric ID.
	err := cmdtesting.InitCommand(command, []string{"--data-dir", dataDir})
	c.Assert(err, tc.ErrorMatches,
		`cannot read controller runtime configuration: validating controller runtime config ".*": controller ID "bogus" not valid`)
}

func (s *dbreplInitSuite) TestInitControllerIdFlagRemoved(c *tc.C) {
	dataDir := c.MkDir()
	writeRuntimeConf(c, dataDir, "0")

	command := s.newCommand(c, dataDir)
	// The --controller-id flag was removed: the controller ID is read
	// from runtime.conf, so passing the flag must fail at parse time.
	err := cmdtesting.InitCommand(command, []string{"--controller-id", "0"})
	c.Assert(err, tc.ErrorMatches, `option provided but not defined: --controller-id`)
}
