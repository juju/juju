// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
)

type ScriptRunnerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ScriptRunnerSuite{})

func (s *ScriptRunnerSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("skipping ScriptRunnerSuite tests on windows")
	}
	s.IsolationSuite.SetUpSuite(c)
}

func (*ScriptRunnerSuite) TestScriptRunnerFails(c *gc.C) {
	clock := testing.NewClock(coretesting.ZeroTime())
	result, err := network.RunCommand("exit 1", os.Environ(), clock, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.TimedOut, gc.Equals, false)
	c.Assert(result.Code, gc.Equals, 1)
}

func (*ScriptRunnerSuite) TestScriptRunnerSucceeds(c *gc.C) {
	clock := testing.NewClock(coretesting.ZeroTime())
	result, err := network.RunCommand("exit 0", os.Environ(), clock, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.TimedOut, gc.Equals, false)
	c.Assert(result.Code, gc.Equals, 0)
}

func (*ScriptRunnerSuite) TestScriptRunnerCheckStdout(c *gc.C) {
	clock := testing.NewClock(coretesting.ZeroTime())
	result, err := network.RunCommand("echo -n 42", os.Environ(), clock, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.TimedOut, gc.Equals, false)
	c.Assert(result.Code, gc.Equals, 0)
	c.Check(string(result.Stdout), gc.Equals, "42")
	c.Check(string(result.Stderr), gc.Equals, "")
}

func (*ScriptRunnerSuite) TestScriptRunnerCheckStderr(c *gc.C) {
	clock := testing.NewClock(coretesting.ZeroTime())
	result, err := network.RunCommand(">&2 echo -n 3.141", os.Environ(), clock, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.TimedOut, gc.Equals, false)
	c.Assert(result.Code, gc.Equals, 0)
	c.Check(string(result.Stdout), gc.Equals, "")
	c.Check(string(result.Stderr), gc.Equals, "3.141")
}

func (*ScriptRunnerSuite) TestScriptRunnerTimeout(c *gc.C) {
	result, err := network.RunCommand("sleep 60", os.Environ(), clock.WallClock, 500*time.Microsecond)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.TimedOut, gc.Equals, true)
	c.Assert(result.Code, gc.Equals, 0)
}

func (*ScriptRunnerSuite) TestBridgeScriptInvocationWithBadArg(c *gc.C) {
	args := []string{"--big-bad-bogus-arg"}

	cmd := fmt.Sprintf(`
if [ -x "$(command -v python2)" ]; then
    PREFERRED_PYTHON_BINARY=/usr/bin/python2
elif [ -x "$(command -v python3)" ]; then
    PREFERRED_PYTHON_BINARY=/usr/bin/python3
elif [ -x "$(command -v python)" ]; then
    PREFERRED_PYTHON_BINARY=/usr/bin/python
fi

if ! [ -x "$(command -v $PREFERRED_PYTHON_BINARY)" ]; then
    echo "error: $PREFERRED_PYTHON_BINARY not executable, or not a command" >&2
    exit 1
fi

${PREFERRED_PYTHON_BINARY} - %s <<'EOF'
%s
EOF
`,
		strings.Join(args, " "),
		network.BridgeScriptPythonContent)

	result, err := network.RunCommand(cmd, os.Environ(), clock.WallClock, 0)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.TimedOut, gc.Equals, false)
	c.Assert(result.Code, gc.Equals, 2) // Python argparse error
}

func (*ScriptRunnerSuite) TestBridgeScriptInvocationWithDryRun(c *gc.C) {
	args := []string{
		"--interfaces-to-bridge=non-existent",
		"--dry-run",
		"/dev/null",
	}

	cmd := fmt.Sprintf(`
if [ -x "$(command -v python2)" ]; then
    PREFERRED_PYTHON_BINARY=/usr/bin/python2
elif [ -x "$(command -v python3)" ]; then
    PREFERRED_PYTHON_BINARY=/usr/bin/python3
elif [ -x "$(command -v python)" ]; then
    PREFERRED_PYTHON_BINARY=/usr/bin/python
fi

if ! [ -x "$(command -v $PREFERRED_PYTHON_BINARY)" ]; then
    echo "error: $PREFERRED_PYTHON_BINARY not executable, or not a command" >&2
    exit 1
fi

${PREFERRED_PYTHON_BINARY} - %s <<'EOF'
%s
EOF
`,
		strings.Join(args, " "),
		network.BridgeScriptPythonContent)

	result, err := network.RunCommand(cmd, os.Environ(), clock.WallClock, 0)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.TimedOut, gc.Equals, false)
	c.Assert(result.Code, gc.Equals, 0)
}
