// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
)

const (
	// HasHostedModelsError defines if an attempt was made to destroy the
	// controller model while it contained non-empty hosted models, without
	// specifying that they should also be destroyed.
	HasHostedModelsError = errors.ConstError("has hosted models")

	// ModelNotEmptyError reports whether or not the given error was caused
	// due to an operation requiring a model to be empty, where the model is
	// non-empty.
	ModelNotEmptyError = errors.ConstError("model not empty")

	// PersistentStorageError indicates  whether or not the given error was
	// caused by an attempt to destroy a model while it contained persistent
	// storage, without specifying how the storage should be removed
	// (destroyed or released).
	PersistentStorageError = errors.ConstError("model contains persistent storage")
)

func NewHasHostedModelsError(i int) error {
	sep := ""
	if i != 1 {
		sep = "s"
	}
	return errors.WithType(
		fmt.Errorf("hosting %d other model%s", i, sep),
		HasHostedModelsError)
}

// NewModelNotEmptyError constructs a ModelNotEmpty with the error message
// tailored to match the number of machines, applications, volumes and
// filesystem's left. The returned error satisfies ModelNotEmptyError.
func NewModelNotEmptyError(machines, applications, volumes, filesystems int) error {
	if machines+applications+volumes+filesystems == 0 {
		return nil
	}
	plural := func(n int, thing string) string {
		s := fmt.Sprintf("%d %s", n, thing)
		if n != 1 {
			s += "s"
		}
		return s
	}
	var contains []string
	if n := machines; n > 0 {
		contains = append(contains, plural(n, "machine"))
	}
	if n := applications; n > 0 {
		contains = append(contains, plural(n, "application"))
	}
	if n := volumes; n > 0 {
		contains = append(contains, plural(n, "volume"))
	}
	if n := filesystems; n > 0 {
		contains = append(contains, plural(n, "filesystem"))
	}
	return fmt.Errorf("%w, found %s", ModelNotEmptyError, strings.Join(contains, ", "))
}
