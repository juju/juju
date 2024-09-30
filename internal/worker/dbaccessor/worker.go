// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain/controllernode/service"
	"github.com/juju/juju/domain/controllernode/state"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/database/app"
	"github.com/juju/juju/internal/database/dqlite"
	"github.com/juju/juju/internal/database/pragma"
	"github.com/juju/juju/internal/worker/controlleragentconfig"
)

const (
	// errTryAgain indicates that the worker should try
	// again later to start a DB tracker worker.
	errTryAgain = errors.ConstError("DB node is nil, but worker is not dying; rescheduling TrackedDB start attempt")

	// errNotReady indicates that we successfully created a new Dqlite app,
	// but the Ready call timed out, and we are waiting for broadcast info.
	errNotReady = errors.ConstError("started DB app, but it failed to become ready; waiting for topology updates")
)

// nodeShutdownTimeout is the timeout that we add to the context passed
// handoff/shutdown calls when shutting down the Dqlite node.
const nodeShutdownTimeout = 30 * time.Second

// NodeManager creates Dqlite `App` initialisation arguments and options.
type NodeManager interface {
	// IsExistingNode returns true if this machine of container has run a
	// Dqlite node in the past.
	IsExistingNode() (bool, error)

	// IsLoopbackPreferred returns true if the Dqlite application should
	// be bound to the loopback address.
	IsLoopbackPreferred() bool

	// IsLoopbackBound returns true if we are a cluster of one,
	// and bound to the loopback IP address.
	IsLoopbackBound(context.Context) (bool, error)

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

	// SetClusterToLocalNode reconfigures the Dqlite cluster
	// so that it has the local node as its only member.
	SetClusterToLocalNode(ctx context.Context) error

	// WithLogFuncOption returns a Dqlite application Option that will proxy Dqlite
	// log output via this factory's logger where the level is recognised.
	WithLogFuncOption() app.Option

	// WithTracingOption returns a Dqlite application Option
	// that will enable tracing of Dqlite operations.
	WithTracingOption() app.Option

	// WithAddressOption returns a Dqlite application Option
	// for specifying the local address:port to use.
	WithAddressOption(string) app.Option

	WithAbstractDomainSocketNameOption(string) app.Option

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
	GetDB(namespace string) (database.TxnRunner, error)
}

type opType int

const (
	getOp opType = iota
	delOp
)

// dbRequest is used to pass requests for TrackedDB
// instances into the worker loop.
type dbRequest struct {
	op        opType
	namespace string
	done      chan error
}

// makeDBGetRequest creates a new TrackedDB request for the input namespace.
func makeDBGetRequest(namespace string) dbRequest {
	return dbRequest{
		op:        getOp,
		namespace: namespace,
		done:      make(chan error),
	}
}

// makeDBDelRequest creates a new request for the deletion of a namespace.
func makeDBDelRequest(namespace string) dbRequest {
	return dbRequest{
		op:        delOp,
		namespace: namespace,
		done:      make(chan error),
	}
}

// WorkerConfig encapsulates the configuration options for the
// dbaccessor worker.
type WorkerConfig struct {
	NodeManager      NodeManager
	Clock            clock.Clock
	MetricsCollector *Collector

	Logger      logger.Logger
	NewApp      func(string, ...app.Option) (DBApp, error)
	NewDBWorker NewDBWorkerFunc

	// ControllerID uniquely identifies the controller that this
	// worker is running on. It is equivalent to the machine ID.
	ControllerID string

	// ControllerConfigWatcher is used to get notifications when the controller
	// agent configuration changes on disk. When it changes, we must reload it
	// and assess potential changes to the database cluster.
	ControllerConfigWatcher controlleragentconfig.ConfigWatcher

	// ClusterConfig supplies bind addresses used for Dqlite clustering.
	ClusterConfig ClusterConfig
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
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if c.NewApp == nil {
		return errors.NotValidf("missing NewApp")
	}
	if c.NewDBWorker == nil {
		return errors.NotValidf("missing NewDBWorker")
	}
	if c.ControllerID == "" {
		return errors.NotValidf("missing ControllerID")
	}
	if c.ControllerConfigWatcher == nil {
		return errors.NotValidf("missing ControllerConfigWatcher")
	}
	if c.ClusterConfig == nil {
		return errors.NotValidf("missing ClusterConfig")
	}
	return nil
}

type dbWorker struct {
	cfg      WorkerConfig
	catacomb catacomb.Catacomb

	mu       sync.RWMutex
	dbApps   []DBApp
	dbRunner *worker.Runner

	// dbsReady is used to signal that we can
	// begin processing GetDB requests.
	dbsReady chan struct{}

	// dbRequests is used to synchronise GetDB
	// requests into this worker's event loop.
	dbRequests chan dbRequest
}

// NewWorker creates a new dbaccessor worker.
func NewWorker(cfg WorkerConfig) (*dbWorker, error) {
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
			IsFatal: func(err error) bool {
				// If a database is dead we should not kill the worker of the
				// runner.
				if errors.Is(err, database.ErrDBDead) {
					return false
				}

				// If there is a rebind during starting up a worker the dbApp
				// will be nil. In this case, we'll return ErrTryAgain. In this
				// case we don't want to kill the worker. We'll force the
				// worker to try again.
				return !errors.Is(err, errTryAgain)
			},
			ShouldRestart: func(err error) bool {
				return !errors.Is(err, database.ErrDBDead)
			},
			RestartDelay: time.Second * 10,
			Logger:       cfg.Logger,
		}),
		dbsReady:   make(chan struct{}),
		dbRequests: make(chan dbRequest),
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
	// The context here should not be tied to the catacomb, as such a context
	// would be cancelled when the worker is stopped, and we want to give a
	// chance for the Dqlite app to shut down gracefully.
	// There is a timeout in shutdownDqlite to ensure that we don't block
	// forever.
	// We allow a very short time to check whether we should attempt to hand
	// over to another node.
	// If we can't determine that we *shouldn't* within the time window,
	// we go ahead and make the attempt.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		bs, _ := w.cfg.NodeManager.IsLoopbackBound(ctx)
		w.shutdownDqlite(context.Background(), !bs)
		cancel()
	}()

	extant, err := w.cfg.NodeManager.IsExistingNode()
	if err != nil {
		return errors.Trace(err)
	}

	// If this is an existing node, we start it up immediately.
	// Otherwise, this host is entering a HA cluster, and we need cluster
	// configuration from disk.
	if extant {
		if err := w.startExistingDqliteNode(); err != nil {
			return errors.Trace(err)
		}
	}

	// Always check for actionable config on start-up in case
	// it was written to disk while we couldn't be notified.
	if err := w.handleClusterConfigChange(false); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		// The following ensures that all dbRequests are serialised and
		// processed in order.
		case req := <-w.dbRequests:
			if req.op == getOp {
				// Ensure the namespace exists or is allowed to open a new one
				// before we attempt to open the database.
				if err := w.ensureNamespace(req.namespace); err != nil {
					select {
					case req.done <- errors.Annotatef(err, "ensuring namespace %q", req.namespace):
					case <-w.catacomb.Dying():
						return w.catacomb.ErrDying()
					}
					continue
				}
				if err := w.openDatabase(req.namespace); err != nil {
					select {
					case req.done <- errors.Annotatef(err, "opening database for namespace %q", req.namespace):
					case <-w.catacomb.Dying():
						return w.catacomb.ErrDying()
					}
					continue
				}
			} else if req.op == delOp {
				w.cfg.Logger.Infof("deleting database for namespace %q", req.namespace)
				if err := w.deleteDatabase(req.namespace); err != nil {
					select {
					case req.done <- errors.Annotatef(err, "deleting database for namespace %q", req.namespace):
					case <-w.catacomb.Dying():
						return w.catacomb.ErrDying()
					}
					continue
				}
			} else {
				select {
				case req.done <- errors.Errorf("unknown op %q", req.op):
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				}
				continue
			}

			req.done <- nil

		case <-w.cfg.ControllerConfigWatcher.Changes():
			w.cfg.Logger.Infof("controller configuration changed on disk")
			if err := w.handleClusterConfigChange(true); err != nil {
				return errors.Trace(err)
			}

		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
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

// Report provides information for the engine report.
func (w *dbWorker) Report() map[string]any {
	return map[string]any{}
}

// GetDB returns a transaction runner for the dqlite-backed
// database that contains the data for the specified namespace.
func (w *dbWorker) GetDB(namespace string) (database.TxnRunner, error) {
	// Ensure Dqlite is initialised.
	select {
	case <-w.dbsReady:
	case <-w.catacomb.Dying():
		return nil, database.ErrDBAccessorDying
	}

	// First check if we've already got the db worker already running.
	// The runner is effectively a DB cache.
	if db, err := w.workerFromCache(namespace); err != nil {
		if errors.Is(err, w.catacomb.ErrDying()) {
			return nil, database.ErrDBAccessorDying
		}

		return nil, errors.Trace(err)
	} else if db != nil {
		return db, nil
	}

	// Enqueue the request as it's either starting up and we need to wait longer
	// or it's not running and we need to start it.
	req := makeDBGetRequest(namespace)
	select {
	case w.dbRequests <- req:
	case <-w.catacomb.Dying():
		return nil, database.ErrDBAccessorDying
	}

	// Wait for the worker loop to indicate it's done.
	select {
	case err := <-req.done:
		// If we know we've got an error, just return that error before
		// attempting to ask the dbRunnerWorker.
		if err != nil {
			return nil, errors.Trace(err)
		}
	case <-w.catacomb.Dying():
		return nil, database.ErrDBAccessorDying
	}

	// This will return a not found error if the request was not honoured.
	// The error will be logged - we don't crash this worker for bad calls.
	tracked, err := w.dbRunner.Worker(namespace, w.catacomb.Dying())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return tracked.(database.TxnRunner), nil
}

func (w *dbWorker) workerFromCache(namespace string) (database.TxnRunner, error) {
	// If the worker already exists, return the existing worker early.
	if tracked, err := w.dbRunner.Worker(namespace, w.catacomb.Dying()); err == nil {
		return tracked.(database.TxnRunner), nil
	} else if errors.Is(errors.Cause(err), worker.ErrDead) {
		// Handle the case where the DB runner is dead due to this worker dying.
		select {
		case <-w.catacomb.Dying():
			return nil, w.catacomb.ErrDying()
		default:
			return nil, errors.Trace(err)
		}
	} else if !errors.Is(errors.Cause(err), errors.NotFound) {
		// If it's not a NotFound error, return the underlying error.
		// We should only start a worker if it doesn't exist yet.
		return nil, errors.Trace(err)
	}

	// We didn't find the worker. Let the caller decide what to do.
	return nil, nil
}

// DeleteDB deletes the dqlite-backed database that contains the data for
// the specified namespace.
// There are currently a set of limitations on the namespaces that can be
// deleted:
//   - It's not possible to delete the controller database.
//   - It currently doesn't support the actual deletion of the database
//     just the removal of the worker. Deletion of the database will be
//     handled once it's supported by dqlite.
func (w *dbWorker) DeleteDB(namespace string) error {
	// Ensure Dqlite is initialised.
	select {
	case <-w.dbsReady:
	case <-w.catacomb.Dying():
		return database.ErrDBAccessorDying
	}

	// Enqueue the request.
	req := makeDBDelRequest(namespace)
	select {
	case w.dbRequests <- req:
	case <-w.catacomb.Dying():
		return database.ErrDBAccessorDying
	}

	// Wait for the worker loop to indicate it's done.
	select {
	case err := <-req.done:
		if err != nil {
			return errors.Trace(err)
		}
	case <-w.catacomb.Dying():
		return database.ErrDBAccessorDying
	}
	return nil
}

// startExistingDqliteNode takes care of starting Dqlite
// when this host has run a node previously.
func (w *dbWorker) startExistingDqliteNode() error {
	mgr := w.cfg.NodeManager
	if mgr.IsLoopbackPreferred() {
		w.cfg.Logger.Infof("Dqlite node is configured to bind to the loopback address")

		return errors.Trace(w.initialiseDqlites())
	}

	w.cfg.Logger.Infof("Dqlite node is configured to bind to a cloud-local address")

	ctx, cancel := w.scopedContext()
	defer cancel()

	asBootstrapped, err := mgr.IsLoopbackBound(ctx)
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

	return errors.Trace(w.initialiseDqlites(options...))
}

// initialiseDqlite starts the local Dqlite app node,
// opens and caches the controller database worker,
// then updates the Dqlite info for this node.
func (w *dbWorker) initialiseDqlites(options ...app.Option) error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	if err := w.startDqliteNodes(ctx, options...); err != nil {
		if errors.Is(err, errNotReady) {
			return nil
		}
		return errors.Trace(err)
	}

	// Open up the default controller database.
	// Other database namespaces are opened lazily via GetDB calls.
	// We don't need to apply the database schema here as the
	// controller database is created during bootstrap.
	if err := w.openDatabase(database.ControllerNS); err != nil {
		return errors.Annotate(err, "opening controller database")
	}

	// Once initialised, set the details for the node.
	// This is a no-op if the details are unchanged.
	// We do this before serving any other requests.
	addr := w.dbApps[0].Address()
	// addr, _, err := net.SplitHostPort(w.dbApps[0].Address())
	// if err != nil {
	// 	return errors.Trace(err)
	// }

	if err := w.nodeService().UpdateDqliteNode(ctx, w.cfg.ControllerID, w.dbApps[0].ID(), addr); err != nil {
		return errors.Trace(err)
	}

	// Begin handling external requests.
	close(w.dbsReady)
	return nil
}

func (w *dbWorker) startDqliteNodes(ctx context.Context, options ...app.Option) (err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.dbApps) > 0 {
		return nil
	}

	defer func() {
		if err != nil {
			w.dbApps = nil
		}
	}()

	mgr := w.cfg.NodeManager

	w.dbApps = make([]DBApp, internaldatabase.NumOfNodes+1)

	for i := 0; i < internaldatabase.NumOfNodes+1; i++ {
		dataDir, err := mgr.EnsureDataDir()
		if err != nil {
			return errors.Trace(err)
		}

		// HACK!
		name := "node-controller"
		if i > 0 {
			name = fmt.Sprintf("node-%d", i)
		}
		nodeDataDir := filepath.Join(dataDir, name)
		if err := os.MkdirAll(nodeDataDir, 0700); err != nil {
			return errors.Annotatef(err, "creating directory for Dqlite data")
		}

		dqliteOptions := append(options,
			mgr.WithAbstractDomainSocketNameOption(name),
			mgr.WithLogFuncOption(),
			mgr.WithTracingOption(),
		)
		if w.dbApps[i], err = w.cfg.NewApp(nodeDataDir, dqliteOptions...); err != nil {
			return errors.Trace(err)
		}

		loggo.GetLogger("*****").Criticalf("new app: %v", name)

		ctx, cCancel := context.WithTimeout(ctx, time.Minute)
		defer cCancel()

		if err := w.dbApps[i].Ready(ctx); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				// We don't know whether we were cancelled by tomb or by timeout.
				// Request API server details in case we need to invoke a backstop
				// scenario. If we are shutting down, this won't matter.
				if err := w.dbApps[i].Close(); err != nil {
					return errors.Trace(err)
				}
				w.dbApps[i] = nil
				return errNotReady
			}
			return errors.Annotatef(err, "ensuring Dqlite is ready to process changes")
		}

		loggo.GetLogger("*****").Criticalf("app ready: %v", name)

		w.cfg.Logger.Infof("serving Dqlite application (ID: %v)", w.dbApps[i].ID())

		if c, err := w.dbApps[i].Client(ctx); err == nil {
			if info, err := c.Cluster(ctx); err == nil {
				w.cfg.Logger.Infof("current cluster: %#v", info)
			}
		}

		loggo.GetLogger("*****").Criticalf("new client: %v", name)
	}

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
// Since the only way we can get into this race is during shutdown or a rebind,
// it is safe to return ErrDying if the catacomb is dying when we detect a nil
// database or ErrTryAgain to force the runner to retry starting the worker
// again.
func (w *dbWorker) openDatabase(namespace string) error {
	// Note: Do not be tempted to create the worker outside of the StartWorker
	// function. This will create potential data race if openDatabase is called
	// multiple times for the same namespace.
	err := w.dbRunner.StartWorker(namespace, func() (worker.Worker, error) {
		w.mu.RLock()
		defer w.mu.RUnlock()
		if len(w.dbApps) == 0 {
			// If the dbApp is nil, then we're either shutting down or
			// rebinding the address. In either case, we don't want to
			// start a new worker. We'll return ErrTryAgain to indicate
			// that we should try again in a bit. This will continue until
			// the dbApp is no longer nil.
			select {
			case <-w.catacomb.Dying():
				return nil, w.catacomb.ErrDying()
			default:
				return nil, errTryAgain
			}
		}

		ctx, cancel := w.scopedContext()
		defer cancel()

		app := w.dbApps[0]
		if namespace != database.ControllerNS {
			h, err := hash(namespace)
			if err != nil {
				return nil, errors.Trace(err)
			}

			app = w.dbApps[(h%internaldatabase.NumOfNodes)+1]

			loggo.GetLogger("*****").Criticalf("namespace: %q, hash: %d, index: %d", namespace, h, (h%internaldatabase.NumOfNodes)+1)
		}

		loggo.GetLogger("*****").Criticalf("open namespace %q", namespace)

		return w.cfg.NewDBWorker(ctx,
			app, namespace,
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

func hash(namespace string) (int, error) {
	h := fnv.New32a()
	if _, err := h.Write([]byte(namespace)); err != nil {
		return -1, errors.Annotate(err, "hashing namespace")
	}
	return int(h.Sum32()), nil
}

type killableWorker interface {
	worker.Worker
	KillWithReason(error)
}

func (w *dbWorker) deleteDatabase(namespace string) error {
	// There will be no runner for the database, so we can't rely on the worker
	// to remove and delete the database. We'll have to do that ourselves.
	if namespace == database.ControllerNS {
		return errors.Forbiddenf("cannot delete controller database")
	}

	ctx, cancel := w.scopedContext()
	defer cancel()

	worker, err := w.workerFromCache(namespace)
	if err != nil {
		return errors.Trace(err)
	} else if worker == nil {
		return errors.NotFoundf("worker for namespace %q", namespace)
	}

	killable, ok := worker.(killableWorker)
	if !ok {
		return errors.Errorf("worker for namespace %q is not killable", namespace)
	}

	// Kill the worker and wait for it to die.
	killable.KillWithReason(database.ErrDBDead)
	if err := killable.Wait(); err != nil && !errors.Is(err, database.ErrDBDead) {
		return errors.Annotatef(err, "waiting for worker to die")
	}

	app := w.dbApps[0]
	if namespace != database.ControllerNS {
		h, err := hash(namespace)
		if err != nil {
			return errors.Trace(err)
		}

		app = w.dbApps[(h%internaldatabase.NumOfNodes)+1]
	}

	// Open the database directly as we can't use the worker to do it for us.
	db, err := app.Open(ctx, namespace)
	if err != nil {
		return errors.Annotatef(err, "opening database for deletion")
	}
	defer db.Close()

	// We need to ensure that foreign keys are disabled before we can blanket
	// delete the database.
	if err := pragma.SetPragma(ctx, db, pragma.ForeignKeysPragma, false); err != nil {
		return errors.Annotate(err, "setting foreign keys pragma")
	}

	// Now attempt to delete the database and all of it's contents.
	// This can be replaced with DROP DB once it's supported by dqlite.
	if err := internaldatabase.Retry(ctx, func() error {
		return internaldatabase.StdTxn(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
			return deleteDBContents(ctx, tx, w.cfg.Logger)
		})
	}); err != nil {
		return errors.Annotatef(err, "deleting database contents")
	}

	return nil
}

// handleClusterConfigChange reconciles the cluster configuration on disk with
// the current running state of this node, and takes action as appropriate.
// The input argument determines whether the inability to read the config
// should be considered an error condition.
func (w *dbWorker) handleClusterConfigChange(noConfigIsFatal bool) error {
	return nil
}

// shutdownDqlite shuts down the local Dqlite node, making a best-effort
// attempt at graceful handover when the input boolean is true.
// If the worker is not shutting down permanently, Dqlite should be
// reinitialised either directly or by bouncing the agent reasonably
// soon after calling this method.
func (w *dbWorker) shutdownDqlite(ctx context.Context, handover bool) {
	w.cfg.Logger.Infof("shutting down Dqlite node")

	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.dbApps) == 0 {
		return
	}

	for _, app := range w.dbApps {
		if app == nil {
			continue
		}
		if handover {
			// Set a bound on the time that we allow for hand off.
			ctx, cancel := context.WithTimeout(ctx, nodeShutdownTimeout)
			defer cancel()

			if err := app.Handover(ctx); err != nil {
				w.cfg.Logger.Errorf("handing off Dqlite responsibilities: %v", err)
			}
		} else {
			w.cfg.Logger.Infof("skipping Dqlite handover")
		}

		if err := app.Close(); err != nil {
			w.cfg.Logger.Errorf("closing Dqlite application: %v", err)
		}
	}

	w.dbApps = nil
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *dbWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}

// ensureNamespace ensures that a given namespace is allowed to exist in
// the database. If the namespace is not within the allowed namespaces, it
// will return a not found error. For any other error it will return the
// underlying error. If it is allowed, then it will return nil.
func (w *dbWorker) ensureNamespace(namespace string) error {
	// If the namespace is the controller namespace, we don't need to
	// validate it. It exists by the very nature of the controller.
	if namespace == database.ControllerNS {
		return nil
	}

	// Otherwise, we need to validate that the namespace exists.
	ctx, cancel := w.scopedContext()
	defer cancel()

	known, err := w.nodeService().IsKnownDatabaseNamespace(ctx, namespace)
	if err != nil {
		return errors.Trace(err)
	}
	if !known {
		return database.ErrDBNotFound
	}
	return nil
}

// nodeService uses the worker's capacity as a DBGetter to return a service
// instance for manipulating the controller node topology.
// We can access the runner cache without going into the worker loop as long as
// the Dqlite node is started - the DB worker for the controller is always
// running in this case. Do not place a call to this method where that may
// *not* be the case.
func (w *dbWorker) nodeService() *service.Service {
	return service.NewService(state.NewState(
		database.NewTxnRunnerFactoryForNamespace(w.workerFromCache, database.ControllerNS)))
}
