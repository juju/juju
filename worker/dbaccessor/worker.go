// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	dqApp "github.com/canonical/go-dqlite/app"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/worker/v3/catacomb"
)

type DBGetter interface {
	GetDB(modelUUID string) (*sql.DB, error)
	GetExistingDB(modelUUID string) (*sql.DB, error)
}

// Hub provides an API for publishing and subscribing to incoming messages.
type Hub interface {
	Publish(topic string, data interface{}) (func(), error)
	Subscribe(topic string, handler interface{}) (func(), error)
}

// WorkerConfig encapsulates the configuration options for the
// dbaccessor worker.
type WorkerConfig struct {
	AgentMachineID string
	DataDir        string
	Hub            Hub
	Clock          clock.Clock
	Logger         Logger
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.AgentMachineID == "" {
		return errors.NotValidf("missing agent machine ID")
	}
	if c.DataDir == "" {
		return errors.NotValidf("missing data dir")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing clock")
	}
	if c.Hub == nil {
		return errors.NotValidf("missing hub")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	return nil
}

type dbWorker struct {
	cfg      WorkerConfig
	catacomb catacomb.Catacomb

	unsubServerDetailsFn func()
	apiServerChangesCh   chan apiserver.Details

	mu            sync.RWMutex
	dqliteApp     *dqApp.App
	dqliteReadyCh chan struct{}
	dbHandles     map[string]*sql.DB
}

func NewWorker(cfg WorkerConfig) (*dbWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &dbWorker{
		cfg:                cfg,
		dbHandles:          make(map[string]*sql.DB),
		apiServerChangesCh: make(chan apiserver.Details, 1),
		dqliteReadyCh:      make(chan struct{}),
	}

	// Subscribe for server information
	w.unsubServerDetailsFn, err = w.cfg.Hub.Subscribe(
		apiserver.DetailsTopic,
		func(topic string, details apiserver.Details, err error) {
			if err != nil { // This should never happen.
				w.cfg.Logger.Errorf("subscriber callback error: %v", err)
				return
			}

			select {
			case <-w.catacomb.Dying():
			case w.apiServerChangesCh <- details:
				// enqueued for processing by main loop.
			}
		},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *dbWorker) loop() (err error) {
	defer func() {
		w.unsubServerDetailsFn()
		w.mu.Lock()
		if w.dqliteApp != nil {
			if hErr := w.dqliteApp.Handover(context.TODO()); hErr != nil {
				if err == nil {
					err = errors.Annotate(err, "gracefully handing over dqlite node responsibilities")
				} else { // we are exiting with another error; so we just log this.
					w.cfg.Logger.Errorf("unable to gracefully hand off dqlite node responsibilities: %v", hErr)
				}
			}

			if cErr := w.dqliteApp.Close(); cErr != nil {
				if err == nil {
					err = errors.Annotate(cErr, "closing dqlite application instance")
				} else { // we are exiting with another error; so we just log this.
					w.cfg.Logger.Errorf("unable to close dqlite application instance: %v", cErr)
				}
			}
			w.dqliteApp = nil
		}
		w.mu.Unlock()
	}()

	// Request a snapshot of the current server details.
	detailsRequest := apiserver.DetailsRequest{
		Requester: "db-accessor",
		LocalOnly: true,
	}

	if _, err := w.cfg.Hub.Publish(apiserver.DetailsRequestTopic, detailsRequest); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case details := <-w.apiServerChangesCh:
			if len(details.Servers) == 0 {
				w.cfg.Logger.Warningf("received empty API server list; publishing new details request")
				if _, err := w.cfg.Hub.Publish(apiserver.DetailsRequestTopic, detailsRequest); err != nil {
					return errors.Trace(err)
				}
				continue
			}

			if err := w.updateDqliteClusterMembers(details); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *dbWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *dbWorker) Wait() error {
	return w.catacomb.Wait()
}

// GetDB returns a sql.DB handle for the dqlite-backed database that contains
// the data for the specified model UUID.
func (w *dbWorker) GetDB(modelUUID string) (*sql.DB, error) {
	select {
	case <-w.dqliteReadyCh:
		// dqlite has been initialized
	case <-w.catacomb.Dying():
		return nil, w.catacomb.ErrDying()
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if a DB handle has already been created and cached.
	if cachedDB := w.dbHandles[modelUUID]; cachedDB != nil {
		return cachedDB, nil
	}

	// Create and cache DB instance.
	db, err := w.dqliteApp.Open(context.TODO(), modelUUID)
	if err != nil {
		return nil, errors.Annotatef(err, "acccessing DB for model %q", modelUUID)
	}
	if err := ensureDBSchema(modelUUID, db); err != nil {
		return nil, errors.Annotatef(err, "acccessing DB for model %q", modelUUID)
	}
	w.dbHandles[modelUUID] = db

	return db, nil
}

// GetExistingDB returns a sql.DB handle for the dqlite-backed database that
// contains the data for the specified model UUID or a NotFound error if the
// DB is not known.
func (w *dbWorker) GetExistingDB(modelUUID string) (*sql.DB, error) {
	select {
	case <-w.dqliteReadyCh:
		// dqlite has been initialized
	case <-w.catacomb.Dying():
		return nil, w.catacomb.ErrDying()
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if a DB handle has already been created and cached.
	if cachedDB := w.dbHandles[modelUUID]; cachedDB != nil {
		return cachedDB, nil
	}

	return nil, errors.NotFoundf("database for model %q", modelUUID)
}

func (w *dbWorker) updateDqliteClusterMembers(details apiserver.Details) error {
	w.cfg.Logger.Tracef("updateDqliteClusterMembers: %#v", details)

	w.mu.Lock()
	defer w.mu.Unlock()

	// Dqlite has already been initialized
	if w.dqliteApp != nil {
		return nil
	}

	if err := w.initializeDqlite(details); err != nil {
		return errors.Annotatef(err, "initializing dqlite application")
	}

	// At this point we have a functioning dqlite cluster so we don't need
	// to listen for future peer changes. New peers can join the cluster
	// and announced to us simply by connecting to any of the already
	// established DB endpoints.
	w.unsubServerDetailsFn()

	w.cfg.Logger.Tracef("updateDqliteClusterMembers: processed all changes")
	return nil
}

func (w *dbWorker) initializeDqlite(details apiserver.Details) error {
	// Split the server list into a local peer and a list of remote peers
	// to connect to. If this is the initial (bootstrapping) node of the
	// cluster, the remote peer list will be empty.
	var localPeer apiserver.APIServer
	var remotePeers []string
	for id, apiServer := range details.Servers {
		target := names.NewControllerAgentTag(id).Id()
		if target == w.cfg.AgentMachineID {
			localPeer = apiServer
			continue
		}

		// Calculate the dqlite endpoint address from the remote API address.
		remoteAddr, err := dqliteAddressForServer(apiServer)
		if err != nil {
			return errors.Annotatef(err, "generating remote address for dqlite endpoint")
		}
		remotePeers = append(remotePeers, remoteAddr)
	}

	if localPeer.ID == "" {
		return errors.Errorf("unable to locate local address for controller")
	}

	if err := os.MkdirAll(w.cfg.DataDir, 0700); err != nil {
		return errors.Annotatef(err, "creating directory for dqlite data")
	}

	localAddr, err := dqliteAddressForServer(localPeer)
	if err != nil {
		return errors.Annotatef(err, "generating remote address for dqlite endpoint")
	}

	w.cfg.Logger.Infof("initializing dqlite application with local address %q and peer addresses %v", localAddr, remotePeers)
	if w.dqliteApp, err = dqApp.New(w.cfg.DataDir, dqApp.WithAddress(localAddr), dqApp.WithCluster(remotePeers)); err != nil {
		return errors.Trace(err)
	}

	// Ensure dqlite is ready to acccept new changes
	if err := w.dqliteApp.Ready(context.TODO()); err != nil {
		_ = w.dqliteApp.Close()
		w.dqliteApp = nil
		return errors.Annotatef(err, "ensuring dqlite is ready to process changes")
	}

	w.cfg.Logger.Infof("initialized dqlite application (ID: %v)", w.dqliteApp.ID())
	close(w.dqliteReadyCh) // start accepting GetDB() requests.
	return nil
}

func dqliteAddressForServer(apiServer apiserver.APIServer) (string, error) {
	var apiAddress string
	if apiServer.InternalAddress != "" { // always prefer internal addresses
		apiAddress = apiServer.InternalAddress
	} else if len(apiServer.Addresses) != 0 {
		apiAddress = apiServer.Addresses[0]
	} else {
		return "", errors.Errorf("no usable addresses for %q", apiServer.ID)
	}

	tokens := strings.Split(apiAddress, ":")
	if len(tokens) != 2 {
		return "", errors.NotValidf("API address %q", apiAddress)
	}

	apiPort, err := strconv.Atoi(tokens[1])
	if err != nil {
		return "", errors.Annotatef(err, "parsing port component of API address %q", apiAddress)
	}

	return fmt.Sprintf("%s:%d", tokens[0], apiPort-1), nil
}
