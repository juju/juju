// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
)

func NewWorker(ctx context.Context, config Config) (worker.Worker, error) {
	worker, err := New(ctx, config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
