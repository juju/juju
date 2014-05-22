// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package shell_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils/shell"
)

type scriptSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&scriptSuite{})

func (*scriptSuite) TestDumpFileOnErrorScriptOutput(c *gc.C) {
	script := shell.DumpFileOnErrorScript("a b c")
	c.Assert(script, gc.Equals, `
dump_file() {
    code=$?
    if [ $code -ne 0 -a -e 'a b c' ]; then
        cat 'a b c' >&2
    fi
    exit $code
}
trap dump_file EXIT
`[1:])
}

func (*scriptSuite) TestDumpFileOnErrorScript(c *gc.C) {
	tempdir := c.MkDir()
	filename := filepath.Join(tempdir, "log.txt")
	err := ioutil.WriteFile(filename, []byte("abc"), 0644)
	c.Assert(err, gc.IsNil)

	dumpScript := shell.DumpFileOnErrorScript(filename)
	c.Logf("%s", dumpScript)
	run := func(command string) (stdout, stderr string) {
		var stdoutBuf, stderrBuf bytes.Buffer
		cmd := exec.Command("/bin/bash", "-s")
		cmd.Stdin = strings.NewReader(dumpScript + command)
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		cmd.Run()
		return stdoutBuf.String(), stderrBuf.String()
	}

	stdout, stderr := run("exit 0")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr = run("exit 1")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "abc")

	err = os.Remove(filename)
	c.Assert(err, gc.IsNil)
	stdout, stderr = run("exit 1")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
}
