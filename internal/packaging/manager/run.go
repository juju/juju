// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package manager

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"

	internallogger "github.com/juju/juju/internal/logger"
)

var (
	logger = internallogger.GetLogger("juju.packaging.manager")

	// Override for testing.
	Delay    = 10 * time.Second
	Attempts = 30
)

// CommandOutput is cmd.Output. It was aliased for testing purposes.
var CommandOutput = (*exec.Cmd).CombinedOutput

// ProcessStateSys is ps.Sys. It was aliased for testing purposes.
var ProcessStateSys = (*os.ProcessState).Sys

// RunCommand is helper function to execute the command and gather the output.
var RunCommand = func(command string, args ...string) (output string, err error) {
	cmd := exec.Command(command, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// exitStatuser is a mini-interface for the ExitStatus() method.
type exitStatuser interface {
	ExitStatus() int
}

// Retryable allows the caller to define if a retry is retryable based on the
// incoming error, command exit code or the stderr message.
type Retryable interface {
	// IsRetryable defines a method for working out if a retry is actually
	// retryable.
	IsRetryable(int, string) bool
}

// ErrorTransformer masks a potential error from one to another.
type ErrorTransformer interface {

	// MaskError masks a potential error from the fatal error if not retryable.
	MaskError(int, string) error
}

// RetryPolicy defines a policy for describing how retries should be executed.
type RetryPolicy struct {
	Delay    time.Duration
	Attempts int
}

// DefaultRetryPolicy returns the default retry policy.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		Delay:    Delay,
		Attempts: Attempts,
	}
}

// RunCommandWithRetry is a helper function which tries to execute the given command.
// It tries to do so for 30 times with a 10 second sleep between commands.
// It returns the output of the command, the exit code, and an error, if one occurs,
// logging along the way.
// It was aliased for testing purposes.
var RunCommandWithRetry = func(cmd string, retryable Retryable, policy RetryPolicy) (output string, code int, _ error) {
	// split the command for use with exec
	args := strings.Fields(cmd)
	if len(args) <= 1 {
		return "", -1, errors.New(fmt.Sprintf("too few arguments: expected at least 2, got %d", len(args)))
	}

	logger.Infof("Running: %s", cmd)

	// Retry operation 30 times, sleeping every 10 seconds between attempts.
	// This avoids failure in the case of something else having the dpkg lock
	// (e.g. a charm on the machine we're deploying containers to).
	var (
		out      []byte
		fatalErr error
	)
	retryErr := retry.Call(retry.CallArgs{
		Clock:    clock.WallClock,
		Delay:    policy.Delay,
		Attempts: policy.Attempts,
		NotifyFunc: func(lastError error, attempt int) {
			logger.Infof("Retrying: %s", cmd)
		},
		Func: func() error {
			// Create the command for each attempt, because we need to
			// call cmd.CombinedOutput only once. See http://pad.lv/1394524.
			command := exec.Command(args[0], args[1:]...)

			var err error
			out, err = CommandOutput(command)
			return errors.Trace(err)
		},
		IsFatalError: func(err error) bool {
			exitError, ok := errors.Cause(err).(*exec.ExitError)
			if !ok {
				logger.Errorf("unexpected error type %T", err)
				return true
			}
			waitStatus, ok := ProcessStateSys(exitError.ProcessState).(exitStatuser)
			if !ok {
				logger.Errorf("unexpected process state type %T", exitError.ProcessState.Sys())
				return true
			}

			code = waitStatus.ExitStatus()
			fatal := !retryable.IsRetryable(code, string(out))
			if fatal {
				// In order to give better error messages to the user, we
				// sometimes want to mask the original error message.
				if trans, ok := retryable.(ErrorTransformer); ok {
					maskedErr := trans.MaskError(code, string(out))
					fatalErr = errors.Annotatef(maskedErr, "encountered fatal error")
				}
			}

			return fatal
		},
	})
	if fatalErr != nil {
		retryErr = fatalErr
	}

	if retryErr != nil {
		logger.Errorf("packaging command failed: %v; cmd: %q; output: %s",
			retryErr, cmd, string(out))
		return string(out), code, errors.Errorf("packaging command failed: %v", retryErr)
	}

	return string(out), 0, nil
}

// regexpRetryable checks a series of regexps to see if they show up in the
// output of a command and mark them as retryable if they show up.

type regexpRetryable struct {
	exitCodes    map[int]struct{}
	failureCases []*regexp.Regexp
}

// makeRegexpRetryable creates a series of regexps from strings.
func makeRegexpRetryable(exitCodes []int, cases ...string) regexpRetryable {
	c := make([]*regexp.Regexp, len(cases))
	for k, v := range cases {
		// This should be picked up in tests, so should be ok to panic.
		c[k] = regexp.MustCompile(v)
	}
	codes := make(map[int]struct{})
	for _, v := range exitCodes {
		codes[v] = struct{}{}
	}
	return regexpRetryable{
		exitCodes:    codes,
		failureCases: c,
	}
}

// IsRetryable checks to see if a regexp is retryable from the exit code and
// command output.
func (r regexpRetryable) IsRetryable(code int, output string) bool {
	if _, ok := r.exitCodes[code]; !ok {
		return false
	}
	for _, re := range r.failureCases {
		if re.MatchString(output) {
			return true
		}
	}
	return false
}
