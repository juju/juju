// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"errors"
	"fmt"
	"sync"

	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	apiwatcher "launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/state/watcher"
)

var ErrTerminateAgent = errors.New("agent should be terminated")

var loadedInvalid = func() {}

var logger = loggo.GetLogger("juju.worker")

// EnvironConfigGetter interface defines a way to read the environment
// configuration.
type EnvironConfigGetter interface {
	EnvironConfig() (*config.Config, error)
}

// TODO(rog) remove WaitForEnviron, as we now should always
// start with a valid environ config.

// WaitForEnviron waits for an valid environment to arrive from
// the given watcher. It terminates with tomb.ErrDying if
// it receives a value on dying.
func WaitForEnviron(w apiwatcher.NotifyWatcher, st EnvironConfigGetter, dying <-chan struct{}) (environs.Environ, error) {
	for {
		select {
		case <-dying:
			return nil, tomb.ErrDying
		case _, ok := <-w.Changes():
			if !ok {
				return nil, watcher.MustErr(w)
			}
			config, err := st.EnvironConfig()
			if err != nil {
				return nil, err
			}
			environ, err := environs.New(config)
			if err == nil {
				return environ, nil
			}
			logger.Errorf("loaded invalid environment configuration: %v", err)
			loadedInvalid()
		}
	}
}

// EnvironObserver watches the current environment configuration
// and makes it available. It discards invalid environment
// configurations.
type EnvironObserver struct {
	tomb           tomb.Tomb
	environWatcher state.NotifyWatcher
	st             *state.State
	mu             sync.Mutex
	environ        environs.Environ
}

// NewEnvironObserver waits for the state to have a valid environment
// configuration and returns a new environment observer. While waiting
// for the first environment configuration, it will return with
// tomb.ErrDying if it receives a value on dying.
func NewEnvironObserver(st *state.State) (*EnvironObserver, error) {
	config, err := st.EnvironConfig()
	if err != nil {
		return nil, err
	}
	environ, err := environs.New(config)
	if err != nil {
		return nil, fmt.Errorf("cannot make Environ: %v", err)
	}
	environWatcher := st.WatchForEnvironConfigChanges()
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
