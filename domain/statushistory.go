// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"github.com/juju/clock"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/statushistory"
)

// NewStatusHistory creates a new StatusHistory using the logger as the
// recorder.
func NewStatusHistory(logger logger.Logger, clock clock.Clock) *statushistory.StatusHistory {
	return statushistory.NewStatusHistory(statushistory.NewLogRecorder(logger), clock)
}
