// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package minunitsworker

import (
	"fmt"

	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

// MinUnitsWorker ensures the minimum number of units for services is respected.
type MinUnitsWorker struct {
	tomb tomb.Tomb
	st   *state.State
}

// NewMinUnitsWorker returns a MinUnitsWorker that runs service.EnsureMinUnits()
// if the number of alive units belonging to a service decreases, or if the
// minimum required number of units for a service is increased.
func NewMinUnitsWorker(st *state.State) *MinUnitsWorker {
	mu := &MinUnitsWorker{st: st}
	go func() {
		defer mu.tomb.Done()
		mu.tomb.Kill(mu.loop())
	}()
	return mu
}

func (mu *MinUnitsWorker) String() string {
	return fmt.Sprintf("minunitsworker")
}

func (mu *MinUnitsWorker) Kill() {
	mu.tomb.Kill(nil)
}

func (mu *MinUnitsWorker) Stop() error {
	mu.tomb.Kill(nil)
	return mu.tomb.Wait()
}

func (mu *MinUnitsWorker) Wait() error {
	return mu.tomb.Wait()
}

func (mu *MinUnitsWorker) handle(serviceName string) error {
	service, err := mu.st.Service(serviceName)
	if err != nil {
		return err
	}
	return service.EnsureMinUnits()
}

func (mu *MinUnitsWorker) loop() error {
	w := mu.st.WatchMinUnits()
	defer watcher.Stop(w, &mu.tomb)
	for {
		select {
		case <-mu.tomb.Dying():
			return tomb.ErrDying
		case serviceNames, ok := <-w.Changes():
			if !ok {
				return watcher.MustErr(w)
			}
			for _, name := range serviceNames {
				log.Infof("worker/minunitsworker: processing service %v", name)
				if err := mu.handle(name); err != nil {
					log.Errorf("worker/minunitsworker: error: service %v: %v", name, err)
				}
			}
		}
	}
	panic("unreachable")
}
