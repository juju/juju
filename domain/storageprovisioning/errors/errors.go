// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

// These errors are used for storage provisioning operations.
const (
	// FilesystemNotFound is used when a filesystem cannot be found.
	FilesystemNotFound = errors.ConstError("filesystem not found")

	// FilesystemAttachmentNotFound is used when a filesystem attachment cannot be found.
	FilesystemAttachmentNotFound = errors.ConstError("filesystem attachment not found")
)
