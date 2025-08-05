// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"
)

type modelFirewallRulesWatcher struct {
	catacomb           catacomb.Catacomb
	modelConfigService ModelConfigService

	out chan struct{}

	sshAllowCache set.Strings
}

// NewModelFirewallRulesWatcher returns a worker that notifies when a change to something
// determining the model firewall rules takes place
//
// NOTE: At this time, ssh-allow model config item is the only thing that needs to be watched
func NewModelFirewallRulesWatcher(modelConfigService ModelConfigService) (*modelFirewallRulesWatcher, error) {
	w := &modelFirewallRulesWatcher{
		modelConfigService: modelConfigService,
		out:                make(chan struct{}),
	}

	err := catacomb.Invoke(catacomb.Plan{
		Name: "model-firewall-rules-watcher",
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, err
}

func (w *modelFirewallRulesWatcher) loop() error {
	defer close(w.out)

	ctx, cancel := w.scopedContext()
	defer cancel()

	configWatcher, err := w.modelConfigService.Watch(ctx)
	if err != nil {
		return errors.Trace(err)
	}
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
			sshAllow, err := w.getSSHAllow(ctx)
			if err != nil {
				return errors.Trace(err)
			}
			if !setEquals(sshAllow, w.sshAllowCache) {
				out = w.out
				w.sshAllowCache = sshAllow
			}
		}
	}
}

// scopedContext returns a context that is in the scope of the watcher lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *modelFirewallRulesWatcher) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}

func (w *modelFirewallRulesWatcher) getSSHAllow(ctx context.Context) (set.Strings, error) {
	cfg, err := w.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sshAllow := set.NewStrings(cfg.SSHAllow()...)
	return sshAllow, nil
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
