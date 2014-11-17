// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"bytes"
	"io"
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/utils/ssh"
)

type fakeClient struct {
	calls      []string
	hostArg    string
	commandArg []string
	optionsArg *ssh.Options
	copyArgs   []string

	err  error
	cmd  *ssh.Cmd
	impl fakeCommandImpl
}

func (cl *fakeClient) checkCalls(c *gc.C, host string, command []string, options *ssh.Options, copyArgs []string, calls ...string) {
	c.Check(cl.hostArg, gc.Equals, host)
	c.Check(cl.commandArg, jc.DeepEquals, command)
	c.Check(cl.optionsArg, gc.Equals, options)
	c.Check(cl.copyArgs, jc.DeepEquals, copyArgs)
	c.Check(cl.calls, jc.DeepEquals, calls)
}

func (cl *fakeClient) Command(host string, command []string, options *ssh.Options) *ssh.Cmd {
	cl.calls = append(cl.calls, "Command")
	cl.hostArg = host
	cl.commandArg = command
	cl.optionsArg = options
	cmd := cl.cmd
	if cmd == nil {
		cmd = ssh.TestNewCmd(&cl.impl)
	}
	return cmd
}

func (cl *fakeClient) Copy(args []string, options *ssh.Options) error {
	cl.calls = append(cl.calls, "Copy")
	cl.copyArgs = args
	cl.optionsArg = options
	return cl.err
}

type bufferWriter struct {
	bytes.Buffer
}

func (*bufferWriter) Close() error {
	return nil
}

type fakeCommandImpl struct {
	calls     []string
	stdinArg  io.Reader
	stdoutArg io.Writer
	stderrArg io.Writer
	stdinData bufferWriter

	err        error
	stdinRaw   io.Reader
	stdoutRaw  io.Writer
	stderrRaw  io.Writer
	stdoutData bytes.Buffer
	stderrData bytes.Buffer
}

func (ci *fakeCommandImpl) checkCalls(c *gc.C, stdin io.Reader, stdout, stderr io.Writer, calls ...string) {
	c.Check(ci.stdinArg, gc.Equals, stdin)
	c.Check(ci.stdoutArg, gc.Equals, stdout)
	c.Check(ci.stderrArg, gc.Equals, stderr)
	c.Check(ci.calls, jc.DeepEquals, calls)
}

func (ci *fakeCommandImpl) checkStdin(c *gc.C, data string) {
	c.Check(ci.stdinData.String(), gc.Equals, data)
}

func (ci *fakeCommandImpl) Start() error {
	ci.calls = append(ci.calls, "Start")
	return ci.err
}

func (ci *fakeCommandImpl) Wait() error {
	ci.calls = append(ci.calls, "Wait")
	return ci.err
}

func (ci *fakeCommandImpl) Kill() error {
	ci.calls = append(ci.calls, "Kill")
	return ci.err
}

func (ci *fakeCommandImpl) SetStdio(stdin io.Reader, stdout, stderr io.Writer) {
	ci.calls = append(ci.calls, "SetStdio")
	ci.stdinArg = stdin
	ci.stdoutArg = stdout
	ci.stderrArg = stderr
}

func (ci *fakeCommandImpl) StdinPipe() (io.WriteCloser, io.Reader, error) {
	ci.calls = append(ci.calls, "StdinPipe")
	return &ci.stdinData, ci.stdinRaw, ci.err
}

func (ci *fakeCommandImpl) StdoutPipe() (io.ReadCloser, io.Writer, error) {
	ci.calls = append(ci.calls, "StdoutPipe")
	return ioutil.NopCloser(&ci.stdoutData), ci.stdoutRaw, ci.err
}

func (ci *fakeCommandImpl) StderrPipe() (io.ReadCloser, io.Writer, error) {
	ci.calls = append(ci.calls, "StderrPipe")
	return ioutil.NopCloser(&ci.stderrData), ci.stderrRaw, ci.err
}
