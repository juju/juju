// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package manager

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/retry.v1"
)

var (
	logger = loggo.GetLogger("juju.packaging.manager")

	AttemptStrategy = retry.Regular{
		Delay: 10 * time.Second,
		Min:   30,
	}
)

// CommandOutput is cmd.Output. It was aliased for testing purposes.
var CommandOutput = (*exec.Cmd).CombinedOutput

// processStateSys is ps.Sys. It was aliased for testing purposes.
var ProcessStateSys = (*os.ProcessState).Sys

// RunCommand is helper function to execute the command and gather the output.
var RunCommand = func(command string, args ...string) (output string, err error) {
	cmd := exec.Command(command, args...)
	out, err := cmd.CombinedOutput()
	output = string(out)
	if err != nil {
		return output, err
	}
	return output, nil
}

// exitStatuser is a mini-interface for the ExitStatus() method.
type exitStatuser interface {
	ExitStatus() int
}

// RunCommandWithRetry is a helper function which tries to execute the given command.
// It tries to do so for 30 times with a 10 second sleep between commands.
// It returns the output of the command, the exit code, and an error, if one occurs,
// logging along the way.
// It was aliased for testing purposes.
var RunCommandWithRetry = func(cmd string, getFatalError func(string) error) (output string, code int, err error) {
	var out []byte

	// split the command for use with exec
	args := strings.Fields(cmd)
	if len(args) <= 1 {
		return "", 1, errors.New(fmt.Sprintf("too few arguments: expected at least 2, got %d", len(args)))
	}

	logger.Infof("Running: %s", cmd)

	// Retry operation 30 times, sleeping every 10 seconds between attempts.
	// This avoids failure in the case of something else having the dpkg lock
	// (e.g. a charm on the machine we're deploying containers to).
	for a := AttemptStrategy.Start(nil); a.Next(); {
		// Create the command for each attempt, because we need to
		// call cmd.CombinedOutput only once. See http://pad.lv/1394524.
		command := exec.Command(args[0], args[1:]...)

		out, err = CommandOutput(command)

		if err == nil {
			return string(out), 0, nil
		}

		exitError, ok := err.(*exec.ExitError)
		if !ok {
			err = errors.Annotatef(err, "unexpected error type %T", err)
			break
		}
		waitStatus, ok := ProcessStateSys(exitError.ProcessState).(exitStatuser)
		if !ok {
			err = errors.Annotatef(err, "unexpected process state type %T", exitError.ProcessState.Sys())
			break
		}

		// Both apt-get and yum return 100 on abnormal execution due to outside
		// issues (ex: momentary dns failure).
		code = waitStatus.ExitStatus()
		if code != 100 {
			break
		}

		if getFatalError != nil {
			if fatalErr := getFatalError(string(out)); fatalErr != nil {
				err = errors.Annotatef(fatalErr, "encountered fatal error")
				break
			}
		}

		logger.Infof("Retrying: %s", cmd)
	}

	if err != nil {
		logger.Errorf("packaging command failed: %v; cmd: %q; output: %s",
			err, cmd, string(out))
		return "", code, errors.Errorf("packaging command failed: %v", err)
	}

	return string(out), 0, nil
}
