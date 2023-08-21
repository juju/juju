// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/worker/v3/catacomb"
)

type modelFirewallRulesWatcher struct {
	catacomb catacomb.Catacomb
	backend  State

	out chan struct{}

	sshAllowCache set.Strings
}

// NewModelFirewallRulesWatcher returns a worker that notifies when a change to something
// determining the model firewall rules takes place
//
// NOTE: At this time, ssh-allow model config item is the only thing that needs to be watched
func NewModelFirewallRulesWatcher(st State) (*modelFirewallRulesWatcher, error) {
	w := &modelFirewallRulesWatcher{
		backend: st,
		out:     make(chan struct{}),
	}

	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, err
}

func (w *modelFirewallRulesWatcher) loop() error {
	defer close(w.out)

	configWatcher := w.backend.WatchForModelConfigChanges()
	if err := w.catacomb.Add(configWatcher); err != nil {
		return errors.Trace(err)
	}

	var out chan struct{}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case out <- struct{}{}:
			out = nil
		case _, ok := <-configWatcher.Changes():
			if !ok {
				return w.catacomb.ErrDying()
			}
			cfg, err := w.backend.ModelConfig(context.TODO())
			if err != nil {
				return errors.Trace(err)
			}
			sshAllow := set.NewStrings(cfg.SSHAllow()...)
			if !setEquals(sshAllow, w.sshAllowCache) {
				out = w.out
				w.sshAllowCache = sshAllow
			}
		}
	}
}

func (w *modelFirewallRulesWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *modelFirewallRulesWatcher) Kill() {
	w.catacomb.Kill(nil)
}

func (w *modelFirewallRulesWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *modelFirewallRulesWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *modelFirewallRulesWatcher) Err() error {
	return w.catacomb.Err()
}
