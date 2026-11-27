// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudconfig_test

import (
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/cloudconfig"
	coretesting "github.com/juju/juju/internal/testing"
)

type agentCmdSuite struct {
	coretesting.BaseSuite
}

func TestAgentCmdSuite(t *stdtesting.T) {
	tc.Run(t, &agentCmdSuite{})
}

func (*agentCmdSuite) TestAgentStartCommand(c *tc.C) {
	base := cloudconfig.AgentStartOptions{
		ToolPath:      "/var/lib/juju/tools/jujuagentd",
		DataDir:       "'/var/lib/juju'",
		LoggingOption: "--debug",
		Timeout:       10 * time.Minute,
	}
	withFlags := func(o cloudconfig.AgentStartOptions) cloudconfig.AgentStartOptions {
		o.FeatureFlagsEnv = "JUJU_DEV_FEATURE_FLAGS=special"
		return o
	}
	withMachineID := func(o cloudconfig.AgentStartOptions) cloudconfig.AgentStartOptions {
		o.MachineID = "7"
		return o
	}
	recovery := func(o cloudconfig.AgentStartOptions) cloudconfig.AgentStartOptions {
		o.RecoveryMode = true
		return o
	}

	tests := []struct {
		about    string
		options  cloudconfig.AgentStartOptions
		expected string
	}{{
		about:   "fresh bootstrap",
		options: base,
		expected: "/var/lib/juju/tools/jujuagentd bootstrap-state " +
			"--data-dir '/var/lib/juju' --debug --timeout 10m0s",
	}, {
		about:   "fresh bootstrap ignores machine id",
		options: withMachineID(base),
		expected: "/var/lib/juju/tools/jujuagentd bootstrap-state " +
			"--data-dir '/var/lib/juju' --debug --timeout 10m0s",
	}, {
		about:   "fresh bootstrap with feature flags",
		options: withFlags(base),
		expected: "JUJU_DEV_FEATURE_FLAGS=special " +
			"/var/lib/juju/tools/jujuagentd bootstrap-state " +
			"--data-dir '/var/lib/juju' --debug --timeout 10m0s",
	}, {
		about:   "fresh bootstrap with feature flags ignores machine id",
		options: withFlags(withMachineID(base)),
		expected: "JUJU_DEV_FEATURE_FLAGS=special " +
			"/var/lib/juju/tools/jujuagentd bootstrap-state " +
			"--data-dir '/var/lib/juju' --debug --timeout 10m0s",
	}, {
		about:   "recovery",
		options: recovery(base),
		expected: "/var/lib/juju/tools/jujuagentd recover-state " +
			"--data-dir '/var/lib/juju' --debug",
	}, {
		about:   "recovery with machine id",
		options: recovery(withMachineID(base)),
		expected: "/var/lib/juju/tools/jujuagentd recover-state " +
			"--data-dir '/var/lib/juju' --debug --machine-id 7",
	}, {
		about:   "recovery with feature flags",
		options: recovery(withFlags(base)),
		expected: "JUJU_DEV_FEATURE_FLAGS=special " +
			"/var/lib/juju/tools/jujuagentd recover-state " +
			"--data-dir '/var/lib/juju' --debug",
	}, {
		about:   "recovery with machine id and feature flags",
		options: recovery(withFlags(withMachineID(base))),
		expected: "JUJU_DEV_FEATURE_FLAGS=special " +
			"/var/lib/juju/tools/jujuagentd recover-state " +
			"--data-dir '/var/lib/juju' --debug --machine-id 7",
	}}

	for _, test := range tests {
		got := cloudconfig.AgentStartCommand(test.options)
		c.Check(got, tc.Equals, test.expected,
			tc.Commentf("case %q", test.about))
		// Each subcommand defines only its own mode's flags: recover-state
		// has no --timeout and bootstrap-state has no --machine-id, so
		// passing either crash-loops the agent on flag parse failure. Both
		// decisions live in AgentStartCommand, so this pairing cannot drift.
		c.Check(strings.Contains(got, "--timeout"), tc.Equals,
			!test.options.RecoveryMode, tc.Commentf("case %q", test.about))
		c.Check(strings.Contains(got, "--machine-id"), tc.Equals,
			test.options.RecoveryMode && test.options.MachineID != "",
			tc.Commentf("case %q", test.about))
	}
}

func (*agentCmdSuite) TestRecoveryMarkerCommand(c *tc.C) {
	c.Check(cloudconfig.RecoveryMarkerCommand("'/var/lib/juju/recovery/params'"),
		tc.Equals, "install -D -m 600 /dev/null '/var/lib/juju/recovery/params'")
	c.Check(cloudconfig.RecoveryMarkerCommand(`"$JUJU_DATA_DIR/recovery/params"`),
		tc.Equals, `install -D -m 600 /dev/null "$JUJU_DATA_DIR/recovery/params"`)
}
