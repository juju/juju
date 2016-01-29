// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/watcher"
)

var ErrTerminateAgent = errors.New("agent should be terminated")
var ErrRebootMachine = errors.New("machine needs to reboot")
var ErrShutdownMachine = errors.New("machine needs to shutdown")

var loadedInvalid = func() {}

var logger = loggo.GetLogger("juju.worker")

// ModelConfigGetter interface defines a way to read the environment
// configuration.
type ModelConfigGetter interface {
	ModelConfig() (*config.Config, error)
}

// TODO(rog) remove WaitForEnviron, as we now should always
// start with a valid environ config.

// WaitForEnviron waits for an valid environment to arrive from
// the given watcher. It terminates with tomb.ErrDying if
// it receives a value on dying.
func WaitForModel(w apiwatcher.NotifyWatcher, st ModelConfigGetter, dying <-chan struct{}) (environs.Environ, error) {
	for {
		select {
		case <-dying:
			return nil, tomb.ErrDying
		case _, ok := <-w.Changes():
			if !ok {
				return nil, watcher.EnsureErr(w)
			}
			config, err := st.ModelConfig()
			if err != nil {
				return nil, err
			}
			environ, err := environs.New(config)
			if err == nil {
				return environ, nil
			}
			logger.Errorf("loaded invalid model configuration: %v", err)
			loadedInvalid()
		}
	}
}

// ModelConfigObserver interface defines a way to read the
// model configuration and watch for changes.
type ModelConfigObserver interface {
	ModelConfigGetter
	WatchForModelConfigChanges() (apiwatcher.NotifyWatcher, error)
}

// ModelObserver watches the current model configuration
// and makes it available. It discards invalid environment
// configurations.
type ModelObserver struct {
	tomb         tomb.Tomb
	modelWatcher apiwatcher.NotifyWatcher
	st           ModelConfigObserver
	mu           sync.Mutex
	environ      environs.Environ
}

// NewModelObserver waits for the model to have a valid
// model configuration and returns a new model observer.
// While waiting for the first model configuration, it will
// return with tomb.ErrDying if it receives a value on dying.
func NewModelObserver(st ModelConfigObserver) (*ModelObserver, error) {
	config, err := st.ModelConfig()
	if err != nil {
		return nil, err
	}
	environ, err := environs.New(config)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create a model")
	}
	modelWatcher, err := st.WatchForModelConfigChanges()
	if err != nil {
		return nil, errors.Annotate(err, "cannot watch model config")
	}
	obs := &ModelObserver{
		st:           st,
		environ:      environ,
		modelWatcher: modelWatcher,
	}
	go func() {
		defer obs.tomb.Done()
		defer watcher.Stop(modelWatcher, &obs.tomb)
		obs.tomb.Kill(obs.loop())
	}()
	return obs, nil
}

func (obs *ModelObserver) loop() error {
	for {
		select {
		case <-obs.tomb.Dying():
			return nil
		case _, ok := <-obs.modelWatcher.Changes():
			if !ok {
				return watcher.EnsureErr(obs.modelWatcher)
			}
		}
		config, err := obs.st.ModelConfig()
		if err != nil {
			logger.Warningf("error reading model config: %v", err)
			continue
		}
		environ, err := environs.New(config)
		if err != nil {
			logger.Warningf("error creating a model: %v", err)
			continue
		}
		obs.mu.Lock()
		obs.environ = environ
		obs.mu.Unlock()
	}
}

// Environ returns the most recent valid Environ.
func (obs *ModelObserver) Environ() environs.Environ {
	obs.mu.Lock()
	defer obs.mu.Unlock()
	return obs.environ
}

func (obs *ModelObserver) Kill() {
	obs.tomb.Kill(nil)
}

func (obs *ModelObserver) Wait() error {
	return obs.tomb.Wait()
}
