// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

import "github.com/juju/juju/internal/errors"

const (
	// ErrUpgradeInProgress indicates that an upgrade is already in progress.
	ErrUpgradeInProgress = errors.ConstError("upgrade in progress")
)
