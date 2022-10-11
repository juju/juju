// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/core/raft/queue"
	"github.com/juju/juju/worker/raft/raftutil"
)

const (
	// defaultSnapshotRetention is the number of
	// snapshots to retain on disk by default.
	defaultSnapshotRetention = 2

	// bootstrapAddress is the raft server address
	// configured for the bootstrap node. This address
	// will be replaced once the raftclusterer worker
	// observes an address for the server.
	bootstrapAddress raft.ServerAddress = "localhost"

	// LoopTimeout is the max time we will wait until the raft object
	// is constructed and the main loop is started. This is to avoid
	// hard-to-debug problems where the transport hung and so this
	// worker wasn't really started even though it seemed like it
	// was. If it crashes instead the logging will give a path to the
	// problem.
	LoopTimeout = 1 * time.Minute

	// noLeaderTimeout is how long a follower will wait for contact
	// from the leader before restarting. This allows us to see config
	// changes (force-appended by the raft-backstop worker) to allow
	// us to become voting again if the leader was removed leaving a
	// 2-node cluster without quorum.
	noLeaderTimeout = 1 * time.Minute

	// noLeaderFrequency is how long the raft worker wait between
	// checking whether it's in contact with the leader.
	noLeaderFrequency = 10 * time.Second
)

var (
	// ErrWorkerStopped is returned by Worker.Raft if the
	// worker has been explicitly stopped.
	ErrWorkerStopped = errors.New("raft worker stopped")

	// ErrStartTimeout is returned by NewWorker if the worker loop
	// didn't start within LoopTimeout.
	ErrStartTimeout = errors.New("timed out waiting for worker loop")

	// ErrNoLeaderTimeout is returned by the worker loop if we've gone
	// too long without contact from the leader. It gives the worker a
	// chance to see any configuration changes the backstop worker
	// might have force-appended to the raft log.
	ErrNoLeaderTimeout = errors.New("timed out waiting for leader contact")
)

// Logger represents the logging methods called.
type Logger interface {
	Criticalf(message string, args ...interface{})
	Warningf(message string, args ...interface{})
	Errorf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Debugf(message string, args ...interface{})
	Tracef(message string, args ...interface{})
	Logf(level loggo.Level, message string, args ...interface{})
	IsTraceEnabled() bool
}

// Queue is a blocking queue to guard access and to serialize raft applications,
// allowing for client side backoff.
type Queue interface {
	// Queue returns the queue of operations. Removing an item from the channel
	// will unblock to allow another to take its place.
	Queue() <-chan []queue.OutOperation
}

// LeaseApplier applies operations from the queue onto the underlying raft
// instance.
type LeaseApplier interface {
	// ApplyOperation applies a lease opeartion against the raft instance. If
	// the raft instance isn't the leader, then an error is returned with the
	// leader information if available.
	// This Raft spec outlines this "The first option, which we recommend ...,
	// is for the server to reject the request and return to the client the
	// address of the leader, if known." (see 6.2.1).
	// If the leader is the current raft instance, then attempt to apply it to
	// the fsm.
	// The time duration is the applying of a command in an operation, not for
	// the whole operation.
	ApplyOperation([]queue.OutOperation, time.Duration)
}

// Config is the configuration required for running a raft worker.
type Config struct {
	// FSM is the raft.FSM to use for this raft worker. This
	// must be non-nil for NewWorker, and nil for Bootstrap.
	FSM raft.FSM

	// Logger is the logger for this worker.
	Logger Logger

	// StorageDir is the directory in which to store raft
	// artifacts: logs, snapshots, etc. It is expected that
	// this directory is under the full control of the raft
	// worker.
	StorageDir string

	// NonSyncedWritesToRaftLog allows the operator to disable fsync calls
	// after each write to the raft log. This option trades performance for
	// data safety and should be used with caution.
	NonSyncedWritesToRaftLog bool

	// LocalID is the raft.ServerID of this worker.
	LocalID raft.ServerID

	// Transport is the raft.Transport to use for communication
	// between raft servers. This must be non-nil for NewWorker,
	// and nil for Bootstrap.
	//
	// The raft worker expects the server address to exactly
	// match the server ID, which is the stringified agent tag.
	// The transport internally maps the server address to one
	// or more network addresses, i.e. by looking up the API
	// connection information in the state database.
	Transport raft.Transport

	// Clock is used for timeouts in the worker (although not inside
	// raft).
	Clock clock.Clock

	// NoLeaderTimeout, if non-zero, will override the default
	// timeout for leader contact before restarting.
	NoLeaderTimeout time.Duration

	// ElectionTimeout, if non-zero, will override the default
	// raft election timeout.
	ElectionTimeout time.Duration

	// HeartbeatTimeout, if non-zero, will override the default
	// raft heartbeat timeout.
	HeartbeatTimeout time.Duration

	// LeaderLeaseTimeout, if non-zero, will override the default
	// raft leader lease timeout.
	LeaderLeaseTimeout time.Duration

	// SnapshotRetention is the non-negative number of snapshots
	// to retain on disk. If zero, defaults to 2.
	SnapshotRetention int

	// PrometheusRegisterer is used to register the raft metrics.
	PrometheusRegisterer prometheus.Registerer

	// Queue is a blocking queue to apply raft operations.
	Queue Queue

	// NewApplier is used to apply the raft operations on to the raft
	// instance, before notifying a target of the changes.
	NewApplier func(Raft, ApplierMetrics, clock.Clock, Logger) LeaseApplier
}

// Validate validates the raft worker configuration.
func (config Config) Validate() error {
	if config.FSM == nil {
		return errors.NotValidf("nil FSM")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.StorageDir == "" {
		return errors.NotValidf("empty StorageDir")
	}
	if config.LocalID == "" {
		return errors.NotValidf("empty LocalID")
	}
	if config.SnapshotRetention < 0 {
		return errors.NotValidf("negative SnapshotRetention")
	}
	if config.Transport == nil {
		return errors.NotValidf("nil Transport")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Queue == nil {
		return errors.NotValidf("nil Queue")
	}
	if config.NewApplier == nil {
		return errors.NotValidf("nil NewApplier")
	}
	return nil
}

// Bootstrap bootstraps the raft cluster, using the given configuration.
//
// This is only to be called once, at the beginning of the raft cluster's
// lifetime, by the bootstrap machine agent.
func Bootstrap(config Config) error {
	if config.FSM != nil {
		return errors.NotValidf("non-nil FSM during Bootstrap")
	}
	if config.Transport != nil {
		return errors.NotValidf("non-nil Transport during Bootstrap")
	}
	if config.NewApplier != nil {
		return errors.NotValidf("non-nil NewApplier during Bootstrap")
	}

	// During bootstrap we use an in-memory transport. We just need
	// to make sure we use the same local address as we'll use later.
	_, transport := raft.NewInmemTransport(bootstrapAddress)
	defer func() { _ = transport.Close() }()
	config.Transport = transport

	// During bootstrap, we do not require an FSM.
	config.FSM = BootstrapFSM{}
	config.NewApplier = func(Raft, ApplierMetrics, clock.Clock, Logger) LeaseApplier {
		return BootstrapLeaseApplier{}
	}

	w, err := newWorker(config)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = worker.Stop(w) }()

	r, err := w.Raft()
	if err != nil {
		return errors.Trace(err)
	}

	if err := r.BootstrapCluster(raft.Configuration{
		Servers: []raft.Server{{
			ID:      config.LocalID,
			Address: bootstrapAddress,
		}},
	}).Error(); err != nil {
		return errors.Annotate(err, "bootstrapping raft cluster")
	}
	return errors.Annotate(worker.Stop(w), "stopping bootstrap raft worker")
}

// NewWorker returns a new raft worker, with the given configuration.
func NewWorker(config Config) (worker.Worker, error) {
	return newWorker(config)
}

func newWorker(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	if config.NoLeaderTimeout == 0 {
		config.NoLeaderTimeout = noLeaderTimeout
	}
	raftConfig, err := NewRaftConfig(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w := &Worker{
		config:     config,
		raftCh:     make(chan *raft.Raft),
		logStoreCh: make(chan raft.LogStore),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: func() error {
			return w.loop(raftConfig)
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}
	// Wait for the loop to be started.
	select {
	case <-config.Clock.After(LoopTimeout):
		w.catacomb.Kill(ErrStartTimeout)
		return nil, ErrStartTimeout
	case <-w.raftCh:
	}
	return w, nil
}

// Worker is a worker that manages a raft.Raft instance.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config

	raftCh     chan *raft.Raft
	logStoreCh chan raft.LogStore
}

// Raft returns the raft.Raft managed by this worker, or
// an error if the worker has stopped.
func (w *Worker) Raft() (*raft.Raft, error) {
	select {
	case <-w.catacomb.Dying():
		err := w.catacomb.Err()
		if err != nil {
			return nil, err
		}
		return nil, ErrWorkerStopped
	case r := <-w.raftCh:
		return r, nil
	}
}

// LogStore returns the raft.LogStore managed by this worker, or
// an error if the worker has stopped.
func (w *Worker) LogStore() (raft.LogStore, error) {
	select {
	case <-w.catacomb.Dying():
		err := w.catacomb.Err()
		if err != nil {
			return nil, err
		}
		return nil, ErrWorkerStopped
	case logStore := <-w.logStoreCh:
		return logStore, nil
	}
}

// Kill is part of the worker.Worker interface.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop(raftConfig *raft.Config) (loopErr error) {
	// Register the metrics.
	if registry := w.config.PrometheusRegisterer; registry != nil {
		registerRaftMetrics(registry, w.config.Logger)
	}

	syncMode := SyncAfterWrite
	if w.config.NonSyncedWritesToRaftLog {
		syncMode = NoSyncAfterWrite
		w.config.Logger.Warningf(`disabling fsync calls between raft log writes as instructed by the "non-synced-writes-to-raft-log option"`)
	}

	rawLogStore, err := NewLogStore(w.config.StorageDir, syncMode)
	if err != nil {
		// Required logging, as ErrStartTimeout can mask the underlying error
		// and we never know the original failure.
		w.config.Logger.Errorf("Failed to setup raw log store, err: %v", err)
		return errors.Trace(err)
	}

	// We need to make sure access to the LogStore methods (+ closing)
	// is synchronised, but we don't need to synchronise the
	// StableStore methods, because we aren't giving out a reference
	// to the StableStore - only the raft instance uses it.
	logStore := &syncLogStore{store: rawLogStore}
	defer func() { _ = logStore.Close() }()

	snapshotRetention := w.config.SnapshotRetention
	if snapshotRetention == 0 {
		snapshotRetention = defaultSnapshotRetention
	}
	snapshotStore, err := NewSnapshotStore(w.config.StorageDir, snapshotRetention, w.config.Logger)
	if err != nil {
		// Required logging, as ErrStartTimeout can mask the underlying error
		// and we never know the original failure.
		w.config.Logger.Errorf("Failed to setup snapshot store, err: %v", err)
		return errors.Trace(err)
	}

	r, err := raft.NewRaft(raftConfig, w.config.FSM, logStore, rawLogStore, snapshotStore, w.config.Transport)
	if err != nil {
		// Required logging, as ErrStartTimeout can mask the underlying error
		// and we never know the original failure.
		w.config.Logger.Errorf("Failed to setup raft instance, err: %v", err)
		return errors.Trace(err)
	}
	defer func() {
		if err := r.Shutdown().Error(); err != nil {
			if loopErr == nil {
				loopErr = err
			} else {
				w.config.Logger.Warningf("raft shutdown failed: %s", err)
			}
		}
	}()

	applierMetrics := newApplierMetrics(w.config.Clock)
	applier := w.config.NewApplier(r, applierMetrics, w.config.Clock, w.config.Logger)
	if registry := w.config.PrometheusRegisterer; registry != nil {
		registerApplierMetrics(registry, applierMetrics, w.config.Logger)
	}

	// applyTimeout represents the amount of time we allow for raft to apply
	// a command. As raft is bootstrapping, we allow an initial timeout and once
	// applied, we reduce the timeout. 5 seconds is a lifetime for every apply.
	applyTimeout := InitialApplyTimeout

	shutdown := make(chan raft.Observation)
	observer := raft.NewObserver(shutdown, true, func(o *raft.Observation) bool {
		return o.Data == raft.Shutdown
	})
	r.RegisterObserver(observer)
	defer r.DeregisterObserver(observer)

	// Every 10 seconds we check whether the no-leader timeout should
	// trip.
	noLeaderCheck := w.config.Clock.After(noLeaderFrequency)
	lastContact := w.config.Clock.Now()
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-shutdown:
			// The raft server shutdown without this worker
			// telling it to do so. This typically means that
			// the local node was removed from the cluster
			// configuration, causing it to shut down.
			return errors.New("raft shutdown")

		case now := <-noLeaderCheck:
			noLeaderCheck = w.config.Clock.After(noLeaderFrequency)
			if r.State() == raft.Leader {
				w.config.Logger.Tracef("raft leadership check passed with me as the leader")
				lastContact = now
				continue
			}
			var zeroTime time.Time
			if latest := r.LastContact(); latest != zeroTime {
				w.config.Logger.Tracef("last contact with leader at %s (%s)", latest, now.Sub(latest))
				lastContact = latest
			}
			if now.After(lastContact.Add(w.config.NoLeaderTimeout)) {
				w.config.Logger.Tracef("lastContact: %s timeout: %s diff: %s",
					lastContact, w.config.NoLeaderTimeout, now.Sub(lastContact))
				w.config.Logger.Errorf("last leader contact %s which is greater than timeout %s",
					humanize.Time(lastContact), w.config.NoLeaderTimeout)
				return ErrNoLeaderTimeout
			}

		case ops := <-w.config.Queue.Queue():
			if w.config.Logger.IsTraceEnabled() {
				w.config.Logger.Tracef("Dequeued %d commands to be applied on the raft log", len(ops))
			}
			// Apply any operation on to the current raft implementation.
			// This ensures that we serialize the applying of operations onto
			// the raft state.
			applier.ApplyOperation(ops, applyTimeout)
			applyTimeout = ApplyTimeout

		case w.raftCh <- r:
		case w.logStoreCh <- logStore:
		}
	}
}

// NewRaftConfig makes a raft config struct from the worker config
// struct passed in.
func NewRaftConfig(config Config) (*raft.Config, error) {
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = config.LocalID
	// Having ShutdownOnRemove true means that the raft node also
	// stops when it's demoted if it's the leader.
	raftConfig.ShutdownOnRemove = false
	raftConfig.Logger = raftutil.NewHCLLogger("raft", config.Logger)

	maybeOverrideDuration := func(d time.Duration, target *time.Duration) {
		if d != 0 {
			*target = d
		}
	}
	maybeOverrideDuration(config.ElectionTimeout, &raftConfig.ElectionTimeout)
	maybeOverrideDuration(config.HeartbeatTimeout, &raftConfig.HeartbeatTimeout)
	maybeOverrideDuration(config.LeaderLeaseTimeout, &raftConfig.LeaderLeaseTimeout)

	if err := raft.ValidateConfig(raftConfig); err != nil {
		return nil, errors.Annotate(err, "validating raft config")
	}
	return raftConfig, nil
}

// SyncMode defines the supported sync modes
// when writing to the raft log store.
type SyncMode bool

const (
	// SyncAfterWrite ensures that an fsync call is performed after each write.
	SyncAfterWrite SyncMode = false

	// NoSyncAfterWrite ensures that no fsync
	// calls are performed between writes.
	NoSyncAfterWrite SyncMode = true
)

// NewLogStore opens a boltDB logstore in the specified directory. If the
// directory doesn't already exist it'll be created. If the caller passes
// NonSyncedAfterWrite as the value of the syncMode argument, the underlying
// store will NOT perform fsync calls between log writes.
func NewLogStore(dir string, syncMode SyncMode) (*raftboltdb.BoltStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.Trace(err)
	}
	logs, err := raftboltdb.New(raftboltdb.Options{
		Path:   filepath.Join(dir, "logs"),
		NoSync: syncMode == NoSyncAfterWrite,
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to create bolt store for raft logs")
	}
	return logs, nil
}

// NewSnapshotStore opens a file-based snapshot store in the specified
// directory. If the directory doesn't exist it'll be created.
func NewSnapshotStore(
	dir string,
	retain int,
	logger Logger,
) (raft.SnapshotStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.Trace(err)
	}
	logLogger := raftutil.NewHCLLogger("snapshot", logger)

	snaps, err := raft.NewFileSnapshotStoreWithLogger(dir, retain, logLogger)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create file snapshot store")
	}
	return snaps, nil
}

// BootstrapFSM is a minimal implementation of raft.FSM for use during
// bootstrap. Its methods should never be invoked.
type BootstrapFSM struct{}

// Apply is part of raft.FSM.
func (BootstrapFSM) Apply(_ *raft.Log) interface{} {
	panic("Apply should not be called during bootstrap")
}

// Snapshot is part of raft.FSM.
func (BootstrapFSM) Snapshot() (raft.FSMSnapshot, error) {
	panic("Snapshot should not be called during bootstrap")
}

// Restore is part of raft.FSM.
func (BootstrapFSM) Restore(io.ReadCloser) error {
	panic("Restore should not be called during bootstrap")
}

type BootstrapLeaseApplier struct{}

func (BootstrapLeaseApplier) ApplyOperation([]queue.OutOperation, time.Duration) {
	panic("ApplyOperation should not be called during bootstrap")
}
