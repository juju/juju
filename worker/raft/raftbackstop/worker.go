// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftbackstop

import (
	"bytes"

	"github.com/hashicorp/go-msgpack/codec"
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/pubsub/apiserver"
)

// RaftNode captures the part of the *raft.Raft API needed by the
// backstop worker.
type RaftNode interface {
	State() raft.RaftState
	GetConfiguration() raft.ConfigurationFuture
	BootstrapCluster(configuration raft.Configuration) raft.Future
}

// Logger represents the logging methods called.
type Logger interface {
	Infof(message string, args ...interface{})
	Debugf(message string, args ...interface{})
	Tracef(message string, args ...interface{})
}

// This worker monitors the state of the raft cluster it's passed, and
// if it detects that there is only one remaining API server that is
// non-voting, will force-append a configuration message to the raft
// log to enable it to recover. This allows us to handle the situation
// where we had two remaining machines (a leader and a non-voting
// follower) but the leader was removed.

// Config holds the values needed by the worker.
type Config struct {
	Raft     RaftNode
	LogStore raft.LogStore
	Hub      *pubsub.StructuredHub
	Logger   Logger
	LocalID  raft.ServerID
}

// Validate validates the raft worker configuration.
func (config Config) Validate() error {
	if config.Hub == nil {
		return errors.NotValidf("nil Hub")
	}
	if config.Raft == nil {
		return errors.NotValidf("nil Raft")
	}
	if config.LogStore == nil {
		return errors.NotValidf("nil LogStore")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.LocalID == "" {
		return errors.NotValidf("empty LocalID")
	}
	return nil
}

// NewWorker returns a worker responsible for recovering the raft
// cluster when it's been reduced to one server that can't become
// leader.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &backstopWorker{
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
		Requester: "raft-backstop",
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

type backstopWorker struct {
	catacomb catacomb.Catacomb
	config   Config

	serverDetails   chan apiserver.Details
	configUpdated   bool
	reportedWaiting bool
}

// Kill is part of the worker.Worker interface.
func (w *backstopWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *backstopWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *backstopWorker) loop() error {
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case details := <-w.serverDetails:
			err := w.maybeRecoverCluster(details)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (w *backstopWorker) maybeRecoverCluster(details apiserver.Details) error {
	w.config.Logger.Tracef("maybeRecoverCluster called with %#v", details)
	if w.configUpdated {
		if !w.reportedWaiting {
			w.config.Logger.Infof("raft configuration already updated; waiting for raft worker restart")
			w.reportedWaiting = true
		}
		return nil
	}
	if len(details.Servers) != 1 {
		return nil
	}
	if _, found := details.Servers[string(w.config.LocalID)]; !found {
		return nil
	}
	if w.config.Raft.State() == raft.Leader {
		return nil
	}

	raftServers, err := w.getConfiguration()
	if err != nil {
		return errors.Annotate(err, "getting raft configuration")
	}

	numServers := len(raftServers)
	if numServers == 0 && len(details.Servers) == 1 {
		// we know it is found because we checked earlier
		// TODO(jam): 2019-03-31 Would we want to handle the case where len(details.Servers) > 1
		//  You could potentially get into that case if you were in HA but one of the
		//  machines had 'raft/' deleted. But hopefully the HA properties will
		//  allow one of the controllers to recover. What if all 3 are removed
		//  simultaneously?
		localDetails := details.Servers[string(w.config.LocalID)]
		localServer := &raft.Server{
			Suffrage: raft.Nonvoter,
			ID:       w.config.LocalID,
			Address:  raft.ServerAddress(localDetails.InternalAddress),
		}
		return errors.Trace(w.recoverViaBootstrap(localServer))
	}
	localServer := raftServers[w.config.LocalID]
	if localServer == nil {
		return nil
	}

	localVote := localServer.Suffrage != raft.Nonvoter
	if numServers == 1 && localVote {
		// The server can vote and has quorum, so it can become leader.
		return nil
	}
	err = w.recoverCluster(localServer)
	return errors.Annotate(err, "recovering cluster")
}

func (w *backstopWorker) recoverCluster(server *raft.Server) error {
	if server == nil {
		return errors.Errorf("nil *server passed to recoverCluster")
	}
	w.config.Logger.Infof("remaining controller machine can't become raft leader - recovering the cluster")

	// Make a configuration that can be written into the log, and append it.
	newServer := *server
	newServer.Suffrage = raft.Voter
	configuration := raft.Configuration{
		Servers: []raft.Server{newServer},
	}
	w.config.Logger.Debugf("appending recovery configuration: %#v", configuration)
	data, err := encodeConfiguration(configuration)
	if err != nil {
		return errors.Trace(err)
	}

	// Work out the last term and index.
	lastIndex, err := w.config.LogStore.LastIndex()
	if err != nil {
		return errors.Annotate(err, "getting last log index")
	}
	var lastLog raft.Log
	err = w.config.LogStore.GetLog(lastIndex, &lastLog)
	if err != nil {
		return errors.Annotatef(err, "getting last log entry %d", lastIndex)
	}

	// Prepare log record to add.
	record := raft.Log{
		Index: lastIndex + 1,
		Term:  lastLog.Term,
		Type:  raft.LogConfiguration,
		Data:  data,
	}
	if err := w.config.LogStore.StoreLog(&record); err != nil {
		return errors.Annotate(err, "storing recovery configuration")
	}
	w.configUpdated = true
	return nil
}

// recoverViaBootstrap re-initializes the raft cluster. This should only be
// used in the case that the raft directory has been completely wiped.
func (w *backstopWorker) recoverViaBootstrap(server *raft.Server) error {
	newServer := *server
	newServer.Suffrage = raft.Voter
	configuration := raft.Configuration{
		Servers: []raft.Server{newServer},
	}
	w.config.Logger.Infof("rebootstrapping raft configuration: %#v", configuration)
	err := w.config.Raft.BootstrapCluster(configuration).Error()
	if err != nil {
		return errors.Annotate(err, "re-bootstrapping cluster")
	}
	return nil
}

func encodeConfiguration(config raft.Configuration) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	hd := codec.MsgpackHandle{}
	enc := codec.NewEncoder(buf, &hd)
	err := enc.Encode(config)
	return buf.Bytes(), err
}

func (w *backstopWorker) getConfiguration() (map[raft.ServerID]*raft.Server, error) {
	future := w.config.Raft.GetConfiguration()
	err := w.waitFuture(future)
	if err != nil {
		return nil, errors.Trace(err)
	}
	servers := make(map[raft.ServerID]*raft.Server)
	config := future.Configuration()
	for i := range config.Servers {
		server := config.Servers[i]
		servers[server.ID] = &server
	}
	return servers, nil
}

// waitFuture waits for the future to return, or for the worker to be
// killed, whichever happens first. If the worker is dying, then the
// catacomb's ErrDying() is returned.
func (w *backstopWorker) waitFuture(f raft.Future) error {
	errch := make(chan error, 1)
	go func() {
		errch <- f.Error()
	}()
	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	case err := <-errch:
		return err
	}
}

func (w *backstopWorker) apiserverDetailsChanged(topic string, details apiserver.Details, err error) {
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
