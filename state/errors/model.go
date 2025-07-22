// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
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
