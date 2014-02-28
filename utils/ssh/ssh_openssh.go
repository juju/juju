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

	"launchpad.net/juju-core/utils"
)

var opensshCommonOptions = map[string][]string{"-o": []string{"StrictHostKeyChecking no"}}

// default identities will not be attempted if
// -i is specified and they are not explcitly
// included.
var defaultIdentities = []string{
	"~/.ssh/identity",
	"~/.ssh/id_rsa",
	"~/.ssh/id_dsa",
	"~/.ssh/id_ecdsa",
}

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

func opensshOptions(options *Options, commandKind opensshCommandKind) map[string][]string {
	args := make(map[string][]string)
	for k, v := range opensshCommonOptions {
		args[k] = v
	}
	if options == nil {
		options = &Options{}
	}
	if !options.passwordAuthAllowed {
		args["-o"] = append(args["-o"], "PasswordAuthentication no")
	}
	if options.allocatePTY {
		args["-t"] = []string{}
	}
	identities := append([]string{}, options.identities...)
	if pk := PrivateKeyFiles(); len(pk) > 0 {
		// Add client keys as implicit identities
		identities = append(identities, pk...)
	}
	// If any identities are specified, the
	// default ones must be explicitly specified.
	if len(identities) > 0 {
		for _, identity := range defaultIdentities {
			path, err := utils.NormalizePath(identity)
			if err != nil {
				logger.Warningf("failed to normalize path %q: %v", identity, err)
				continue
			}
			if _, err := os.Stat(path); err == nil {
				identities = append(identities, path)
			}
		}
	}
	for _, identity := range identities {
		args["-i"] = append(args["-i"], identity)
	}
	if options.port != 0 {
		port := fmt.Sprint(options.port)
		if commandKind == scpKind {
			// scp uses -P instead of -p (-p means preserve).
			args["-P"] = []string{port}
		} else {
			args["-p"] = []string{port}
		}
	}
	return args
}

func expandArgs(args map[string][]string, quote bool) []string {
	var list []string
	for opt, vals := range args {
		if len(vals) == 0 {
			list = append(list, opt)
			if opt == "-t" {
				// In order to force a PTY to be allocated, we need to
				// pass -t twice.
				list = append(list, opt)
			}
		}
		for _, val := range vals {
			list = append(list, opt)
			if quote {
				val = fmt.Sprintf("%q", val)
			}
			list = append(list, val)
		}
	}
	return list
}

// Command implements Client.Command.
func (c *OpenSSHClient) Command(host string, command []string, options *Options) *Cmd {
	opts := opensshOptions(options, sshKind)
	args := expandArgs(opts, false)
	args = append(args, host)
	if len(command) > 0 {
		args = append(args, command...)
	}
	bin, args := sshpassWrap("ssh", args)
	optsList := strings.Join(expandArgs(opts, true), " ")
	fullCommand := strings.Join(command, " ")
	logger.Debugf("running: %s %s %q '%s'", bin, optsList, host, fullCommand)
	return &Cmd{impl: &opensshCmd{exec.Command(bin, args...)}}
}

// Copy implements Client.Copy.
func (c *OpenSSHClient) Copy(targets, extraArgs []string, userOptions *Options) error {
	var options Options
	if userOptions != nil {
		options = *userOptions
		options.allocatePTY = false // doesn't make sense for scp
	}
	opts := opensshOptions(&options, scpKind)
	args := expandArgs(opts, false)
	args = append(args, extraArgs...)
	args = append(args, targets...)
	bin, args := sshpassWrap("scp", args)
	cmd := exec.Command(bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	allOpts := append(expandArgs(opts, true), extraArgs...)
	optsList := strings.Join(allOpts, " ")
	targetList := `"` + strings.Join(targets, `" "`) + `"`
	logger.Debugf("running: %s %s %s", bin, optsList, targetList)
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
