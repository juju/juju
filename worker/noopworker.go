// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"gopkg.in/juju/worker.v1"
)

func NewNoOpWorker() worker.Worker {
	return NewSimpleWorker(doNothing)
}

func doNothing(stop <-chan struct{}) error {
	<-stop
	return nil
}
