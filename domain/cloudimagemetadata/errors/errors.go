// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"strings"

	"github.com/juju/juju/internal/errors"
)

const (
	// NotValid indicates that the metadata provided is invalid, meaning that it as several required
	// fields empty.
	NotValid = errors.ConstError("invalid metadata")

	// NotFound is an error constant indicating that the requested metadata could not be found.
	NotFound = errors.ConstError("metadata not found")

	// EmptyImageID is an error constant indicating that the image ID provided is empty.
	EmptyImageID = errors.ConstError("image id is empty")
)

// NotValidMissingFields returns an error indicating that certain fields are missing from the metadata for the specified image ID.
func NotValidMissingFields(imageID string, missingFields []string) error {
	return errors.Errorf("missing %s: %w for image %v", strings.Join(missingFields, ", "), NotValid, imageID)
}
