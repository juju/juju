// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"sync"

	"launchpad.net/tomb"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
)

// TODO(rog) 2013-10-02
// Refactor other workers to use this common functionality.

// EnvironObserver watches the current environment configuration
// and makes it available.
type EnvironObserver struct {
	tomb           tomb.Tomb
	environWatcher state.EnvironConfigWatcher
	st             *state.State
	mu             sync.Mutex
	environ        environs.Environ
}

// newEnvironObserver waits for the state to have a valid environment
// configuration and returns a new environment observer. While waiting
// for the first environment configuration, it will return with
// tomb.ErrDying if it receives a value on dying.
func NewEnvironObserver(st *state.State, dying <-chan struct{}) (*EnvironObserver, error) {
	environWatcher := st.WatchEnvironConfig()
	environ, err := WaitForEnviron(environWatcher, st, dying)
	if err != nil {
		return nil, err
	}
	obs := &EnvironObserver{
		st:             st,
		environ:        environ,
		environWatcher: environWatcher,
	}
	go func() {
		defer obs.tomb.Done()
		defer watcher.Stop(environWatcher, &obs.tomb)
		obs.tomb.Kill(obs.loop())
	}()
	return obs, nil
}

func (obs *EnvironObserver) loop() error {
	for {
		select {
		case <-obs.tomb.Dying():
			return nil
		case configAttr, ok := <-obs.environWatcher.Changes():
			if !ok {
				return watcher.MustErr(obs.environWatcher)
			}
			environ, err := newEnviron(configAttr)
			if err != nil {
				continue
			}
			obs.mu.Lock()
			obs.environ = environ
			obs.mu.Unlock()
		}
	}
}

// Environ returns the most recent valid Environ.
func (obs *EnvironObserver) Environ() environs.Environ {
	obs.mu.Lock()
	defer obs.mu.Unlock()
	return obs.environ
}

func (obs *EnvironObserver) Kill() {
	obs.tomb.Kill(nil)
}

func (obs *EnvironObserver) Wait() error {
	return obs.tomb.Wait()
}
