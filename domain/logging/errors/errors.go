// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// LokiEndpointNotFound is returned when no Loki endpoint has been
	// configured.
	LokiEndpointNotFound = errors.ConstError("loki endpoint not found")
)
