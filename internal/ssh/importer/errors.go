// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package importer

import (
	"github.com/juju/errors"
)

const (
	// NoResolver represents an error that occurs when no resolver exists to
	// fulfil the import request.
	NoResolver = errors.ConstError("no resolver")

	// SubjectNotFound represents an error where the specified subject for
	// public key import was not found.
	SubjectNotFound = errors.ConstError("subject not found")
)
