// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec

import (
	"fmt"
	"regexp"

	"github.com/juju/errors"
	"k8s.io/client-go/util/exec"
)

// ExitError exposes what we need from k8s exec.ExitError
type ExitError interface {
	error
	String() string
	ExitStatus() int
}

var _ ExitError = exec.CodeExitError{}

// ContainerNotRunningError error is returned when the container is valid
// but not currently running, so the operation is retryable.
type ContainerNotRunningError struct {
	err string
}

var _ error = &ContainerNotRunningError{}

func (e ContainerNotRunningError) Error() string {
	return e.err
}

func containerNotRunningError(container string) error {
	return &ContainerNotRunningError{
		err: fmt.Sprintf("container %q not running", container),
	}
}

// IsContainerNotRunningError returns true when the supplied error is
// caused by a ContainerNotRunningError.
func IsContainerNotRunningError(err error) bool {
	_, ok := errors.Cause(err).(*ContainerNotRunningError)
	return ok
}

var kubeletContainerNotFoundRegexp = regexp.MustCompile(`^.*container not found \("([a-zA-Z0-9\-]+)"\)$`)

func handleContainerNotFoundError(err error) error {
	match := kubeletContainerNotFoundRegexp.FindStringSubmatch(err.Error())
	if match == nil {
		return err
	}
	return containerNotRunningError(match[1])
}

type execRetryableError struct {
	err string
}

var _ error = &execRetryableError{}

func (e execRetryableError) Error() string {
	return e.err
}

// NewExecRetryableError constructs an execRetryableError.
func NewExecRetryableError(err error) error {
	return &execRetryableError{
		err: fmt.Sprintf("%q", err.Error()),
	}
}

// IsExecRetryableError returns true when the supplied error is
// caused by an execRetryableError.
func IsExecRetryableError(err error) bool {
	_, ok := errors.Cause(err).(*execRetryableError)
	return ok
}

func handleExecRetryableError(err error) error {
	if err == nil {
		return nil
	}
	if exitErr, ok := errors.Cause(err).(ExitError); ok {
		if exitErr.ExitStatus() == 137 {
			return NewExecRetryableError(exitErr)
		}
	}
	return err
}
