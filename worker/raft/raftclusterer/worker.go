// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftclusterer

import (
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/pubsub/apiserver"
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
	// Now that we're subscribed, request the current API server details.
	req := apiserver.DetailsRequest{
		Requester: "raft-clusterer",
		LocalOnly: true,
	}
	if _, err := config.Hub.Publish(apiserver.DetailsRequestTopic, req); err != nil {
		return nil, errors.Annotate(err, "requesting current apiserver details")
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

func (w *Worker) getConfiguration() (map[raft.ServerID]*raft.Server, uint64, error) {
	future := w.config.Raft.GetConfiguration()
	prevIndex, err := w.waitIndexFuture(future)
	if err != nil {
		return nil, 0, errors.Trace(err)
	}
	servers := make(map[raft.ServerID]*raft.Server)
	config := future.Configuration()
	for i := range config.Servers {
		server := config.Servers[i]
		servers[server.ID] = &server
	}
	return servers, prevIndex, nil
}

func (w *Worker) updateConfiguration(
	configuredServers map[raft.ServerID]*raft.Server,
	prevIndex uint64,
	details apiserver.Details,
) (uint64, error) {
	newServers := make(map[raft.ServerID]raft.ServerAddress)
	for _, server := range details.Servers {
		if server.InternalAddress == "" {
			continue
		}
		serverID := raft.ServerID(server.ID)
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

func (w *Worker) configOps(configuredServers map[raft.ServerID]*raft.Server, newServers map[raft.ServerID]raft.ServerAddress) []configOp {
	if len(newServers) == 0 {
		logger.Infof("peergrouper reported 0 API server addresses; not removing all servers from cluster")
		return nil
	}
	// This worker can only run on the raft leader, ergo Leader
	// returns the local address.
	localAddr := w.config.Raft.Leader()

	// Add new servers and update addresses first, then remove old
	// servers. If the local server is to be removed, we demote it
	// last, to ensure that the other servers can elect a leader
	// amongst themselves and then remove this server.
	var ops []configOp
	for id, newAddr := range newServers {
		if server, ok := configuredServers[id]; ok {
			if server.Address == newAddr {
				continue
			}
			logger.Infof("server %q (%s) address changed from %q to %q", id, server.Suffrage, server.Address, newAddr)
			// Update the address of the server but take care not to
			// change its suffrage.
			server.Address = newAddr
			if server.Suffrage == raft.Voter {
				ops = append(ops, w.addVoterOp(id, newAddr))
			} else {
				ops = append(ops, w.addNonvoterOp(id, newAddr))
			}
		} else {
			logger.Infof("server %q added with address %q", id, newAddr)
			ops = append(ops, w.addVoterOp(id, newAddr))
			configuredServers[id] = &raft.Server{
				ID:       id,
				Address:  newAddr,
				Suffrage: raft.Voter,
			}
		}
	}
	var demoteLocal *raft.Server
	for id, server := range configuredServers {
		if _, ok := newServers[id]; ok {
			continue
		}
		if server.Address == localAddr {
			demoteLocal = server
			continue
		}
		delete(configuredServers, id)
		logger.Infof("server %q removed", id)
		ops = append(ops, w.removeServerOp(id))
	}
	if demoteLocal != nil {
		// The local (leader) server was removed, so we demote it in
		// the raft configuration. This will prompt another server to
		// become leader, and they can then remove it and make any
		// other changes needed (like ensuring an odd number of
		// voters).
		logger.Infof("leader %q being removed - demoting so the new leader can remove it", demoteLocal.ID)
		demoteLocal.Suffrage = raft.Nonvoter
		ops = append(ops, w.demoteVoterOp(demoteLocal.ID))
		return ops
	}

	// Prevent there from being an even number of voters, to avoid
	// problems from having a split quorum (especially in the case of
	// 2).
	correctionOps := w.correctVoterCountOps(configuredServers, localAddr)
	ops = append(ops, correctionOps...)
	return ops
}

func (w *Worker) correctVoterCountOps(configuredServers map[raft.ServerID]*raft.Server, localAddr raft.ServerAddress) []configOp {
	var voters, nonvoters []*raft.Server
	for _, server := range configuredServers {
		if server.Suffrage == raft.Nonvoter {
			nonvoters = append(nonvoters, server)
		} else {
			voters = append(voters, server)
		}
	}

	// We should have at most one nonvoter, bail if we have more than
	// expected.
	if len(nonvoters) > 1 {
		logger.Errorf("expected at most one nonvoter, found %d: %#v", len(nonvoters), nonvoters)
		return nil
	}

	needNonvoter := len(configuredServers)%2 == 0
	if !needNonvoter && len(nonvoters) == 1 {
		promote := nonvoters[0]
		logger.Infof("promoting nonvoter %q to maintain odd voter count", promote.ID)
		promote.Suffrage = raft.Voter
		return []configOp{w.addVoterOp(promote.ID, promote.Address)}
	}
	if needNonvoter && len(nonvoters) == 0 {
		// Find a voter that isn't us and demote it.
		for _, voter := range voters {
			if voter.Address == localAddr {
				continue
			}
			logger.Infof("demoting voter %q to maintain odd voter count", voter.ID)
			voter.Suffrage = raft.Nonvoter
			return []configOp{w.demoteVoterOp(voter.ID)}
		}
	}

	// No correction needed.
	return nil
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

func (w *Worker) addNonvoterOp(id raft.ServerID, addr raft.ServerAddress) configOp {
	return func(prevIndex uint64) raft.IndexFuture {
		return w.config.Raft.AddNonvoter(id, addr, prevIndex, 0)
	}
}

func (w *Worker) demoteVoterOp(id raft.ServerID) configOp {
	return func(prevIndex uint64) raft.IndexFuture {
		return w.config.Raft.DemoteVoter(id, prevIndex, 0)
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
