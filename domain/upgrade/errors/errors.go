// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// AlreadyStarted states that the upgrade could not be started.
	// This error occurs when the upgrade is already in progress.
	AlreadyStarted = errors.ConstError("upgrade already started")
	// AlreadyExists states that an upgrade operation has already been created.
	// This error can occur when an upgrade is created.
	AlreadyExists = errors.ConstError("upgrade already exists")
	// NotFound states that an upgrade operation cannot be found where one is
	// expected.
	NotFound = errors.ConstError("upgrade not found")
)
