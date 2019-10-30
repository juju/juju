// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec

import (
	"fmt"
	"regexp"

	"k8s.io/client-go/util/exec"
)

// ExitError exposes what we need from k8s exec.ExitError
type ExitError interface {
	error
	String() string
	ExitStatus() int
}

var _ ExitError = exec.CodeExitError{}

// PodNotFoundError error is returned when the pod does not exist.
type PodNotFoundError struct {
	err string
}

var _ error = &PodNotFoundError{}

func (e PodNotFoundError) Error() string {
	return e.err
}

func podNotFoundError(pod string) error {
	return &PodNotFoundError{
		err: fmt.Sprintf("pod %q not found", pod),
	}
}

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

var kubeletContainerNotFoundRegexp = regexp.MustCompile(`^.*container not found \("([a-zA-Z0-9\-]+)"\)$`)

func handleContainerNotFoundError(err error) error {
	match := kubeletContainerNotFoundRegexp.FindStringSubmatch(err.Error())
	if match == nil {
		return err
	}
	return containerNotRunningError(match[1])
}
