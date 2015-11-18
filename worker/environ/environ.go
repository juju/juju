// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environ

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.environ")

var loadedInvalid = func() {}

// EnvironConfigGetter interface defines a way to read the environment
// configuration.
type EnvironConfigGetter interface {
	EnvironConfig() (*config.Config, error)
}

// ErrWaitAborted is returned from WaitForEnviron when the wait is terminated by
// closing the abort chan.
var ErrWaitAborted = errors.New("environ wait aborted")

// TODO(rog) remove WaitForEnviron, as we now should always
// start with a valid environ config.

// WaitForEnviron waits for an valid environment to arrive from
// the given watcher. It terminates with ErrWaitAborted if
// it receives a value on dying.
func WaitForEnviron(w watcher.NotifyWatcher, st EnvironConfigGetter, dying <-chan struct{}) (environs.Environ, error) {
	for {
		select {
		case <-dying:
			return nil, ErrWaitAborted
		case _, ok := <-w.Changes():
			if !ok {
				return nil, errors.New("watcher closed channel")
			}
			config, err := st.EnvironConfig()
			if err != nil {
				return nil, errors.Trace(err)
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

// EnvironConfigObserver interface defines a way to read the
// environment configuration and watch for changes.
type EnvironConfigObserver interface {
	EnvironConfigGetter
	WatchForEnvironConfigChanges() (watcher.NotifyWatcher, error)
}

// EnvironObserver watches the current environment configuration
// and makes it available. It discards invalid environment
// configurations.
type EnvironObserver struct {
	catacomb catacomb.Catacomb
	st       EnvironConfigObserver
	mu       sync.Mutex
	environ  environs.Environ
}

// NewEnvironObserver waits for the environment to have a valid
// environment configuration and returns a new environment observer.
// While waiting for the first environment configuration, it will
// return with tomb.ErrDying if it receives a value on dying.
func NewEnvironObserver(st EnvironConfigObserver) (*EnvironObserver, error) {
	config, err := st.EnvironConfig()
	if err != nil {
		return nil, err
	}
	environ, err := environs.New(config)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create an environment")
	}

	obs := &EnvironObserver{
		st:      st,
		environ: environ,
	}
	err = catacomb.Invoke(catacomb.Plan{
		Site: &obs.catacomb,
		Work: obs.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return obs, nil
}

func (obs *EnvironObserver) loop() error {
	environWatcher, err := obs.st.WatchForEnvironConfigChanges()
	if err != nil {
		return errors.Annotate(err, "cannot watch environment config")
	}
	if err := obs.catacomb.Add(environWatcher); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-obs.catacomb.Dying():
			return obs.catacomb.ErrDying()
		case _, ok := <-environWatcher.Changes():
			if !ok {
				return errors.New("environment config watch closed")
			}
		}
		config, err := obs.st.EnvironConfig()
		if err != nil {
			logger.Warningf("error reading environment config: %v", err)
			continue
		}
		environ, err := environs.New(config)
		if err != nil {
			logger.Warningf("error creating an environment: %v", err)
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

// Kill is part of the worker.Worker interface.
func (obs *EnvironObserver) Kill() {
	obs.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (obs *EnvironObserver) Wait() error {
	return obs.catacomb.Wait()
}
