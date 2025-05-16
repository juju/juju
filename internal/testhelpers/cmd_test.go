// Copyright 2012-2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testhelpers_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	testing "github.com/juju/juju/internal/testhelpers"
)

type cmdSuite struct {
	testing.CleanupSuite
}

func TestCmdSuite(t *stdtesting.T) { tc.Run(t, &cmdSuite{}) }
func (s *cmdSuite) TestHookCommandOutput(c *tc.C) {
	var commandOutput = (*exec.Cmd).CombinedOutput

	cmdChan, cleanup := testing.HookCommandOutput(&commandOutput, []byte{1, 2, 3, 4}, nil)
	defer cleanup()

	testCmd := exec.Command("fake-command", "arg1", "arg2")
	out, err := commandOutput(testCmd)
	c.Assert(err, tc.IsNil)
	cmd := <-cmdChan
	c.Assert(out, tc.DeepEquals, []byte{1, 2, 3, 4})
	c.Assert(cmd.Args, tc.DeepEquals, []string{"fake-command", "arg1", "arg2"})
}

func (s *cmdSuite) EnsureArgFileRemoved(name string) {
	s.AddCleanup(func(c *tc.C) {
		c.Assert(name+".out", tc.DoesNotExist)
	})
}

const testFunc = "test-output"

func (s *cmdSuite) TestPatchExecutableNoArgs(c *tc.C) {
	s.EnsureArgFileRemoved(testFunc)
	testing.PatchExecutableAsEchoArgs(c, s, testFunc)
	output := runCommand(c, testFunc)
	output = strings.TrimRight(output, "\r\n")
	c.Assert(output, tc.Equals, testFunc)
	testing.AssertEchoArgs(c, testFunc)
}

func (s *cmdSuite) TestPatchExecutableWithArgs(c *tc.C) {
	s.EnsureArgFileRemoved(testFunc)
	testing.PatchExecutableAsEchoArgs(c, s, testFunc)
	output := runCommand(c, testFunc, "foo", "bar baz")
	output = strings.TrimRight(output, "\r\n")

	c.Assert(output, tc.DeepEquals, testFunc+" 'foo' 'bar baz'")

	testing.AssertEchoArgs(c, testFunc, "foo", "bar baz")
}

func (s *cmdSuite) TestPatchExecutableThrowError(c *tc.C) {
	testing.PatchExecutableThrowError(c, s, testFunc, 1)
	cmd := exec.Command(testFunc)
	out, err := cmd.CombinedOutput()
	c.Assert(err, tc.ErrorMatches, "exit status 1")
	output := strings.TrimRight(string(out), "\r\n")
	c.Assert(output, tc.Equals, "failing")
}

func (s *cmdSuite) TestCaptureOutput(c *tc.C) {
	f := func() {
		_, err := fmt.Fprint(os.Stderr, "this is stderr")
		c.Assert(err, tc.ErrorIsNil)
		_, err = fmt.Fprint(os.Stdout, "this is stdout")
		c.Assert(err, tc.ErrorIsNil)
	}
	stdout, stderr := testing.CaptureOutput(c, f)
	c.Check(string(stdout), tc.Equals, "this is stdout")
	c.Check(string(stderr), tc.Equals, "this is stderr")
}
func TestExecHelperSuite(t *stdtesting.T) { tc.Run(t, &ExecHelperSuite{}) }
func TestMain(m *stdtesting.M) {
	testing.ExecHelperProcess()
	os.Exit(m.Run())
}

type ExecHelperSuite struct{}

func (s *ExecHelperSuite) TestExecHelperError(c *tc.C) {
	argChan := make(chan []string, 1)

	cfg := testing.PatchExecConfig{
		Stdout:   "Hellooooo stdout!",
		Stderr:   "Hellooooo stderr!",
		ExitCode: 55,
		Args:     argChan,
	}

	f := testing.ExecCommand(cfg)

	stderr := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	cmd := f("echo", "hello world!")
	cmd.Stderr = stderr
	cmd.Stdout = stdout
	err := cmd.Run()
	c.Assert(err, tc.NotNil)
	_, ok := err.(*exec.ExitError)
	if !ok {
		c.Errorf("Expected *exec.ExitError, but got %T", err)
	} else {
		c.Check(err.Error(), tc.Equals, "exit status 55")
	}
	c.Check(stderr.String(), tc.Equals, cfg.Stderr+"\n")
	c.Check(stdout.String(), tc.Equals, cfg.Stdout+"\n")

	select {
	case args := <-argChan:
		c.Assert(args, tc.DeepEquals, []string{"echo", "hello world!"})
	default:
		c.Fatalf("No arguments passed to output channel")
	}
}

func (s *ExecHelperSuite) TestExecHelper(c *tc.C) {
	argChan := make(chan []string, 1)

	cfg := testing.PatchExecConfig{
		Stdout: "Hellooooo stdout!",
		Stderr: "Hellooooo stderr!",
		Args:   argChan,
	}

	f := testing.ExecCommand(cfg)

	stderr := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	cmd := f("echo", "hello world!")
	cmd.Stderr = stderr
	cmd.Stdout = stdout
	err := cmd.Run()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(stderr.String(), tc.Equals, cfg.Stderr+"\n")
	c.Check(stdout.String(), tc.Equals, cfg.Stdout+"\n")

	select {
	case args := <-argChan:
		c.Assert(args, tc.DeepEquals, []string{"echo", "hello world!"})
	default:
		c.Fatalf("No arguments passed to output channel")
	}
}

func runCommand(c *tc.C, command string, args ...string) string {
	cmd := exec.Command(command, args...)
	out, err := cmd.CombinedOutput()
	c.Assert(err, tc.ErrorIsNil)
	return string(out)
}
