// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"sync"

	"launchpad.net/tomb"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/worker"
)

// TODO(rog) 2013-10-02
// Put this somewhere generally available and
// refactor other workers to use it.

// environObserver watches the current environment configuration
// and makes it available. It discards invalid environment
// configurations.
type environObserver struct {
	tomb           tomb.Tomb
	environWatcher state.NotifyWatcher
	st             *state.State
	mu             sync.Mutex
	environ        environs.Environ
}

// newEnvironObserver waits for the state to have a valid environment
// configuration and returns a new environment observer. While waiting
// for the first environment configuration, it will return with
// tomb.ErrDying if it receives a value on dying.
func newEnvironObserver(st *state.State, dying <-chan struct{}) (*environObserver, error) {
	environWatcher := st.WatchForEnvironConfigChanges()
	environ, err := worker.WaitForEnviron(environWatcher, st, dying)
	if err != nil {
		return nil, err
	}
	obs := &environObserver{
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

func (obs *environObserver) loop() error {
	for {
		select {
		case <-obs.tomb.Dying():
			return nil
		case _, ok := <-obs.environWatcher.Changes():
			if !ok {
				return watcher.MustErr(obs.environWatcher)
			}
		}
		config, err := obs.st.EnvironConfig()
		if err != nil {
			logger.Warningf("error reading environment config: %v", err)
			continue
		}
		environ, err := environs.New(config)
		if err != nil {
			logger.Warningf("error creating Environ: %v", err)
			continue
		}
		obs.mu.Lock()
		obs.environ = environ
		obs.mu.Unlock()
	}
}

// Environ returns the most recent valid Environ.
func (obs *environObserver) Environ() environs.Environ {
	obs.mu.Lock()
	defer obs.mu.Unlock()
	return obs.environ
}

func (obs *environObserver) Kill() {
	obs.tomb.Kill(nil)
}

func (obs *environObserver) Wait() error {
	return obs.tomb.Wait()
}
