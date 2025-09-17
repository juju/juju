// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/juju/internal/errors"
)

const (
	// BlockDeviceNotFound is used when a block device cannot be found when an
	// association is being made to a volume attachment.
	BlockDeviceNotFound = errors.ConstError("block device not found")
)
