// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package machine provides the services for managing machines in Juju.
package machine

import (
	"time"

	"github.com/juju/juju/core/status"
)

// StatusInfo contains the status information for a machine.
type StatusInfo struct {
	Status  status.Status
	Message string
	Data    []byte
	Since   *time.Time
}
