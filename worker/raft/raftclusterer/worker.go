// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftclusterer

import (
	"sync"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

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
	// Subscribe to API server address changes.
	unsubscribe, err := config.Hub.Subscribe(
		apiserver.DetailsTopic,
		w.apiserverDetailsChanged,
	)
	if err != nil {
		return nil, errors.Annotate(err, "subscribing to apiserver details")
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: func() error {
			defer unsubscribe()
			return w.loop()
		},
	}); err != nil {
		unsubscribe()
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
	// Get the initial raft cluster configuration.
	servers, prevIndex, err := w.getConfiguration()
	if err != nil {
		return errors.Annotate(err, "getting raft configuration")
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case details := <-w.serverDetails:
			prevIndex, err = w.updateConfiguration(servers, prevIndex, details)
			if err != nil {
				return errors.Annotate(err, "updating raft configuration")
			}
		}
	}
}

func (w *Worker) getConfiguration() (map[raft.ServerID]raft.ServerAddress, uint64, error) {
	future := w.config.Raft.GetConfiguration()
	prevIndex, err := w.waitIndexFuture(future)
	if err != nil {
		return nil, 0, errors.Trace(err)
	}
	servers := make(map[raft.ServerID]raft.ServerAddress)
	config := future.Configuration()
	for _, server := range config.Servers {
		servers[server.ID] = server.Address
	}
	return servers, prevIndex, nil
}

func (w *Worker) updateConfiguration(
	configuredServers map[raft.ServerID]raft.ServerAddress,
	prevIndex uint64,
	details apiserver.Details,
) (uint64, error) {
	newServers := make(map[raft.ServerID]raft.ServerAddress)
	for _, server := range details.Servers {
		if server.InternalAddress == "" {
			continue
		}
		serverID := raft.ServerID(names.NewMachineTag(server.ID).String())
		serverAddress := raft.ServerAddress(server.InternalAddress)
		newServers[serverID] = serverAddress
	}
	ops := w.configOps(configuredServers, newServers)
	for _, op := range ops {
		var err error
		prevIndex, err = w.waitIndexFuture(op(prevIndex))
		if err != nil {
			return 0, errors.Trace(err)
		}
	}
	return prevIndex, nil
}

func (w *Worker) configOps(configuredServers, newServers map[raft.ServerID]raft.ServerAddress) []configOp {
	// This worker can only run on the raft leader, ergo Leader
	// returns the local address.
	localAddr := w.config.Raft.Leader()

	// Add new servers first, then remove old servers,
	// and finish by changing addresses. If the local
	// server is to be removed, we do that last, to
	// ensure that the other servers can communicate
	// amongst themselves.
	var ops []configOp
	for id, newAddr := range newServers {
		if oldAddr, ok := configuredServers[id]; ok {
			if oldAddr == newAddr {
				continue
			}
			logger.Infof("server %q address changed from %q to %q", id, oldAddr, newAddr)
		} else {
			logger.Infof("server %q added with address %q", id, newAddr)
		}
		// NOTE(axw) the docs for AddVoter say that it is ineffective
		// if the server is already a voting member, but that is not
		// correct. Calling AddVoter will update the address for an
		// existing entry.
		ops = append(ops, w.addVoterOp(id, newAddr))
		configuredServers[id] = newAddr
	}
	var removeLocal raft.ServerID
	for id, oldAddr := range configuredServers {
		if _, ok := newServers[id]; ok {
			continue
		}
		logger.Infof("server %q removed", id)
		delete(configuredServers, id)
		if oldAddr == localAddr {
			removeLocal = id
			continue
		}
		ops = append(ops, w.removeServerOp(id))
	}
	if removeLocal != "" {
		// The local (leader) server was removed, so we remove
		// it from the raft configuration. This will prompt
		// another server to become leader.
		ops = append(ops, w.removeServerOp(removeLocal))
	}
	return ops
}

type configOp func(prevIndex uint64) raft.IndexFuture

func (w *Worker) removeServerOp(id raft.ServerID) configOp {
	return func(prevIndex uint64) raft.IndexFuture {
		return w.config.Raft.RemoveServer(id, prevIndex, 0)
	}
}

func (w *Worker) addVoterOp(id raft.ServerID, addr raft.ServerAddress) configOp {
	return func(prevIndex uint64) raft.IndexFuture {
		return w.config.Raft.AddVoter(id, addr, prevIndex, 0)
	}
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
