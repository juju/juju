// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"context"

	"github.com/juju/worker/v4"
)

// NoopWorker returns a worker that waits for the context to be done.
func NoopWorker() worker.Worker {
	return NewSimpleWorker(func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})
}
