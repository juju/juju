// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// LokiConfigNotFound is returned when no Loki config has been configured.
	LokiConfigNotFound = errors.ConstError("loki config not found")
)
