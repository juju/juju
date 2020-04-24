// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiconfigwatcher

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/utils/voyeur"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
)

// Logger represents the methods used by the worker to log information.
type Logger interface {
	Debugf(string, ...interface{})
	Errorf(string, ...interface{})
}

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead use the one passed as manifold config.
var logger interface{}

type ManifoldConfig struct {
	AgentName          string
	AgentConfigChanged *voyeur.Value
	Logger             Logger
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
				logger:             config.Logger,
			}
			w.tomb.Go(w.loop)
			return w, nil
		},
	}
}

type apiconfigwatcher struct {
	tomb               tomb.Tomb
	agent              agent.Agent
	agentConfigChanged *voyeur.Value
	addrs              []string
	logger             Logger
}

func (w *apiconfigwatcher) loop() error {
	w.addrs = w.getAPIAddresses()
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
		if !stringSliceEq(w.addrs, w.getAPIAddresses()) {
			w.logger.Debugf("API addresses changed in agent config")
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

func (w *apiconfigwatcher) getAPIAddresses() []string {
	config := w.agent.CurrentConfig()
	addrs, err := config.APIAddresses()
	if err != nil {
		w.logger.Errorf("retrieving API addresses: %s", err)
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
