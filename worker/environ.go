package worker
import (
	"launchpad.net/tomb"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
)

var loadedInvalid = func(){}

// WaitForEnviron waits for an valid environment to arrive from
// the given watcher. It terminates with tomb.ErrDying if
// it receives a value on stopped.
func WaitForEnviron(w *state.EnvironConfigWatcher, stop <-chan struct{}) (environs.Environ, error) {
	for {
		select {
		case <-stop:
			return nil, tomb.ErrDying
		case config, ok := <-w.Changes():
			if !ok {
				return nil, w.Err()
			}
			var err error
			environ, err := environs.New(config)
			if err == nil {
				log.Printf("loaded new environment configuration")
				return environ, nil
			}
			log.Printf("firewaller loaded invalid environment configuration: %v", err)
			loadedInvalid()
		}
	}
	panic("not reached")
}
