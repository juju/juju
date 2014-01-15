// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
	sshtesting "launchpad.net/juju-core/utils/ssh/testing"
)

// installDetectionFakeSSH installs a fake SSH command, which will respond
// to the series/hardware detection script with the specified
// series/arch.
func installDetectionFakeSSH(c *gc.C, series, arch string) testbase.Restorer {
	if series == "" {
		series = "precise"
	}
	if arch == "" {
		arch = "amd64"
	}
	detectionoutput := strings.Join([]string{
		series,
		arch,
		"MemTotal: 4096 kB",
		"processor: 0",
	}, "\n")
	return sshtesting.InstallFakeSSH(c, detectionScript, detectionoutput, 0)
}

// FakeSSH wraps the invocation of InstallFakeSSH based on the parameters.
type fakeSSH struct {
	Series string
	Arch   string

	// Provisioned should be set to true if the fakeSSH script
	// should respond to checkProvisioned with a non-empty result.
	Provisioned bool

	// exit code for the checkProvisioned script.
	CheckProvisionedExitCode int

	// exit code for the machine agent provisioning script.
	ProvisionAgentExitCode int

	// InitUbuntuUser should be set to true if the fakeSSH script
	// should respond to an attempt to initialise the ubuntu user.
	InitUbuntuUser bool

	// there are conditions other than error in the above
	// that might cause provisioning to not go ahead, such
	// as tools being missing.
	SkipProvisionAgent bool

	// detection will be skipped if the series/hardware were
	// detected ahead of time. This should always be set to
	// true when testing Bootstrap.
	SkipDetection bool
}

// install installs fake SSH commands, which will respond to
// manual provisioning/bootstrapping commands with the specified
// output and exit codes.
func (r fakeSSH) install(c *gc.C) testbase.Restorer {
	var restore testbase.Restorer
	add := func(input, output interface{}, rc int) {
		restore = restore.Add(sshtesting.InstallFakeSSH(c, input, output, rc))
	}
	if !r.SkipProvisionAgent {
		add(nil, nil, r.ProvisionAgentExitCode)
	}
	if !r.SkipDetection {
		restore.Add(installDetectionFakeSSH(c, r.Series, r.Arch))
	}
	var checkProvisionedOutput interface{}
	if r.Provisioned {
		checkProvisionedOutput = "/etc/init/jujud-machine-0.conf"
	}
	add(checkProvisionedScript, checkProvisionedOutput, r.CheckProvisionedExitCode)
	if r.InitUbuntuUser {
		add("", nil, 0)
	}
	return restore
}
