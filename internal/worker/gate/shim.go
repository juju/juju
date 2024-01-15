// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gate

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
)

func NewFlagWorker(gate Waiter) (worker.Worker, error) {
	worker, err := NewFlag(gate)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
