// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	"github.com/hashicorp/raft-boltdb"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/worker/catacomb"
	"github.com/juju/juju/worker/raft/raftutil"
)

const (
	// defaultSnapshotRetention is the number of
	// snapshots to retain on disk by default.
	defaultSnapshotRetention = 2
)

var (
	// ErrWorkerStopped is returned by Worker.Raft if the
	// worker has been explicitly stopped.
	ErrWorkerStopped = errors.New("raft worker stopped")
)

// Config is the configuration required for running a raft worker.
type Config struct {
	// FSM is the raft.FSM to use for this raft worker. This
	// must be non-nil for NewWorker, and nil for Bootstrap.
	FSM raft.FSM

	// Logger is the logger for this worker.
	Logger loggo.Logger

	// StorageDir is the directory in which to store raft
	// artifacts: logs, snapshots, etc. It is expected that
	// this directory is under the full control of the raft
	// worker.
	StorageDir string

	// Tag is the tag of the agent running this worker.
	Tag names.Tag

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
}

// Validate validates the raft worker configuration.
func (config Config) Validate() error {
	if config.FSM == nil {
		return errors.NotValidf("nil FSM")
	}
	if config.StorageDir == "" {
		return errors.NotValidf("empty StorageDir")
	}
	if config.Tag == nil {
		return errors.NotValidf("nil Tag")
	}
	if config.SnapshotRetention < 0 {
		return errors.NotValidf("negative SnapshotRetention")
	}
	if config.Transport == nil {
		return errors.NotValidf("nil Transport")
	}
	expectedLocalAddr := raft.ServerAddress(config.Tag.String())
	transportLocalAddr := config.Transport.LocalAddr()
	if transportLocalAddr != expectedLocalAddr {
		return errors.NewNotValid(nil, fmt.Sprintf(
			"transport local address %q not valid, expected %q",
			transportLocalAddr, expectedLocalAddr,
		))
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

	// During bootstrap we use an in-memory transport. We just need
	// to make sure we use the same local address as we'll use later.
	localID := raft.ServerID(config.Tag.String())
	localAddr := raft.ServerAddress(localID)
	_, transport := raft.NewInmemTransport(localAddr)
	defer transport.Close()
	config.Transport = transport

	// During bootstrap, we do not require an FSM.
	config.FSM = bootstrapFSM{}

	w, err := NewWorker(config)
	if err != nil {
		return errors.Trace(err)
	}
	defer worker.Stop(w)

	r, err := w.Raft()
	if err != nil {
		return errors.Trace(err)
	}

	if err := r.BootstrapCluster(raft.Configuration{
		Servers: []raft.Server{{
			ID:      localID,
			Address: localAddr,
		}},
	}).Error(); err != nil {
		return errors.Annotate(err, "bootstrapping raft cluster")
	}
	return errors.Annotate(worker.Stop(w), "stopping bootstrap raft worker")
}

// NewWorker returns a new raft worker, with the given configuration.
func NewWorker(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	raftConfig, err := newRaftConfig(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w := &Worker{
		config: config,
		raftCh: make(chan *raft.Raft),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: func() error {
			return w.loop(raftConfig)
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Worker is a worker that manages a raft.Raft instance.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config

	raftCh chan *raft.Raft
}

// Raft returns the raft.Raft managed by this worker, or
// an error if the worker has stopped.
func (w *Worker) Raft() (*raft.Raft, error) {
	// Give precedence to the worker dying.
	select {
	case <-w.catacomb.Dying():
	default:
		select {
		case raft := <-w.raftCh:
			return raft, nil
		case <-w.catacomb.Dying():
		}
	}
	err := w.catacomb.Err()
	if err != nil && err != w.catacomb.ErrDying() {
		return nil, err
	}
	return nil, ErrWorkerStopped
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
	if err := os.MkdirAll(w.config.StorageDir, 0700); err != nil {
		return errors.Trace(err)
	}
	logStore, err := newLogStore(w.config.StorageDir)
	if err != nil {
		return errors.Trace(err)
	}
	defer logStore.Close()

	snapshotRetention := w.config.SnapshotRetention
	if snapshotRetention == 0 {
		snapshotRetention = defaultSnapshotRetention
	}
	snapshotStore, err := newSnapshotStore(w.config.StorageDir, snapshotRetention, w.config.Logger)
	if err != nil {
		return errors.Trace(err)
	}

	r, err := raft.NewRaft(raftConfig, w.config.FSM, logStore, logStore, snapshotStore, w.config.Transport)
	if err != nil {
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

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case w.raftCh <- r:
		}
	}
}

func newRaftConfig(config Config) (*raft.Config, error) {
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(config.Tag.String())

	logWriter := &raftutil.LoggoWriter{config.Logger, loggo.DEBUG}
	raftConfig.Logger = log.New(logWriter, "", 0)

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

func newLogStore(dir string) (*raftboltdb.BoltStore, error) {
	logs, err := raftboltdb.New(raftboltdb.Options{
		Path: filepath.Join(dir, "logs"),
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to create bolt store for raft logs")
	}
	return logs, nil
}

func newSnapshotStore(
	dir string,
	retain int,
	logger loggo.Logger,
) (raft.SnapshotStore, error) {
	const logPrefix = "[snapshot] "
	logWriter := &raftutil.LoggoWriter{logger, loggo.DEBUG}
	logLogger := log.New(logWriter, logPrefix, 0)

	snaps, err := raft.NewFileSnapshotStoreWithLogger(dir, retain, logLogger)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create file snapshot store")
	}
	return snaps, nil
}

// bootstrapFSM is a minimal implementation of raft.FSM for use during
// bootstrap. Its methods should never be invoked.
type bootstrapFSM struct{}

func (bootstrapFSM) Apply(log *raft.Log) interface{} {
	panic("Apply should not be called during bootstrap")
}

func (bootstrapFSM) Snapshot() (raft.FSMSnapshot, error) {
	panic("Snapshot should not be called during bootstrap")
}

func (bootstrapFSM) Restore(io.ReadCloser) error {
	panic("Restore should not be called during bootstrap")
}
