// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package keyvalue provides a worker that provides a concurrency-safe
way for other workers to share information without depending on
each other (worker sharing information is not affected by restarts
of worker it is sharing information with).

The original motivating use case was the need of the collect metric
manifold to have access to the charm URL of the unit ran by the uniter.
Originally the collect metric manifold did direct queries to the controller
via the API, which meant that metrics collection would stop, should the
unit lose connection to the controller. The uniter knows the charm url of
the charm it is running and can share that information out with other
workers running on the same unit.
*/
package keyvalue

import (
	"path"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

var (
	logger = loggo.GetLogger("juju.worker.keyvalue")
)

// ManifoldConfig identifies resource names upon which the keyvalue
// store manifold depends.
type ManifoldConfig struct {
	AgentName string
}

// Manifold returns a keyvalue store manifold.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			return newKeyValueWorker(config, getResource)
		},
		Output: outputFunc,
	}
}

func newKeyValueWorker(config ManifoldConfig, getResource dependency.GetResourceFunc) (worker.Worker, error) {
	var agent agent.Agent
	if err := getResource(config.AgentName, &agent); err != nil {
		return nil, err
	}

	w := &keyValueWorker{store: newKeyValueStore(path.Join(agent.CurrentConfig().DataDir(), "uniter_keyvalue.yaml"))}
	go func() {
		defer w.tomb.Done()
		<-w.tomb.Dying()
	}()
	return w, nil
}

// Setter sets the value for the specified key.
type Setter interface {
	Set(key string, value interface{}) error
}

// Getter retrieves the value for the specified key.
type Getter interface {
	Get(key string) (interface{}, error)
}

type keyValueWorker struct {
	tomb  tomb.Tomb
	store *KeyValueStore
}

// Kill implements the worker.Worker interface.
func (w *keyValueWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements the worker.Worker interface.
func (w *keyValueWorker) Wait() error {
	return w.tomb.Wait()
}

func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*keyValueWorker)
	outSetter, _ := out.(*Setter)
	outGetter, _ := out.(*Getter)
	if inWorker == nil || (outSetter == nil && outGetter == nil) {
		return errors.Errorf("expected %T-> %T or %T, got %T->%T", inWorker, outSetter, outGetter, in, out)
	}
	if outSetter != nil {
		*outSetter = inWorker.store
		return nil
	}
	*outGetter = inWorker.store
	return nil
}
