// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backends

import (
	"github.com/juju/worker/v5"

	"github.com/juju/juju/internal/worker/logsender"
)

// Backend is a worker that accepts log records.
type Backend interface {
	worker.Worker
	LogRecords() logsender.LogRecordCh
}
