// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiconfigwatcher

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/voyeur"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

var logger = loggo.GetLogger("juju.worker.apiconfigwatcher")

type ManifoldConfig struct {
	AgentName          string
	AgentConfigChanged *voyeur.Value
}

// Manifold returns a dependency.Manifold which wraps an agent's
// voyeur.Value which is set whenever the agent config is
// changed. When the API server addresses in the config change the
// manifold will bounce itself.
//
// The manifold is intended to be a dependency for the api-caller
// manifold and is required to support model migrations.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.AgentName},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if config.AgentConfigChanged == nil {
				return nil, errors.NotValidf("nil AgentConfigChanged")
			}

			var a agent.Agent
			if err := context.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			w := &apiconfigwatcher{
				agent:              a,
				agentConfigChanged: config.AgentConfigChanged,
				addrs:              getAPIAddresses(a),
			}
			go func() {
				defer w.tomb.Done()
				w.tomb.Kill(w.loop())
			}()
			return w, nil
		},
	}
}

type apiconfigwatcher struct {
	tomb               tomb.Tomb
	agent              agent.Agent
	agentConfigChanged *voyeur.Value
	addrs              []string
}

func (w *apiconfigwatcher) loop() error {
	watch := w.agentConfigChanged.Watch()
	defer watch.Close()

	// TODO(mjs) - this is pretty awful. There should be a
	// NotifyWatcher for voyeur.Value. Note also that this code is
	// repeated elsewhere.
	watchCh := make(chan bool)
	go func() {
		for {
			if watch.Next() {
				select {
				case <-w.tomb.Dying():
					return
				case watchCh <- true:
				}
			} else {
				// watcher or voyeur.Value closed.
				close(watchCh)
				return
			}
		}
	}()

	for {
		// Always unconditionally check for a change in API addresses
		// first, in case there was a change between the start func
		// and the call to Watch.
		if !stringSliceEq(w.addrs, getAPIAddresses(w.agent)) {
			logger.Debugf("API addresses changed in agent config")
			return dependency.ErrBounce
		}

		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case _, ok := <-watchCh:
			if !ok {
				return errors.New("config changed value closed")
			}
		}
	}
}

// Kill implements worker.Worker.
func (w *apiconfigwatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *apiconfigwatcher) Wait() error {
	return w.tomb.Wait()
}

func getAPIAddresses(a agent.Agent) []string {
	config := a.CurrentConfig()
	addrs, err := config.APIAddresses()
	if err != nil {
		logger.Errorf("retrieving API addresses: %s", err)
		addrs = nil
	}
	sort.Strings(addrs)
	return addrs
}

func stringSliceEq(a, b []string) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
