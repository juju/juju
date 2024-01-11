// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"github.com/juju/worker/v4"
)

// Watcher defines a worker that defines changes for a given type of T.
type Watcher[T any] interface {
	worker.Worker

	// Changes returns a channel of type T, that will be closed when the watcher
	// is stopped.
	Changes() <-chan T
}
