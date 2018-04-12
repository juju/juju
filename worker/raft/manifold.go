// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"path/filepath"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig holds the information necessary to run a raft
// worker in a dependency.Engine.
type ManifoldConfig struct {
	AgentName     string
	TransportName string

	FSM       raft.FSM
	Logger    loggo.Logger
	NewWorker func(Config) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.TransportName == "" {
		return errors.NotValidf("empty TransportName")
	}
	if config.FSM == nil {
		return errors.NotValidf("nil FSM")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a raft worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.TransportName,
		},
		Start:  config.start,
		Output: raftOutput,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var transport raft.Transport
	if err := context.Get(config.TransportName, &transport); err != nil {
		return nil, errors.Trace(err)
	}

	// TODO(axw) make the directory path configurable, so we can
	// potentially have multiple Rafts. The dqlite raft should go
	// in <data-dir>/dqlite.
	agentConfig := agent.CurrentConfig()
	raftDir := filepath.Join(agentConfig.DataDir(), "raft")

	return config.NewWorker(Config{
		FSM:        config.FSM,
		Logger:     config.Logger,
		StorageDir: raftDir,
		Tag:        agentConfig.Tag(),
		Transport:  transport,
	})
}

func raftOutput(in worker.Worker, out interface{}) error {
	w, ok := in.(withRaft)
	if !ok {
		return errors.Errorf("expected input of type %T, got %T", w, in)
	}
	rout, ok := out.(**raft.Raft)
	if ok {
		r, err := w.Raft()
		if err != nil {
			return err
		}
		*rout = r
		return nil
	}
	return errors.Errorf("expected output of type %T, got %T", rout, out)
}

type withRaft interface {
	Raft() (*raft.Raft, error)
}
