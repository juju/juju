// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

// sshscript should only print the result on the first execution,
// to handle the case where it's called multiple times. On
// subsequent executions, it should find the next 'ssh' in $PATH
// and exec that.
var sshscript = `#!/bin/bash
if [ ! -e "$0.run" ]; then
    touch "$0.run"
    diff "$0.expected-input" -
    exitcode=$?
    if [ $exitcode -ne 0 ]; then
        echo "ERROR: did not match expected input" >&2
        exit $exitcode
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

// InstallFakeSSH creates a fake "ssh" command in a new $PATH,
// updates $PATH, and returns a function to reset $PATH to
// its original value when called.
//
// output may be:
//    - nil (no output)
//    - a string (stdout)
//    - a slice of strings, of length two (stdout, stderr)
func InstallFakeSSH(c *gc.C, input string, output interface{}, rc int) testbase.Restorer {
	fakebin := c.MkDir()
	ssh := filepath.Join(fakebin, "ssh")
	sshexpectedinput := ssh + ".expected-input"
	var stdout, stderr string
	switch output := output.(type) {
	case nil:
	case string:
		stdout = fmt.Sprintf("cat<<EOF\n%s\nEOF", output)
	case []string:
		stdout = fmt.Sprintf("cat<<EOF\n%s\nEOF", output[0])
		stderr = fmt.Sprintf("cat>&2<<EOF\n%s\nEOF", output[1])
	}
	script := fmt.Sprintf(sshscript, stdout, stderr, rc)
	err := ioutil.WriteFile(ssh, []byte(script), 0777)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(sshexpectedinput, []byte(input), 0644)
	c.Assert(err, gc.IsNil)
	return testbase.PatchEnvironment("PATH", fakebin+":"+os.Getenv("PATH"))
}

// InstallDetectionFakeSSH installs a fake SSH command, which will respond
// to the series/hardware detection script with the specified
// series/arch.
func InstallDetectionFakeSSH(c *gc.C, series, arch string) testbase.Restorer {
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
	return InstallFakeSSH(c, detectionScript, detectionoutput, 0)
}

// FakeSSH wraps the invocation of installFakeSSH based on the parameters.
type FakeSSH struct {
	Series string
	Arch   string

	// exit code for the machine agent provisioning script.
	ProvisionAgentExitCode int

	// there are conditions other than error in the above
	// that might cause provisioning to not go ahead, such
	// as tools being missing.
	SkipProvisionAgent bool

	// detection will be skipped if the series/hardware were
	// detected ahead of time.
	SkipDetection bool
}

// Install installs fake SSH commands, which will respond to
// manual provisioning/bootstrapping commands with the specified
// output and exit codes.
func (r FakeSSH) Install(c *gc.C) testbase.Restorer {
	var restore testbase.Restorer
	add := func(input string, output interface{}, rc int) {
		restore = restore.Add(InstallFakeSSH(c, input, output, rc))
	}
	if !r.SkipProvisionAgent {
		add("", nil, r.ProvisionAgentExitCode)
	}
	if !r.SkipDetection {
		restore.Add(InstallDetectionFakeSSH(c, r.Series, r.Arch))
	}
	add("", nil, 0) // checkProvisioned
	return restore
}
