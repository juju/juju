// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// DuplicateNamespaceSequence is returned when a sequence has duplicate
	// namespaces.
	DuplicateNamespaceSequence = errors.ConstError("duplicate namespace sequence")
)
