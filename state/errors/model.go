// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
)

type hasHostedModelsError struct{ i int }

func NewHasHostedModelsError(i int) error {
	return &hasHostedModelsError{i: i}
}

func (e hasHostedModelsError) Error() string {
	s := ""
	if e.i != 1 {
		s = "s"
	}
	return fmt.Sprintf("hosting %d other model"+s, e.i)
}

// IsHasHostedModelsError reports whether or not the given error
// was caused by an attempt to destroy the controller model while
// it contained non-empty hosted models, without specifying that
// they should also be destroyed.
func IsHasHostedModelsError(err error) bool {
	_, ok := errors.Cause(err).(*hasHostedModelsError)
	return ok
}

type hasPersistentStorageError struct{}

func NewHasPersistentStorageError() error {
	return &hasPersistentStorageError{}
}

func (hasPersistentStorageError) Error() string {
	return "model contains persistent storage"
}

// IsHasPersistentStorageError reports whether or not the given
// error was caused by an attempt to destroy a model while it
// contained persistent storage, without specifying how the
// storage should be removed (destroyed or released).
func IsHasPersistentStorageError(err error) bool {
	_, ok := errors.Cause(err).(*hasPersistentStorageError)
	return ok
}

type modelNotEmptyError struct {
	machines     int
	applications int
	volumes      int
	filesystems  int
}

func NewModelNotEmptyError(machines, applications, volumes, filesystems int) error {
	if machines+applications+volumes+filesystems == 0 {
		return nil
	}
	return &modelNotEmptyError{
		machines:     machines,
		applications: applications,
		volumes:      volumes,
		filesystems:  filesystems,
	}
}

// Error is part of the error interface.
func (e modelNotEmptyError) Error() string {
	msg := "model not empty, found "
	plural := func(n int, thing string) string {
		s := fmt.Sprintf("%d %s", n, thing)
		if n != 1 {
			s += "s"
		}
		return s
	}
	var contains []string
	if n := e.machines; n > 0 {
		contains = append(contains, plural(n, "machine"))
	}
	if n := e.applications; n > 0 {
		contains = append(contains, plural(n, "application"))
	}
	if n := e.volumes; n > 0 {
		contains = append(contains, plural(n, "volume"))
	}
	if n := e.filesystems; n > 0 {
		contains = append(contains, plural(n, "filesystem"))
	}
	return msg + strings.Join(contains, ", ")
}

// IsModelNotEmptyError reports whether or not the given error was caused
// due to an operation requiring a model to be empty, where the model is
// non-empty.
func IsModelNotEmptyError(err error) bool {
	_, ok := errors.Cause(err).(*modelNotEmptyError)
	return ok
}
