// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import "github.com/juju/juju/internal/errors"

const (
	// FileNotFound describes an error that occurs when a file is not found in
	// a charm dir.
	FileNotFound = errors.ConstError("file not found")
)
