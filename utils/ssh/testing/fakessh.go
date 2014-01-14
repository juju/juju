// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
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

// InstallFakeSSH creates a fake "ssh" command in a new $PATH,
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
func InstallFakeSSH(c *gc.C, input, output interface{}, rc int) testbase.Restorer {
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
	return testbase.PatchEnvironment("PATH", fakebin+":"+os.Getenv("PATH"))
}
