// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package minuniter

import (
	"fmt"

	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

// MinUniter ensures the minimum number of units for services is respected.
type MinUniter struct {
	tomb tomb.Tomb
	st   *state.State
}

// NewMinUniter returns a MinUniter that runs service.EnsureMinUnits() if the
// number of alive units belonging to a service decreases, or if the minimum
// required number of units for a service is increased.
func NewMinUniter(st *state.State) *MinUniter {
	mu := &MinUniter{st: st}
	go func() {
		defer mu.tomb.Done()
		mu.tomb.Kill(mu.loop())
	}()
	return mu
}

func (mu *MinUniter) String() string {
	return fmt.Sprintf("minuniter")
}

func (mu *MinUniter) Kill() {
	mu.tomb.Kill(nil)
}

func (mu *MinUniter) Stop() error {
	mu.tomb.Kill(nil)
	return mu.tomb.Wait()
}

func (mu *MinUniter) Wait() error {
	return mu.tomb.Wait()
}

func (mu *MinUniter) handle(serviceName string) error {
	service, err := mu.st.Service(serviceName)
	if err != nil {
		return err
	}
	return service.EnsureMinUnits()
}

func (mu *MinUniter) loop() error {
	w := mu.st.WatchMinimumUnits()
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
				log.Infof("worker/minuniter: processing service %v", name)
				if err := mu.handle(name); err != nil {
					log.Errorf("worker/minuniter: error: service %v: %v", name, err)
				}
			}
		}
	}
	panic("unreachable")
}
