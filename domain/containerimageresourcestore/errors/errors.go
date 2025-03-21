// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// ContainerImageMetadataNotFound describes an error that occurs when
	// container image metadata is not found.
	ContainerImageMetadataNotFound = errors.ConstError("container image metadata not found")
	// ContainerImageMetadataAlreadyStored describes an error that occurs when
	// container image metadata has already been stored under the specified
	// storage key.
	ContainerImageMetadataAlreadyStored = errors.ConstError("container image metadata already stored")
)
