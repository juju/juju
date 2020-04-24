// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftflag

import (
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// ManifoldConfig holds the information necessary to run a raftflag.Worker in
// a dependency.Engine.
type ManifoldConfig struct {
	RaftName string

	NewWorker func(Config) (worker.Worker, error)
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	var r *raft.Raft
	if err := context.Get(config.RaftName, &r); err != nil {
		return nil, errors.Trace(err)
	}

	flag, err := config.NewWorker(Config{Raft: r})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return wrappedWorker{flag}, nil
}

// wrappedWorker wraps a flag worker, translating ErrRefresh into
// dependency.ErrBounce.
type wrappedWorker struct {
	worker.Worker
}

// Wait is part of the worker.Worker interface.
func (w wrappedWorker) Wait() error {
	err := w.Worker.Wait()
	if err == ErrRefresh {
		err = dependency.ErrBounce
	}
	return err
}

// Manifold returns a dependency.Manifold that will run a FlagWorker and
// expose it to clients as a engine.Flag resource.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.RaftName,
		},
		Start: config.start,
		Output: func(in worker.Worker, out interface{}) error {
			if w, ok := in.(wrappedWorker); ok {
				in = w.Worker
			}
			return engine.FlagOutput(in, out)
		},
	}
}
