// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftclusterer

import (
	"sync"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/worker/catacomb"
)

var (
	logger = loggo.GetLogger("juju.worker.raft.raftclusterer")
)

// Config holds the configuration necessary to run a worker for
// maintaining the raft cluster configuration.
type Config struct {
	Raft *raft.Raft
	Hub  *pubsub.StructuredHub
}

// Validate validates the raft worker configuration.
func (config Config) Validate() error {
	if config.Hub == nil {
		return errors.NotValidf("nil Hub")
	}
	if config.Raft == nil {
		return errors.NotValidf("nil Raft")
	}
	return nil
}

// NewWorker returns a new worker responsible for maintaining
// the raft cluster configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &Worker{
		config:        config,
		serverDetails: make(chan apiserver.Details),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Worker is a worker that manages raft cluster configuration.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config

	mu            sync.Mutex
	serverDetails chan apiserver.Details
}

// Kill is part of the worker.Worker interface.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() error {
	// Subscribe to API server address changes.
	unsubscribe, err := w.config.Hub.Subscribe(
		apiserver.DetailsTopic,
		w.apiserverDetailsChanged,
	)
	if err != nil {
		return errors.Annotate(err, "subscribing to apiserver details")
	}
	defer unsubscribe()

	// Get the initial raft cluster configuration.
	serverIDs, err := w.getConfiguration()
	if err != nil {
		return errors.Annotate(err, "getting raft configuration")
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case details := <-w.serverDetails:
			if err := w.updateConfiguration(serverIDs, details); err != nil {
				return errors.Annotate(err, "updating raft configuration")
			}
		}
	}
}

func (w *Worker) getConfiguration() (serverIDs set.Strings, _ error) {
	future := w.config.Raft.GetConfiguration()
	if _, err := w.waitIndexFuture(future); err != nil {
		return nil, errors.Trace(err)
	}
	serverIDs = set.NewStrings()
	for _, server := range future.Configuration().Servers {
		serverIDs.Add(string(server.ID))
	}
	return serverIDs, nil
}

func (w *Worker) updateConfiguration(configuredServerIDs set.Strings, details apiserver.Details) error {
	newServerIDs := set.NewStrings()
	for _, server := range details.Servers {
		newServerIDs.Add(names.NewMachineTag(server.ID).String())
	}
	var futures []raft.IndexFuture
	for _, serverID := range configuredServerIDs.Difference(newServerIDs).Values() {
		configuredServerIDs.Remove(serverID)
		f := w.config.Raft.RemoveServer(raft.ServerID(serverID), 0, 0)
		futures = append(futures, f)
	}
	for _, serverID := range newServerIDs.Difference(configuredServerIDs).Values() {
		configuredServerIDs.Add(serverID)
		raftServerID := raft.ServerID(serverID)
		raftServerAddress := raft.ServerAddress(serverID)
		f := w.config.Raft.AddVoter(raftServerID, raftServerAddress, 0, 0)
		futures = append(futures, f)
	}
	for _, f := range futures {
		if _, err := w.waitIndexFuture(f); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// waitIndexFuture waits for the future to return, or for the worker
// to be killed, whichever happens first. If the worker is dying, then
// the catacomb's ErrDying() is returned.
func (w *Worker) waitIndexFuture(f raft.IndexFuture) (uint64, error) {
	errch := make(chan error, 1)
	go func() {
		errch <- f.Error()
	}()
	select {
	case <-w.catacomb.Dying():
		return 0, w.catacomb.ErrDying()
	case err := <-errch:
		return f.Index(), err
	}
}

func (w *Worker) apiserverDetailsChanged(topic string, details apiserver.Details, err error) {
	if err != nil {
		// This should never happen, so treat it as fatal.
		w.catacomb.Kill(errors.Annotate(err, "apiserver details callback failed"))
		return
	}
	select {
	case w.serverDetails <- details:
	case <-w.catacomb.Dying():
	}
}
