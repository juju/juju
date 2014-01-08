// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmversionworker

import (
	"fmt"
	"time"

	"launchpad.net/tomb"

	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/state/api/charmversionupdater"
)

// defaultInterval is the standard value for the interval setting.
const defaultInterval = 24 * time.Hour

// interval sets how often the resuming is called.
var interval = defaultInterval

var _ worker.Worker = (*VersionLookupWorker)(nil)

// VersionLookupWorker is responsible for a periodical retrieval of charm versions
// from the charm store.
type VersionLookupWorker struct {
	st   *charmversionupdater.State
	tomb tomb.Tomb
}

// NewVersionLookup periodically retrieves charm versions from the charm store.
func NewVersionLookup(st *charmversionupdater.State) *VersionLookupWorker {
	vl := &VersionLookupWorker{st: st}
	go func() {
		defer vl.tomb.Done()
		vl.tomb.Kill(vl.loop())
	}()
	return vl
}

func (vl *VersionLookupWorker) String() string {
	return fmt.Sprintf("charm version lookup worker")
}

func (vl *VersionLookupWorker) Stop() error {
	vl.tomb.Kill(nil)
	return vl.tomb.Wait()
}

// Kill is defined on the worker.Worker interface.
func (vl *VersionLookupWorker) Kill() {
	vl.tomb.Kill(nil)
}

// Wait is defined on the worker.Worker interface.
func (vl *VersionLookupWorker) Wait() error {
	return vl.tomb.Wait()
}

func (vl *VersionLookupWorker) loop() error {
	for {
		select {
		case <-vl.tomb.Dying():
			return tomb.ErrDying
		case <-time.After(interval):
			if err := vl.st.UpdateVersions(); err != nil {
				log.Errorf("worker/charm version lookup: cannot process charm versions: %v", err)
			}
		}
	}
}
