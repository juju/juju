// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environ

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

// ConfigGetter exposes an environment configuration to its clients.
type ConfigGetter interface {
	EnvironConfig() (*config.Config, error)
}

// ConfigObserver exposes an environment configuration and a watch constructor
// that allows clients to be informed of changes to the configuration.
type ConfigObserver interface {
	ConfigGetter
	WatchForEnvironConfigChanges() (watcher.NotifyWatcher, error)
}

// Config describes the dependencies of a Tracker.
//
// It's arguable that it should be called TrackerConfig, because of the heavy
// use of environ config in this package.
type Config struct {
	Observer ConfigObserver
}

// Validate returns an error if the config cannot be used to start a Tracker.
func (config Config) Validate() error {
	if config.Observer == nil {
		return errors.NotValidf("nil Observer")
	}
	return nil
}

// Tracker loads an environment, makes it available to clients, and updates
// the environment in response to config changes until it is killed.
type Tracker struct {
	config   Config
	catacomb catacomb.Catacomb
	environ  environs.Environ
}

// NewTracker loads an environment from the observer and returns a new Tracker,
// or an error if anything goes wrong. If a tracker is returned, its Environ()
// method is immediately usable.
//
// The caller is responsible for Kill()ing the returned Tracker and Wait()ing
// for any errors it might return.
func NewTracker(config Config) (*Tracker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	environConfig, err := config.Observer.EnvironConfig()
	if err != nil {
		return nil, errors.Annotate(err, "cannot read environ config")
	}
	environ, err := environs.New(environConfig)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create environ")
	}

	t := &Tracker{
		config:  config,
		environ: environ,
	}
	err = catacomb.Invoke(catacomb.Plan{
		Site: &t.catacomb,
		Work: t.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return t, nil
}

// Environ returns the encapsulated Environ. It will continue to be updated in
// the background for as long as the Tracker continues to run.
func (t *Tracker) Environ() environs.Environ {
	return t.environ
}

func (t *Tracker) loop() error {
	environWatcher, err := t.config.Observer.WatchForEnvironConfigChanges()
	if err != nil {
		return errors.Annotate(err, "cannot watch environ config")
	}
	if err := t.catacomb.Add(environWatcher); err != nil {
		return errors.Trace(err)
	}
	for {
		logger.Debugf("waiting for environ watch notification")
		select {
		case <-t.catacomb.Dying():
			return t.catacomb.ErrDying()
		case _, ok := <-environWatcher.Changes():
			if !ok {
				return errors.New("environ config watch closed")
			}
		}
		logger.Debugf("reloading environ config")
		environConfig, err := t.config.Observer.EnvironConfig()
		if err != nil {
			return errors.Annotate(err, "cannot read environ config")
		}
		if err = t.environ.SetConfig(environConfig); err != nil {
			return errors.Annotate(err, "cannot update environ config")
		}
	}
}

// Kill is part of the worker.Worker interface.
func (t *Tracker) Kill() {
	t.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (t *Tracker) Wait() error {
	return t.catacomb.Wait()
}
