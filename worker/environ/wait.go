// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environ

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/watcher"
)

// TODO(fwereade) remove WaitForEnviron, use a manifold-managed Tracker to share
// a single environs.Environ among firewaller, instancepoller, provisioner.

var logger = loggo.GetLogger("juju.worker.environ")

// ErrWaitAborted is returned from WaitForEnviron when the wait is terminated by
// closing the abort chan.
var ErrWaitAborted = errors.New("environ wait aborted")

// WaitForEnviron waits for an valid environment to arrive from the given
// watcher. It terminates with ErrWaitAborted if it receives a value on abort.
//
// In practice, it shouldn't wait at all: juju *should* never deliver invalid
// environ configs. Regardless, it should be considered deprecated; clients
// should prefer to access an Environ via a shared Tracker.
//
// It never takes responsibility for the supplied watcher; the client remains
// responsible for detecting and handling any watcher errors that may occur,
// whether this func succeeds or fails.
func WaitForEnviron(
	// TODO(wallyworld) - pass in credential watcher
	w watcher.NotifyWatcher,
	getter environs.EnvironConfigGetter,
	newEnviron environs.NewEnvironFunc,
	abort <-chan struct{},
) (environs.Environ, error) {
	for {
		select {
		case <-abort:
			return nil, ErrWaitAborted
		case _, ok := <-w.Changes():
			if !ok {
				return nil, errors.New("environ config watch closed")
			}
			// First check the model config is valid as we want to exit with
			// an error if we have received a config but it is not valid.
			// This distinguishes from the case where environ construction fails
			// because no config has been received yet.
			if _, err := getter.ModelConfig(); err != nil {
				return nil, errors.Annotate(err, "cannot read environ config")
			}
			environ, err := environs.GetEnviron(getter, newEnviron)
			if err == nil {
				return environ, nil
			}
			logger.Errorf("loaded invalid environment configuration: %v", err)
		}
	}
}
