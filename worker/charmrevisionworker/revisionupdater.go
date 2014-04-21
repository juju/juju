// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionworker

import (
	"fmt"
	"time"

	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/state/api/charmrevisionupdater"
	"launchpad.net/juju-core/worker"
)

var logger = loggo.GetLogger("juju.worker.charmrevisionworker")

// interval sets how often the resuming is called.
var interval = 24 * time.Hour

var _ worker.Worker = (*RevisionUpdateWorker)(nil)

// RevisionUpdateWorker is responsible for a periodical retrieval of charm versions
// from the charm store, and recording the revision status for deployed charms.
type RevisionUpdateWorker struct {
	st   *charmrevisionupdater.State
	tomb tomb.Tomb
}

// NewRevisionUpdateWorker periodically retrieves charm versions from the charm store.
func NewRevisionUpdateWorker(st *charmrevisionupdater.State) *RevisionUpdateWorker {
	ruw := &RevisionUpdateWorker{st: st}
	go func() {
		defer ruw.tomb.Done()
		ruw.tomb.Kill(ruw.loop())
	}()
	return ruw
}

func (ruw *RevisionUpdateWorker) String() string {
	return fmt.Sprintf("charm version lookup worker")
}

// Stop stops the worker.
func (ruw *RevisionUpdateWorker) Stop() error {
	ruw.tomb.Kill(nil)
	return ruw.tomb.Wait()
}

// Kill is defined on the worker.Worker interface.
func (ruw *RevisionUpdateWorker) Kill() {
	ruw.tomb.Kill(nil)
}

// Wait is defined on the worker.Worker interface.
func (ruw *RevisionUpdateWorker) Wait() error {
	return ruw.tomb.Wait()
}

func (ruw *RevisionUpdateWorker) loop() error {
	ruw.updateVersions()
	for {
		select {
		case <-ruw.tomb.Dying():
			return tomb.ErrDying
		case <-time.After(interval):
			ruw.updateVersions()
		}
	}
}

func (ruw *RevisionUpdateWorker) updateVersions() {
	if err := ruw.st.UpdateLatestRevisions(); err != nil {
		logger.Errorf("cannot process charms: %v", err)
	}
}
