// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"github.com/juju/worker/v2"
)

func NewNoOpWorker() worker.Worker {
	return NewSimpleWorker(doNothing)
}

func doNothing(stop <-chan struct{}) error {
	select {
	case <-stop:
		return nil
	}
}
