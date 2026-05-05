// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// MachineNotFound indicates that the machine does not exist.
	MachineNotFound = errors.ConstError("machine not found")

	// ImageMetadataNotFound indicates that no image metadata could be
	// found for the machine's base and constraints.
	ImageMetadataNotFound = errors.ConstError("image metadata not found")
)
