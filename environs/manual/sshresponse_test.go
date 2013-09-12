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

	"launchpad.net/juju-core/testing"
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
%s
    exit %d
else
    export PATH=${PATH#*:}
    exec ssh $*
fi`

// sshresponse creates a fake "ssh" command in a new $PATH,
// updates $PATH, and returns a function to reset $PATH to
// its original value when called.
func sshresponse(c *gc.C, input, output string, rc int) func() {
	fakebin := c.MkDir()
	ssh := filepath.Join(fakebin, "ssh")
	sshexpectedinput := ssh + ".expected-input"
	if output != "" {
		output = fmt.Sprintf("cat<<EOF\n%s\nEOF", output)
	}
	script := fmt.Sprintf(sshscript, output, rc)
	err := ioutil.WriteFile(ssh, []byte(script), 0777)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(sshexpectedinput, []byte(input), 0644)
	c.Assert(err, gc.IsNil)
	return testing.PatchEnvironment("PATH", fakebin+":"+os.Getenv("PATH"))
}

// sshsesponder wraps the invocation of sshresponse based on the parameters.
type sshresponder struct {
	series string
	arch   string

	// exit code for the machine agent provisioning script.
	provisionAgentExitCode int

	// there are conditions other than error in the above
	// that might cause provisioning to not go ahead, such
	// as tools being missing.
	skipProvisionAgent bool
}

func (r sshresponder) respond(c *gc.C) func() {
	series := r.series
	if series == "" {
		series = "precise"
	}
	arch := r.arch
	if arch == "" {
		arch = "amd64"
	}
	detectionoutput := strings.Join([]string{
		series,
		arch,
		"MemTotal: 4096 kB",
		"processor: 0",
	}, "\n")
	// Responses are elicited conditionally, hence this mess of dynamicity.
	add := func(oldf func(), input, output string, rc int) func() {
		f := sshresponse(c, input, output, rc)
		if oldf != nil {
			newf := f
			f = func() {
				newf()
				oldf()
			}
		}
		return f
	}
	var f func()
	if !r.skipProvisionAgent {
		f = add(f, "", "", r.provisionAgentExitCode)
	}
	f = add(f, detectionScript, detectionoutput, 0)
	return add(f, checkProvisionedScript, "", 0)
}
