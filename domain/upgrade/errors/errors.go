// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// ErrUpgradeAlreadyStarted states that the upgrade could not be started.
	// This error occurs when the upgrade is already in progress.
	ErrUpgradeAlreadyStarted = errors.ConstError("upgrade already started")
)
