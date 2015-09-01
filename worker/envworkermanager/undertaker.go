// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envworkermanager

import (
	"time"

	"launchpad.net/loggo"

	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

var (
	undertakerLogger = loggo.GetLogger("juju.worker.envworkermanager.undertaker")
)

const (
	undertakerPeriod = 5 * time.Minute
)

// NewUndertaker is a worker which processes a dying environment.
func NewUndertaker(st *state.State, notify func()) worker.Worker {
	f := func(stopCh <-chan struct{}) error {
		defer notify()
		err := st.ProcessDyingEnviron()
		if err != nil {
			undertakerLogger.Warningf("failed to process dying environment: %v - will retry later", err)
			return nil
		}
		return nil
	}
	return worker.NewPeriodicWorker(f, undertakerPeriod)
}
