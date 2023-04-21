// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"github.com/juju/worker/v3/dependency"
	"gopkg.in/tomb.v2"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/database/app"
	"github.com/juju/juju/database/dqlite"
	"github.com/juju/juju/pubsub/apiserver"
)

// nodeShutdownTimeout is the timeout that we add to the context passed
// handoff/shutdown calls when shutting down the Dqlite node.
const nodeShutdownTimeout = 30 * time.Second

// NodeManager creates Dqlite `App` initialisation arguments and options.
type NodeManager interface {
	// IsExistingNode returns true if this machine of container has run a
	// Dqlite node in the past.
	IsExistingNode() (bool, error)

	// IsBootstrappedNode returns true if this machine or container was where
	// we first bootstrapped Dqlite, and it hasn't been reconfigured since.
	IsBootstrappedNode(context.Context) (bool, error)

	// EnsureDataDir ensures that a directory for Dqlite data exists at
	// a path determined by the agent config, then returns that path.
	EnsureDataDir() (string, error)

	// ClusterServers returns the node information for
	// Dqlite nodes configured to be in the cluster.
	ClusterServers(context.Context) ([]dqlite.NodeInfo, error)

	//SetClusterServers reconfigures the Dqlite cluster members.
	SetClusterServers(context.Context, []dqlite.NodeInfo) error

	// SetNodeInfo rewrites the local node information
	// file in the Dqlite data directory.
	SetNodeInfo(dqlite.NodeInfo) error

	// WithLogFuncOption returns a Dqlite application Option that will proxy Dqlite
	// log output via this factory's logger where the level is recognised.
	WithLogFuncOption() app.Option

	// WithTracingOption returns a Dqlite application Option
	// that will enable tracing of Dqlite operations.
	WithTracingOption() app.Option

	// WithAddressOption returns a Dqlite application Option
	// for specifying the local address:port to use.
	WithAddressOption(string) app.Option

	// WithTLSOption returns a Dqlite application Option for TLS encryption
	// of traffic between clients and clustered application nodes.
	WithTLSOption() (app.Option, error)

	// WithClusterOption returns a Dqlite application Option for initialising
	// Dqlite as the member of a cluster with peers representing other controllers.
	WithClusterOption([]string) app.Option
}

// DBGetter describes the ability to supply a sql.DB
// reference for a particular database.
type DBGetter interface {
	// GetDB returns a sql.DB reference for the dqlite-backed database that
	// contains the data for the specified namespace.
	// A NotFound error is returned if the worker is unaware of the requested DB.
	GetDB(namespace string) (coredatabase.TrackedDB, error)
}

// dbRequest is used to pass requests for TrackedDB
// instances into the worker loop.
type dbRequest struct {
	namespace string
	done      chan struct{}
}

// makeDBRequest creates a new TrackedDB request for the input namespace.
func makeDBRequest(namespace string) dbRequest {
	return dbRequest{
		namespace: namespace,
		done:      make(chan struct{}),
	}
}

// WorkerConfig encapsulates the configuration options for the
// dbaccessor worker.
type WorkerConfig struct {
	NodeManager      NodeManager
	Clock            clock.Clock
	MetricsCollector *Collector

	// Hub is the pub/sub central hub used to receive notifications
	// about API server topology changes.
	Hub         Hub
	Logger      Logger
	NewApp      func(string, ...app.Option) (DBApp, error)
	NewDBWorker func(DBApp, string, ...TrackedDBWorkerOption) (TrackedDB, error)

	// ControllerID uniquely identifies the controller that this
	// worker is running on. It is equivalent to the machine ID.
	ControllerID string
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.NodeManager == nil {
		return errors.NotValidf("missing NodeManager")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if c.MetricsCollector == nil {
		return errors.NotValidf("missing metrics collector")
	}
	if c.Hub == nil {
		return errors.NotValidf("missing Hub")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if c.NewApp == nil {
		return errors.NotValidf("missing NewApp")
	}
	if c.NewDBWorker == nil {
		return errors.NotValidf("missing NewDBWorker")
	}
	return nil
}

type dbWorker struct {
	cfg      WorkerConfig
	catacomb catacomb.Catacomb

	mu       sync.RWMutex
	dbApp    DBApp
	dbRunner *worker.Runner

	// dbReady is used to signal that we can
	// begin processing GetDB requests.
	dbReady chan struct{}

	// dbRequests is used to synchronise GetDB
	// requests into this worker's event loop.
	dbRequests chan dbRequest

	// apiServerChanges is used to handle incoming changes
	// to API server details within the worker loop.
	apiServerChanges chan apiserver.Details
}

func newWorker(cfg WorkerConfig) (*dbWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &dbWorker{
		cfg: cfg,
		dbRunner: worker.NewRunner(worker.RunnerParams{
			Clock: cfg.Clock,
			// If a worker goes down, we've attempted multiple retries and in
			// that case we do want to cause the dbaccessor to go down. This
			// will then bring up a new dqlite app.
			IsFatal:      func(err error) bool { return true },
			RestartDelay: time.Second * 30,
		}),
		dbReady:          make(chan struct{}),
		dbRequests:       make(chan dbRequest),
		apiServerChanges: make(chan apiserver.Details),
	}

	if err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.dbRunner,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *dbWorker) loop() (err error) {
	// The context here should not be tied to the catacomb, as the context will
	// be cancelled when the worker is stopped, and we want to wait for the
	// Dqlite app to shut down gracefully.
	// There is a timeout in shutdownDqlite to ensure that we don't wait
	// forever.
	defer w.shutdownDqlite(context.Background())

	extant, err := w.cfg.NodeManager.IsExistingNode()
	if err != nil {
		return errors.Trace(err)
	}

	// At this time, while Juju is using both Mongo and Dqlite, we piggyback
	// off the peer-grouper, which applies any configured HA space and
	// broadcasts clustering addresses. Once we do away with mongo,
	// that worker will be replaced with a Dqlite-focussed analogue that does
	// largely the same thing, though potentially disseminating changes via a
	// mechanism other than pub/sub.
	unsub, err := w.cfg.Hub.Subscribe(apiserver.DetailsTopic, w.handleAPIServerChangeMsg)
	if err != nil {
		return errors.Annotate(err, "subscribing to API server topology changes")
	}
	defer unsub()

	// If this is an existing node, we start it up immediately.
	// Otherwise, this host is entering a HA cluster, and we need to wait for
	// the peer-grouper to determine and broadcast addresses satisfying the
	// Juju HA space (if configured); request those details.
	// Once received we can continue configuring this node as a member.
	if extant {
		if err := w.startExistingDqliteNode(); err != nil {
			return errors.Trace(err)
		}
	} else {
		if _, err := w.cfg.Hub.Publish(apiserver.DetailsRequestTopic, apiserver.DetailsRequest{
			Requester: "db-accessor",
			LocalOnly: true,
		}); err != nil {
			return errors.Trace(err)
		}
	}

	for {
		select {
		case req := <-w.dbRequests:
			if err := w.openDatabase(req.namespace); err != nil {
				w.cfg.Logger.Errorf("opening database %q: %s", req.namespace, err.Error())
			}
			close(req.done)
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case apiDetails := <-w.apiServerChanges:
			if err := w.processAPIServerChange(apiDetails); err != nil {
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

// GetDB returns a TrackedDB reference for the dqlite-backed
// database that contains the data for the specified namespace.
// TODO (stickupkid): Before handing out any DB for any namespace,
// we should first validate it exists in the controller list.
// This should only be required if it's not the controller DB.
func (w *dbWorker) GetDB(namespace string) (coredatabase.TrackedDB, error) {
	// Ensure Dqlite is initialised.
	select {
	case <-w.dbReady:
	case <-w.catacomb.Dying():
		return nil, w.catacomb.ErrDying()
	}

	// Enqueue the request.
	req := makeDBRequest(namespace)
	select {
	case w.dbRequests <- req:
	case <-w.catacomb.Dying():
		return nil, w.catacomb.ErrDying()
	}

	// Wait for the worker loop to indicate it's done.
	select {
	case <-req.done:
	case <-w.catacomb.Dying():
		return nil, w.catacomb.ErrDying()
	}

	// This will return a not found error if the request was not honoured.
	// The error will be logged - we don't crash this worker for bad calls.
	tracked, err := w.dbRunner.Worker(namespace, w.catacomb.Dying())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return tracked.(coredatabase.TrackedDB), nil
}

// startExistingDqliteNode takes care of starting Dqlite
// when this host has run a node previously.
func (w *dbWorker) startExistingDqliteNode() error {
	w.cfg.Logger.Infof("host is configured as a Dqlite node")

	mgr := w.cfg.NodeManager

	ctx, cancel := w.scopedContext()
	defer cancel()

	asBootstrapped, err := mgr.IsBootstrappedNode(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// If this existing node is not as bootstrapped, then it is part of a
	// cluster. The Dqlite Raft log and configuration in the Dqlite data
	// directory will indicate the cluster members, but we need to ensure
	// TLS for traffic between nodes explicitly.
	var options []app.Option
	if !asBootstrapped {
		withTLS, err := mgr.WithTLSOption()
		if err != nil {
			return errors.Trace(err)
		}
		options = append(options, withTLS)
	}

	return errors.Trace(w.initialiseDqlite(options...))
}

func (w *dbWorker) initialiseDqlite(options ...app.Option) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.dbApp != nil {
		return nil
	}

	mgr := w.cfg.NodeManager

	dataDir, err := mgr.EnsureDataDir()
	if err != nil {
		return errors.Trace(err)
	}

	dqliteOptions := append(options,
		mgr.WithLogFuncOption(),
		mgr.WithTracingOption(),
	)
	if w.dbApp, err = w.cfg.NewApp(dataDir, dqliteOptions...); err != nil {
		return errors.Trace(err)
	}

	ctx, cancel := w.scopedContext()
	defer cancel()

	if err := w.dbApp.Ready(ctx); err != nil {
		return errors.Annotatef(err, "ensuring Dqlite is ready to process changes")
	}

	// Open up the default controller database. Other database namespaces can
	// be opened up in a more lazy fashion.
	if err := w.openDatabase(coredatabase.ControllerNS); err != nil {
		return errors.Annotate(err, "opening initial databases")
	}

	w.cfg.Logger.Infof("initialized Dqlite application (ID: %v)", w.dbApp.ID())

	if c, err := w.dbApp.Client(ctx); err == nil {
		if info, err := c.Cluster(ctx); err == nil {
			w.cfg.Logger.Infof("current cluster: %#v", info)
		}
	}

	close(w.dbReady)
	return nil
}

// openDatabase starts a TrackedDB worker for the database with the input name.
// It is called by initialiseDqlite to open the controller databases,
// and via GetDB to service downstream database requests.
// It is important to note that the start function passed to StartWorker is not
// invoked synchronously.
// Since GetDB blocks until dbReady is closed, and initialiseDqlite waits for
// the node to be ready, we can assume that we will never race with a nil dbApp
// when first starting up.
// Since the only way we can get into this race is during shutdown, it is safe
// to return ErrDying if we detect a nil database.
// This preserves the error that the worker is exiting with, including nil.
func (w *dbWorker) openDatabase(namespace string) error {
	err := w.dbRunner.StartWorker(namespace, func() (worker.Worker, error) {
		w.mu.RLock()
		defer w.mu.RUnlock()
		if w.dbApp == nil {
			return nil, tomb.ErrDying
		}

		return w.cfg.NewDBWorker(w.dbApp, namespace,
			WithClock(w.cfg.Clock),
			WithLogger(w.cfg.Logger),
			WithMetricsCollector(w.cfg.MetricsCollector),
		)
	})
	if errors.Is(err, errors.AlreadyExists) {
		return nil
	}
	return errors.Trace(err)
}

// handleAPIServerChangeMsg is the callback supplied to the pub/sub
// subscription for API server details. It effectively synchronises the
// handling of such messages into the worker's evert loop.
func (w *dbWorker) handleAPIServerChangeMsg(_ string, apiDetails apiserver.Details, err error) {
	if err != nil {
		// This should never happen.
		w.cfg.Logger.Errorf("pub/sub callback error: %v", err)
		return
	}

	select {
	case <-w.catacomb.Dying():
	case w.apiServerChanges <- apiDetails:
	}
}

// processAPIServerChange deals with cluster topology changes.
func (w *dbWorker) processAPIServerChange(apiDetails apiserver.Details) error {
	w.cfg.Logger.Debugf("new API server details: %#v", apiDetails)

	mgr := w.cfg.NodeManager

	extant, err := mgr.IsExistingNode()
	if err != nil {
		return errors.Trace(err)
	}

	ctx, cancel := w.scopedContext()
	defer cancel()

	// If this is an existing node, check if we are bound to the loopback IP
	// and entering HA, which requires a new local-cloud bind address.
	if extant {
		asBootstrapped, err := mgr.IsBootstrappedNode(ctx)
		if err != nil {
			return errors.Trace(err)
		}

		// If this is a node that is already clustered, there is nothing to do.
		if !asBootstrapped {
			w.cfg.Logger.Debugf("node already clustered; no work to do")
			return nil
		}

		// If we are the as-bootstrapped node, and there is only one server,
		// there is no need to change our bind address.
		// Just keep the loopback binding.
		if len(apiDetails.Servers) == 1 {
			return nil
		}

		addr, err := w.bindAddrFromServerDetails(apiDetails)
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				w.cfg.Logger.Infof(err.Error())
				return nil
			}
			return errors.Trace(err)
		}

		if err := w.rebindAddress(ctx, addr); err != nil {
			return errors.Trace(err)
		}

		// Just bounce wholesale after reconfiguring the node.
		// We flush the opened DBs and ensure a clean slate for dependents.
		w.cfg.Logger.Infof("successfully reconfigured Dqlite; restarting worker")
		return dependency.ErrBounce
	}

	// Otherwise this is a node added by enabling HA,
	// and we need to join to an existing cluster.
	return errors.Trace(w.joinNodeToCluster(apiDetails))
}

// rebindAddress stops the current node, reconfigures the cluster so that
// it is a single server bound to the input local-cloud address.
// It should be called only for a cluster constituted by a single node
// bound to the loopback IP address.
func (w *dbWorker) rebindAddress(ctx context.Context, addr string) error {
	w.shutdownDqlite(ctx)

	mgr := w.cfg.NodeManager
	servers, err := mgr.ClusterServers(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// This should be implied by an earlier check of
	// NodeManager.IsBootstrappedNode, but we want to guard very
	// conservatively against breaking established clusters.
	if len(servers) != 1 {
		w.cfg.Logger.Debugf("not a singular server; skipping address rebind")
		return nil
	}

	// We need to preserve the port from the existing address.
	_, port, err := net.SplitHostPort(servers[0].Address)
	if err != nil {
		return errors.Trace(err)
	}
	servers[0].Address = net.JoinHostPort(addr, port)

	w.cfg.Logger.Infof("rebinding Dqlite node to %s", addr)
	if err := mgr.SetClusterServers(ctx, servers); err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(mgr.SetNodeInfo(servers[0]))
}

// joinNodeToCluster uses the input server details to determine a bind address
// for this node, and one or more addresses of other nodes to cluster with.
// It then uses these to initialise Dqlite.
// If either bind or cluster addresses can not be determined,
// we just return nil and keep waiting for further server detail messages.
func (w *dbWorker) joinNodeToCluster(apiDetails apiserver.Details) error {
	// Get our address from the API details.
	localAddr, err := w.bindAddrFromServerDetails(apiDetails)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			w.cfg.Logger.Infof(err.Error())
			return nil
		}
		return errors.Trace(err)
	}

	// Then get addresses for any other of the servers,
	// so we can join the cluster.
	var clusterAddrs []string
	for id, server := range apiDetails.Servers {
		hostPort := server.InternalAddress
		if id != w.cfg.ControllerID && hostPort != "" {
			addr, _, err := net.SplitHostPort(hostPort)
			if err != nil {
				return errors.Annotatef(err, "splitting host/port for %s", hostPort)
			}
			clusterAddrs = append(clusterAddrs, addr)
		}
	}
	if len(clusterAddrs) == 0 {
		w.cfg.Logger.Infof("no addresses available for this Dqlite node to join cluster")
		return nil
	}

	w.cfg.Logger.Infof("joining Dqlite cluster")
	mgr := w.cfg.NodeManager

	withTLS, err := mgr.WithTLSOption()
	if err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(w.initialiseDqlite(
		mgr.WithAddressOption(localAddr), mgr.WithClusterOption(clusterAddrs), withTLS))
}

// bindAddrFromServerDetails returns the internal IP address from the
// input details that corresponds with this controller machine.
func (w *dbWorker) bindAddrFromServerDetails(apiDetails apiserver.Details) (string, error) {
	hostPort := apiDetails.Servers[w.cfg.ControllerID].InternalAddress
	if hostPort == "" {
		return "", errors.NotFoundf("internal address for this Dqlite node to bind to")
	}

	addr, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		return "", errors.Annotatef(err, "splitting host/port for %s", hostPort)
	}

	return addr, nil
}

// shutdownDqlite makes a best-effort attempt to hand off and shut
// down the local Dqlite node.
// We reassign the dbReady channel, which blocks GetDB requests.
// If the worker is not shutting down permanently, Dqlite should be
// reinitialised either directly or by bouncing the agent reasonably
// soon after calling this method.
func (w *dbWorker) shutdownDqlite(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.dbApp == nil {
		return
	}

	// Ensure that we don't block forever and bound the shutdown time for
	// handing over a dqlite node.
	ctx, cancel := context.WithTimeout(ctx, nodeShutdownTimeout)
	defer cancel()

	if err := w.dbApp.Handover(ctx); err != nil {
		w.cfg.Logger.Errorf("handing off Dqlite responsibilities: %v", err)
	}

	if err := w.dbApp.Close(); err != nil {
		w.cfg.Logger.Errorf("closing Dqlite application: %v", err)
	}

	w.dbApp = nil
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *dbWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}
