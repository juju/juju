// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"bytes"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	utilexec "github.com/juju/utils/exec"
)

// ExecParams are used for the parameters for ExecuteCommandOnMachine.
type ExecParams struct {
	IdentityFile string
	Host         string
	Command      string
	Timeout      time.Duration
}

// StartCommandOnMachine executes the command on the given host. The
// command is run in a Bash shell over an SSH connection. All output
// is captured. A RunningCmd is returned that may be used to wait
// for the command to finish running.
func StartCommandOnMachine(params ExecParams) (*RunningCmd, error) {
	// execute bash accepting commands on stdin
	if params.Host == "" {
		return nil, errors.Errorf("missing host address")
	}
	logger.Debugf("execute on %s", params.Host)

	var options Options
	if params.IdentityFile != "" {
		options.SetIdentities(params.IdentityFile)
	}
	command := Command(params.Host, []string{"/bin/bash", "-s"}, &options)

	// Run the command.
	running := &RunningCmd{
		SSHCmd: command,
	}
	command.Stdout = &running.Stdout
	command.Stderr = &running.Stderr
	command.Stdin = strings.NewReader(params.Command + "\n")
	if err := command.Start(); err != nil {
		return nil, errors.Trace(err)
	}

	return running, nil
}

// RunningCmd represents a command that has been started.
type RunningCmd struct {
	// SSHCmd is the command the was started.
	SSHCmd *Cmd

	// Stdout and Stderr are the output streams the command is using.
	Stdout bytes.Buffer
	Stderr bytes.Buffer
}

// Wait waits for the command to complete and returns the result.
func (cmd *RunningCmd) Wait() (result utilexec.ExecResponse, _ error) {
	defer func() {
		// Gather as much as we have from stdout and stderr.
		result.Stdout = cmd.Stdout.Bytes()
		result.Stderr = cmd.Stderr.Bytes()
	}()

	err := cmd.SSHCmd.Wait()
	logger.Debugf("command.Wait finished (err: %v)", err)
	code, err := getExitCode(err)
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Code = code
	return result, nil
}

// TODO(ericsnow) Add RunningCmd.WaitAbortable(abortChan <-chan error) ...
// based on WaitWithTimeout and update WaitWithTimeout to use it. We
// could make it WaitAbortable(abortChans ...<-chan error), which would
// require using reflect.Select(). Then that could simply replace Wait().
// It may make more sense, however, to have a helper function:
//   Wait(cmd T, abortChans ...<-chan error) ...

// Cancelled is an error indicating that a command timed out.
var Cancelled = errors.New("command timed out")

// WaitWithCancel waits for the command to complete and returns the result. If
// cancel is closed before the result was returned, then it takes longer than
// the provided timeout then Cancelled is returned.
func (cmd *RunningCmd) WaitWithCancel(cancel <-chan struct{}) (utilexec.ExecResponse, error) {
	var result utilexec.ExecResponse

	done := make(chan error, 1)
	go func() {
		defer close(done)
		waitResult, err := cmd.Wait()
		result = waitResult
		done <- err
	}()

	select {
	case err := <-done:
		return result, errors.Trace(err)
	case <-cancel:
		logger.Infof("killing the command due to cancellation")
		cmd.SSHCmd.Kill()

		<-done            // Ensure that the original cmd.Wait() call completed.
		cmd.SSHCmd.Wait() // Finalize cmd.SSHCmd, if necessary.
		return result, Cancelled
	}
}

func getExitCode(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	err = errors.Cause(err)
	if ee, ok := err.(*exec.ExitError); ok {
		raw := ee.ProcessState.Sys()
		status, ok := raw.(syscall.WaitStatus)
		if !ok {
			logger.Errorf("unexpected type %T from ProcessState.Sys()", raw)
		} else if status.Exited() {
			// A non-zero return code isn't considered an error here.
			return status.ExitStatus(), nil
		}
	}
	return -1, err
}

// ExecuteCommandOnMachine will execute the command passed through on
// the host specified. This is done using ssh, and passing the commands
// through /bin/bash.  If the command is not finished within the timeout
// specified, an error is returned.  Any output captured during that time
// is also returned in the remote response.
func ExecuteCommandOnMachine(args ExecParams) (utilexec.ExecResponse, error) {
	var result utilexec.ExecResponse

	cmd, err := StartCommandOnMachine(args)
	if err != nil {
		return result, errors.Trace(err)
	}

	cancel := make(chan struct{})
	go func() {
		<-clock.WallClock.After(args.Timeout)
		close(cancel)
	}()
	result, err = cmd.WaitWithCancel(cancel)
	if err != nil {
		return result, errors.Trace(err)
	}

	return result, nil
}
