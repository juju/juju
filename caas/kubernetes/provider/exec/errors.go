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

type exec137Error struct {
	err string
}

var _ error = &exec137Error{}

func (e exec137Error) Error() string {
	return e.err
}

func newexec137Error(err error) error {
	return &exec137Error{
		err: fmt.Sprintf("%v", err),
	}
}

// IsExec137Error returns true when the supplied error is
// caused by an exec137Error.
func IsExec137Error(err error) bool {
	_, ok := errors.Cause(err).(*exec137Error)
	return ok
}

func handleExec137Error(err error) error {
	if err == nil {
		return nil
	}
	if exitErr, ok := errors.Cause(err).(ExitError); ok {
		logger.Criticalf("handleExec137Error!!! %#v, %q", exitErr, exitErr.ExitStatus())
		if exitErr.ExitStatus() == 137 {
			return newexec137Error(exitErr)
		}
	}
	return err
}
