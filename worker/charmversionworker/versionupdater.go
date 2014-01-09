// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmversionworker

import (
	"fmt"
	"time"

	"launchpad.net/tomb"

	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/api/charmversionupdater"
	"launchpad.net/juju-core/worker"
)

// defaultInterval is the standard value for the interval setting.
const defaultInterval = 6 * time.Hour

// interval sets how often the resuming is called.
var interval = defaultInterval

var _ worker.Worker = (*VersionUpdateWorker)(nil)

// VersionUpdateWorker is responsible for a periodical retrieval of charm versions
// from the charm store, and recording the revision status for deployed charms.
type VersionUpdateWorker struct {
	st   *charmversionupdater.State
	tomb tomb.Tomb
}

// NewVersionUpdateWorker periodically retrieves charm versions from the charm store.
func NewVersionUpdateWorker(st *charmversionupdater.State) *VersionUpdateWorker {
	vuw := &VersionUpdateWorker{st: st}
	go func() {
		defer vuw.tomb.Done()
		vuw.tomb.Kill(vuw.loop())
	}()
	return vuw
}

func (vuw *VersionUpdateWorker) String() string {
	return fmt.Sprintf("charm version lookup worker")
}

// Stop stops the worker.
func (vuw *VersionUpdateWorker) Stop() error {
	vuw.tomb.Kill(nil)
	return vuw.tomb.Wait()
}

// Kill is defined on the worker.Worker interface.
func (vuw *VersionUpdateWorker) Kill() {
	vuw.tomb.Kill(nil)
}

// Wait is defined on the worker.Worker interface.
func (vuw *VersionUpdateWorker) Wait() error {
	return vuw.tomb.Wait()
}

func (vuw *VersionUpdateWorker) loop() error {
	vuw.updateVersions()
	for {
		select {
		case <-vuw.tomb.Dying():
			return tomb.ErrDying
		case <-time.After(interval):
			vuw.updateVersions()
		}
	}
}

func (vuw *VersionUpdateWorker) updateVersions() {
	if err := vuw.st.UpdateVersions(); err != nil {
		log.Errorf("worker/charm version lookup: cannot process charm versions: %v", err)
	}
}
