// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

var opensshCommonOptions = []string{"-o", "StrictHostKeyChecking no"}

type opensshCommandKind int

const (
	sshKind opensshCommandKind = iota
	scpKind
)

// sshpassWrap wraps the command/args with sshpass if it is found in $PATH
// and the SSHPASS environment variable is set. Otherwise, the original
// command/args are returned.
func sshpassWrap(cmd string, args []string) (string, []string) {
	if os.Getenv("SSHPASS") != "" {
		if path, err := exec.LookPath("sshpass"); err == nil {
			return path, append([]string{"-e", cmd}, args...)
		}
	}
	return cmd, args
}

// OpenSSHClient is an implementation of Client that
// uses the ssh and scp executables found in $PATH.
type OpenSSHClient struct{}

// NewOpenSSHClient creates a new OpenSSHClient.
// If the ssh and scp programs cannot be found
// in $PATH, then an error is returned.
func NewOpenSSHClient() (*OpenSSHClient, error) {
	var c OpenSSHClient
	if _, err := exec.LookPath("ssh"); err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("scp"); err != nil {
		return nil, err
	}
	return &c, nil
}

func opensshOptions(options *Options, commandKind opensshCommandKind) []string {
	args := append([]string{}, opensshCommonOptions...)
	if options == nil {
		options = &Options{}
	}
	if !options.passwordAuthAllowed {
		args = append(args, "-o", "PasswordAuthentication no")
	}
	if options.allocatePTY {
		args = append(args, "-t")
	}
	for _, identity := range options.identities {
		args = append(args, "-i", identity)
	}
	if options.port != 0 {
		if commandKind == scpKind {
			// scp uses -P instead of -p (-p means preserve).
			args = append(args, "-P")
		} else {
			args = append(args, "-p")
		}
		args = append(args, fmt.Sprint(options.port))
	}
	return args
}

// Command implements Client.Command.
func (c *OpenSSHClient) Command(host string, command []string, options *Options) *Cmd {
	args := opensshOptions(options, sshKind)
	args = append(args, host)
	if len(command) > 0 {
		args = append(args, "--")
		args = append(args, command...)
	}
	bin, args := sshpassWrap("ssh", args)
	return &Cmd{impl: &opensshCmd{exec.Command(bin, args...)}}
}

// Copy implements Client.Copy.
func (c *OpenSSHClient) Copy(source, dest string, userOptions *Options) error {
	var options Options
	if userOptions != nil {
		options = *userOptions
		options.allocatePTY = false // doesn't make sense for scp
	}
	args := opensshOptions(&options, scpKind)
	args = append(args, source, dest)
	bin, args := sshpassWrap("scp", args)
	cmd := exec.Command(bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		stderr := strings.TrimSpace(stderr.String())
		if len(stderr) > 0 {
			err = fmt.Errorf("%v (%v)", err, stderr)
		}
		return err
	}
	return nil
}

type opensshCmd struct {
	*exec.Cmd
}

func (c *opensshCmd) SetStdio(stdin io.Reader, stdout, stderr io.Writer) {
	c.Stdin, c.Stdout, c.Stderr = stdin, stdout, stderr
}

func (c *opensshCmd) StdinPipe() (io.WriteCloser, io.Reader, error) {
	wc, err := c.Cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	return wc, c.Stdin, nil
}

func (c *opensshCmd) StdoutPipe() (io.ReadCloser, io.Writer, error) {
	rc, err := c.Cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	return rc, c.Stdout, nil
}

func (c *opensshCmd) StderrPipe() (io.ReadCloser, io.Writer, error) {
	rc, err := c.Cmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}
	return rc, c.Stderr, nil
}

func (c *opensshCmd) Kill() error {
	if c.Process == nil {
		return fmt.Errorf("process has not been started")
	}
	return c.Process.Kill()
}
