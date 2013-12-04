// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"errors"

	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state/api/params"
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

// WaitForEnviron waits for an valid environment to arrive from
// the given watcher. It terminates with tomb.ErrDying if
// it receives a value on dying.
func WaitForEnviron(w apiwatcher.EnvironConfigWatcher, st EnvironConfigGetter, dying <-chan struct{}) (environs.Environ, error) {
	for {
		select {
		case <-dying:
			return nil, tomb.ErrDying
		case configAttr, ok := <-w.Changes():
			if !ok {
				return nil, watcher.MustErr(w)
			}
			environ, err := newEnviron(configAttr)
			if err == nil {
				return environ, nil
			}
			loadedInvalid()
		}
	}
}

func newEnviron(configAttr params.EnvironConfig) (environs.Environ, error) {
	conf, err := config.New(config.NoDefaults, configAttr)
	if err != nil {
		logger.Errorf("received invalid environ config: %v", err)
		return nil, err
	}
	environ, err := environs.New(conf)
	if err != nil {
		logger.Errorf("error creating Environ: %v", err)
		return nil, err
	}
	return environ, nil
}
