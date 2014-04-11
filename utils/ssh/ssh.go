// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package ssh contains utilities for dealing with SSH connections,
// key management, and so on. All SSH-based command executions in
// Juju should use the Command/ScpCommand functions in this package.
//
package ssh

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"syscall"

	"launchpad.net/juju-core/cmd"
)

// Options is a client-implementation independent SSH options set.
type Options struct {
	// proxyCommand specifies the command to
	// execute to proxy SSH traffic through.
	proxyCommand []string
	// ssh server port; zero means use the default (22)
	port int
	// no PTY forced by default
	allocatePTY bool
	// password authentication is disallowed by default
	passwordAuthAllowed bool
	// identities is a sequence of paths to private key/identity files
	// to use when attempting to login. A client implementaton may attempt
	// with additional identities, but must give preference to these
	identities []string
}

// SetProxyCommand sets a command to execute to proxy traffic through.
func (o *Options) SetProxyCommand(command ...string) {
	o.proxyCommand = append([]string{}, command...)
}

// SetPort sets the SSH server port to connect to.
func (o *Options) SetPort(port int) {
	o.port = port
}

// EnablePTY forces the allocation of a pseudo-TTY.
//
// Forcing a pseudo-TTY is required, for example, for sudo
// prompts on the target host.
func (o *Options) EnablePTY() {
	o.allocatePTY = true
}

// AllowPasswordAuthentication allows the SSH
// client to prompt the user for a password.
//
// Password authentication is disallowed by default.
func (o *Options) AllowPasswordAuthentication() {
	o.passwordAuthAllowed = true
}

// SetIdentities sets a sequence of paths to private key/identity files
// to use when attempting login. Client implementations may attempt to
// use additional identities, but must give preference to the ones
// specified here.
func (o *Options) SetIdentities(identityFiles ...string) {
	o.identities = append([]string{}, identityFiles...)
}

// Client is an interface for SSH clients to implement
type Client interface {
	// Command returns a Command for executing a command
	// on the specified host. Each Command is executed
	// within its own SSH session.
	//
	// Host is specified in the format [user@]host.
	Command(host string, command []string, options *Options) *Cmd

	// Copy copies file(s) between local and remote host(s).
	// Paths are specified in the scp format, [[user@]host:]path. If
	// any extra arguments are specified in extraArgs, they are passed
	// verbatim.
	Copy(args []string, options *Options) error
}

// Cmd represents a command to be (or being) executed
// on a remote host.
type Cmd struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	impl   command
}

// CombinedOutput runs the command, and returns the
// combined stdout/stderr output and result of
// executing the command.
func (c *Cmd) CombinedOutput() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("ssh: Stdout already set")
	}
	if c.Stderr != nil {
		return nil, errors.New("ssh: Stderr already set")
	}
	var b bytes.Buffer
	c.Stdout = &b
	c.Stderr = &b
	err := c.Run()
	return b.Bytes(), err
}

// Output runs the command, and returns the stdout
// output and result of executing the command.
func (c *Cmd) Output() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("ssh: Stdout already set")
	}
	var b bytes.Buffer
	c.Stdout = &b
	err := c.Run()
	return b.Bytes(), err
}

// Run runs the command, and returns the result as an error.
func (c *Cmd) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	err := c.Wait()
	if exitError, ok := err.(*exec.ExitError); ok && exitError != nil {
		status := exitError.ProcessState.Sys().(syscall.WaitStatus)
		if status.Exited() {
			return cmd.NewRcPassthroughError(status.ExitStatus())
		}
	}
	return err
}

// Start starts the command running, but does not wait for
// it to complete. If the command could not be started, an
// error is returned.
func (c *Cmd) Start() error {
	c.impl.SetStdio(c.Stdin, c.Stdout, c.Stderr)
	return c.impl.Start()
}

// Wait waits for the started command to complete,
// and returns the result as an error.
func (c *Cmd) Wait() error {
	return c.impl.Wait()
}

// Kill kills the started command.
func (c *Cmd) Kill() error {
	return c.impl.Kill()
}

// StdinPipe creates a pipe and connects it to
// the command's stdin. The read end of the pipe
// is assigned to c.Stdin.
func (c *Cmd) StdinPipe() (io.WriteCloser, error) {
	wc, r, err := c.impl.StdinPipe()
	if err != nil {
		return nil, err
	}
	c.Stdin = r
	return wc, nil
}

// StdoutPipe creates a pipe and connects it to
// the command's stdout. The write end of the pipe
// is assigned to c.Stdout.
func (c *Cmd) StdoutPipe() (io.ReadCloser, error) {
	rc, w, err := c.impl.StdoutPipe()
	if err != nil {
		return nil, err
	}
	c.Stdout = w
	return rc, nil
}

// StderrPipe creates a pipe and connects it to
// the command's stderr. The write end of the pipe
// is assigned to c.Stderr.
func (c *Cmd) StderrPipe() (io.ReadCloser, error) {
	rc, w, err := c.impl.StderrPipe()
	if err != nil {
		return nil, err
	}
	c.Stderr = w
	return rc, nil
}

// command is an implementation-specific representation of a
// command prepared to execute against a specific host.
type command interface {
	Start() error
	Wait() error
	Kill() error
	SetStdio(stdin io.Reader, stdout, stderr io.Writer)
	StdinPipe() (io.WriteCloser, io.Reader, error)
	StdoutPipe() (io.ReadCloser, io.Writer, error)
	StderrPipe() (io.ReadCloser, io.Writer, error)
}

// DefaultClient is the default SSH client for the process.
//
// If the OpenSSH client is found in $PATH, then it will be
// used for DefaultClient; otherwise, DefaultClient will use
// an embedded client based on go.crypto/ssh.
var DefaultClient Client

// chosenClient holds the type of SSH client created for
// DefaultClient, so that we can log it in Command or Copy.
var chosenClient string

func init() {
	initDefaultClient()
}

func initDefaultClient() {
	if client, err := NewOpenSSHClient(); err == nil {
		DefaultClient = client
		chosenClient = "OpenSSH"
	} else if client, err := NewGoCryptoClient(); err == nil {
		DefaultClient = client
		chosenClient = "go.crypto (embedded)"
	}
}

// Command is a short-cut for DefaultClient.Command.
func Command(host string, command []string, options *Options) *Cmd {
	logger.Debugf("using %s ssh client", chosenClient)
	return DefaultClient.Command(host, command, options)
}

// Copy is a short-cut for DefaultClient.Copy.
func Copy(args []string, options *Options) error {
	logger.Debugf("using %s ssh client", chosenClient)
	return DefaultClient.Copy(args, options)
}
