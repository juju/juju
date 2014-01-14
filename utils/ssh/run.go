// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// ExecParams are used for the parameters for ExecuteCommandOnMachine.
type ExecParams struct {
	IdentityFile string
	Host         string
	Command      string
	Timeout      time.Duration
}

// ExecuteCommandOnMachine will execute the command passed through on
// the host specified. This is done using ssh, and passing the commands
// through /bin/bash.  If the command is not finished within the timeout
// specified, an error is returned.  Any output captured during that time
// is also returned.
func ExecuteCommandOnMachine(params ExecParams) (rc int, stdout, stderr []byte, err error) {
	// execute bash accepting commands on stdin
	if params.Host == "" {
		return -1, nil, nil, fmt.Errorf("missing host address")
	}
	logger.Debugf("execute on %s", params.Host)
	var options Options
	if params.IdentityFile != "" {
		options.SetIdentities(params.IdentityFile)
	}
	command := Command(params.Host, []string{"/bin/bash", "-s"}, &options)
	// start a go routine to do the actual execution
	var stdoutBuf, stderrBuf bytes.Buffer
	command.Stdout = &stdoutBuf
	command.Stderr = &stderrBuf
	command.Stdin = strings.NewReader(params.Command + "\n")

	if err = command.Start(); err != nil {
		return -1, nil, nil, err
	}
	commandDone := make(chan error)
	go func() {
		defer close(commandDone)
		err := command.Wait()
		logger.Debugf("command.Wait finished: %v", err)
		commandDone <- err
	}()

	select {
	case err = <-commandDone:
		logger.Debugf("select from commandDone channel: %v", err)
		// command finished and returned us the results
		if ee, ok := err.(*exec.ExitError); ok && err != nil {
			status := ee.ProcessState.Sys().(syscall.WaitStatus)
			if status.Exited() {
				// A non-zero return code isn't considered an error here.
				rc = status.ExitStatus()
				err = nil
			}
		}

	case <-time.After(params.Timeout):
		logger.Infof("killing the command due to timeout")
		err = fmt.Errorf("command timed out")
		command.Kill()
	}
	// In either case, gather as much as we have from stdout and stderr
	return rc, stdoutBuf.Bytes(), stderrBuf.Bytes(), err
}
