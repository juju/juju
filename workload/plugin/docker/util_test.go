// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(&utilSuite{})

type utilSuite struct{}

func (utilSuite) TestRunDocker(c *gc.C) {
	calls := []execCommandCall{{}}
	execCommand = fakeExecCommand(calls)
	defer func() { execCommand = exec.Command }()

	out, err := runDocker("inspect", "sad_perlman")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(out), gc.Equals, `ran []string{"docker", "inspect", "sad_perlman"}`)
	c.Check(calls[0].nameIn, gc.Equals, "docker")
	c.Check(calls[0].argsIn, jc.DeepEquals, []string{"inspect", "sad_perlman"})
}

type execCommandCall struct {
	fail bool

	nameIn string
	argsIn []string
}

// fakeExecCommand returns a func that replaces the normal exec.Command
// call to produce executables. It returns a command that calls this
// test executable, telling it to run our TestExecHelper test.  The
// original command and arguments are passed as arguments to the
// testhelper after a "--" argument.
func fakeExecCommand(calls []execCommandCall) func(string, ...string) *exec.Cmd {
	index := 0
	return func(name string, args ...string) *exec.Cmd {
		calls[index].nameIn = name
		calls[index].argsIn = args
		call := calls[index]
		index += 1

		args = append([]string{"-test.run=TestExecHelper", "--", name}, args...)
		cmd := exec.Command(os.Args[0], args...)
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		if call.fail {
			cmd.Env = append(cmd.Env, "GO_HELPER_PROCESS_ERROR=1")
		}
		return cmd
	}
}

// TestExecHelper is a fake test that is just used to do predictable things when
// we run commands.
func TestExecHelper(*testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := getTestArgs()
	shouldErr := os.Getenv("GO_HELPER_PROCESS_ERROR") == "1"
	if shouldErr {
		defer os.Exit(1)
	} else {
		defer os.Exit(0)
	}

	if shouldErr {
		fmt.Fprintln(os.Stderr, "command failed!")
		return
	}
	fmt.Fprintf(os.Stdout, "ran %#v", args)
}

// getTestArgs returns the args being passed to docker, so arg[0] would be the
// docker command, like "run" or "stop".  This function will exit out of the
// helper exec if it was not passed at least 3 arguments (e.g. docker stop id),
// or if the first arg is not "docker".
func getTestArgs() []string {
	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	return args
}
