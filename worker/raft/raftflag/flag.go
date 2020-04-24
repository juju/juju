// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftflag

import (
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"
)

var logger = loggo.GetLogger("juju.worker.raft.raftflag")

// Config holds a raftflag.Worker's dependencies and resources.
type Config struct {
	Raft *raft.Raft
}

// Validate returns an error if the config cannot be expected to run a
// raftflag.Worker.
func (config Config) Validate() error {
	if config.Raft == nil {
		return errors.NotValidf("nil Raft")
	}
	return nil
}

// ErrRefresh indicates that the flag's Check result is no longer valid,
// and a new raftflag.Worker must be started to get a valid result.
var ErrRefresh = errors.New("raft leadership changed, restart worker")

// Worker implements worker.Worker and util.Flag, representing
// controller ownership of a model, such that the Flag's validity is
// tied to the Worker's lifetime.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
	leader   bool
}

func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	flag := &Worker{
		config: config,
		leader: check(config.Raft),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &flag.catacomb,
		Work: flag.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return flag, nil
}

// Kill is part of the worker.Worker interface.
func (flag *Worker) Kill() {
	flag.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (flag *Worker) Wait() error {
	return flag.catacomb.Wait()
}

// Check is part of the util.Flag interface.
//
// Check returns true if the flag indicates that the controller agent is
// the current raft leader.
//
// The validity of this result is tied to the lifetime of the Worker;
// once the worker has stopped, no inferences may be drawn from any Check
// result.
func (flag *Worker) Check() bool {
	return flag.leader
}

func (flag *Worker) loop() error {
	ch := make(chan raft.Observation, 1)
	or := raft.NewObserver(ch, false, func(o *raft.Observation) bool {
		_, ok := o.Data.(raft.RaftState)
		return ok
	})
	flag.config.Raft.RegisterObserver(or)
	defer flag.config.Raft.DeregisterObserver(or)

	for {
		select {
		case <-flag.catacomb.Dying():
			return flag.catacomb.ErrDying()
		case <-ch:
			logger.Debugf("raft state changed: %s", flag.config.Raft.State())
			if check(flag.config.Raft) != flag.leader {
				return ErrRefresh
			}
		}
	}
}

func check(r *raft.Raft) bool {
	return r.State() == raft.Leader
}
