package worker

import (
	"errors"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

var ErrDead = errors.New("agent entity is dead")

var loadedInvalid = func() {}

// WaitForEnviron waits for an valid environment to arrive from
// the given watcher. It terminates with tomb.ErrDying if
// it receives a value on dying.
func WaitForEnviron(w *state.EnvironConfigWatcher, dying <-chan struct{}) (environs.Environ, error) {
	for {
		select {
		case <-dying:
			return nil, tomb.ErrDying
		case config, ok := <-w.Changes():
			if !ok {
				return nil, watcher.MustErr(w)
			}
			environ, err := environs.New(config)
			if err == nil {
				return environ, nil
			}
			log.Printf("loaded invalid environment configuration: %v", err)
			loadedInvalid()
		}
	}
	panic("not reached")
}
