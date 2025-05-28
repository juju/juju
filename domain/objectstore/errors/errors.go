// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// ErrNotFound is returned when a path is not found.
	ErrNotFound = errors.ConstError("path not found")

	// ErrHashAndSizeAlreadyExists is returned when a hash already exists, but
	// the associated size is different. This should never happen, it means that
	// there is a collision in the hash function.
	ErrHashAndSizeAlreadyExists = errors.ConstError("hash exists for different file size")

	// ErrPathAlreadyExistsDifferentHash is returned when a path already exists
	// with a different hash.
	ErrPathAlreadyExistsDifferentHash = errors.ConstError("path already exists with different hash")

	// ErrMissingHash is returned when a hash is missing.
	ErrMissingHash = errors.ConstError("missing hash")

	// ErrHashPrefixTooShort is returned when the hash prefix is too short. To
	// help ensure uniqueness, we enforce a minimum length of 7 characters for
	// hash prefixes.
	ErrHashPrefixTooShort = errors.ConstError("minimum hash prefix length is 7")

	// ErrInvalidHashPrefix is returned when the hash prefix is invalid for a
	// reason other than being too short.
	ErrInvalidHashPrefix = errors.ConstError("invalid hash prefix")

	// ErrInvalidHash is returned when the hash is invalid.
	ErrInvalidHash = errors.ConstError("invalid hash")

	// ErrInvalidHashLength is returned when the hash length is invalid.
	ErrInvalidHashLength = errors.ConstError("invalid hash length")

	// ErrDrainingPhaseNotFound is returned when the draining phase is not
	// found.
	ErrDrainingPhaseNotFound = errors.ConstError("draining phase not found")

	// ErrDrainingAlreadyInProgress is returned when the draining phase is
	// already in progress.
	ErrDrainingAlreadyInProgress = errors.ConstError("draining already in progress")
)
