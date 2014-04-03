// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/manual"
)

// sshscript should only print the result on the first execution,
// to handle the case where it's called multiple times. On
// subsequent executions, it should find the next 'ssh' in $PATH
// and exec that.
var sshscript = `#!/bin/bash --norc
if [ ! -e "$0.run" ]; then
    touch "$0.run"
    if [ -e "$0.expected-input" ]; then
        diff "$0.expected-input" -
        exitcode=$?
        if [ $exitcode -ne 0 ]; then
            echo "ERROR: did not match expected input" >&2
            exit $exitcode
        fi
        else
            head >/dev/null
        fi
    # stdout
    %s
    # stderr
    %s
    exit %d
else
    export PATH=${PATH#*:}
    exec ssh $*
fi`

// installFakeSSH creates a fake "ssh" command in a new $PATH,
// updates $PATH, and returns a function to reset $PATH to its
// original value when called.
//
// input may be:
//    - nil (ignore input)
//    - a string (match input exactly)
// output may be:
//    - nil (no output)
//    - a string (stdout)
//    - a slice of strings, of length two (stdout, stderr)
func installFakeSSH(c *gc.C, input, output interface{}, rc int) testing.Restorer {
	fakebin := c.MkDir()
	ssh := filepath.Join(fakebin, "ssh")
	switch input := input.(type) {
	case nil:
	case string:
		sshexpectedinput := ssh + ".expected-input"
		err := ioutil.WriteFile(sshexpectedinput, []byte(input), 0644)
		c.Assert(err, gc.IsNil)
	default:
		c.Errorf("input has invalid type: %T", input)
	}
	var stdout, stderr string
	switch output := output.(type) {
	case nil:
	case string:
		stdout = fmt.Sprintf("cat<<EOF\n%s\nEOF", output)
	case []string:
		c.Assert(output, gc.HasLen, 2)
		stdout = fmt.Sprintf("cat<<EOF\n%s\nEOF", output[0])
		stderr = fmt.Sprintf("cat>&2<<EOF\n%s\nEOF", output[1])
	}
	script := fmt.Sprintf(sshscript, stdout, stderr, rc)
	err := ioutil.WriteFile(ssh, []byte(script), 0777)
	c.Assert(err, gc.IsNil)
	return testing.PatchEnvPathPrepend(fakebin)
}

// installDetectionFakeSSH installs a fake SSH command, which will respond
// to the series/hardware detection script with the specified
// series/arch.
func installDetectionFakeSSH(c *gc.C, series, arch string) testing.Restorer {
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
	return installFakeSSH(c, manual.DetectionScript, detectionoutput, 0)
}

// fakeSSH wraps the invocation of InstallFakeSSH based on the parameters.
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
func (r fakeSSH) install(c *gc.C) testing.Restorer {
	var restore testing.Restorer
	add := func(input, output interface{}, rc int) {
		restore = restore.Add(installFakeSSH(c, input, output, rc))
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
	add(manual.CheckProvisionedScript, checkProvisionedOutput, r.CheckProvisionedExitCode)
	if r.InitUbuntuUser {
		add("", nil, 0)
	}
	return restore
}
