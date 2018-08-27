// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manifold

// This needs to be in a different package from the lease manager
// because it uses state (to construct the raftlease store), but the
// lease manager also runs as a worker in state, so the state package
// depends on worker/lease. Having it in worker/lease produces an
// import cycle.

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/utils"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/globalclock"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/lease"
	workerstate "github.com/juju/juju/worker/state"
)

const (
	// MaxSleep is the longest the manager will sleep before checking
	// whether any leases should be expired. If it can see a lease
	// expiring sooner than that it will still wake up earlier.
	MaxSleep = time.Minute

	// ForwardTimeout is how long the store should wait for a response
	// after sending a lease operation over the hub.
	ForwardTimeout = 200 * time.Millisecond

	// maxLogs is the maximum number of backup store log files to keep.
	maxLogs = 10

	// maxLogSizeMB is the maximum size of the store log file on disk
	// in megabytes.
	maxLogSizeMB = 30
)

// TODO(raftlease): This manifold does too much - split out a worker
// that holds the lease store and a manifold that creates it. Then
// make this one depend on that.

// ManifoldConfig holds the resources needed to start the lease
// manager in a dependency engine.
type ManifoldConfig struct {
	AgentName      string
	ClockName      string
	StateName      string
	CentralHubName string

	FSM          *raftlease.FSM
	RequestTopic string
	Logger       lease.Logger
	NewWorker    func(lease.ManagerConfig) (worker.Worker, error)
	NewStore     func(raftlease.StoreConfig) *raftlease.Store
	NewTarget    func(*state.State, io.Writer, lease.Logger) raftlease.NotifyTarget
}

// Validate checks that the config has all the required values.
func (c ManifoldConfig) Validate() error {
	if c.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if c.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if c.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if c.CentralHubName == "" {
		return errors.NotValidf("empty CentralHubName")
	}
	if c.FSM == nil {
		return errors.NotValidf("nil FSM")
	}
	if c.RequestTopic == "" {
		return errors.NotValidf("empty RequestTopic")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if c.NewStore == nil {
		return errors.NotValidf("nil NewStore")
	}
	if c.NewTarget == nil {
		return errors.NotValidf("nil NewTarget")
	}
	return nil
}

type manifoldState struct {
	config ManifoldConfig
	store  *raftlease.Store
}

func (s *manifoldState) start(context dependency.Context) (worker.Worker, error) {
	if err := s.config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := context.Get(s.config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var clock clock.Clock
	if err := context.Get(s.config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	var hub *pubsub.StructuredHub
	if err := context.Get(s.config.CentralHubName, &hub); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(s.config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	st := statePool.SystemState()

	source := rand.NewSource(clock.Now().UnixNano())
	runID := rand.New(source).Int31()

	logPath := filepath.Join(agent.CurrentConfig().LogDir(), "lease.log")
	if err := primeLogFile(logPath); err != nil {
		// This isn't a fatal error, so log and continue if priming
		// fails.
		s.config.Logger.Errorf(
			"unable to prime log file %q (proceeding anyway): %s",
			logPath,
			err.Error(),
		)
	}

	notifyTarget := s.config.NewTarget(st, makeLogger(logPath), s.config.Logger)
	s.store = s.config.NewStore(raftlease.StoreConfig{
		FSM:          s.config.FSM,
		Hub:          hub,
		Target:       notifyTarget,
		RequestTopic: s.config.RequestTopic,
		ResponseTopic: func(requestID uint64) string {
			return fmt.Sprintf("%s.%08x.%d", s.config.RequestTopic, runID, requestID)
		},
		Clock:          clock,
		ForwardTimeout: ForwardTimeout,
	})

	controllerUUID := agent.CurrentConfig().Controller().Id()
	w, err := s.config.NewWorker(lease.ManagerConfig{
		Secretary:  lease.SecretaryFinder(controllerUUID),
		Store:      s.store,
		Clock:      clock,
		Logger:     s.config.Logger,
		MaxSleep:   MaxSleep,
		EntityUUID: controllerUUID,
	})
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { stTracker.Done() }), nil
}

func (s *manifoldState) output(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	manager, ok := in.(*lease.Manager)
	if !ok {
		return errors.Errorf("expected input of type *worker/lease.Manager, got %T", in)
	}
	switch out := out.(type) {
	case *globalclock.Updater:
		*out = s.store
		return nil
	case *corelease.Manager:
		*out = manager
		return nil
	default:
		return errors.Errorf("expected output of type *globalclock.Updater or *core/lease.Manager, got %T", out)
	}
}

// Manifold builds a dependency.Manifold for running a lease manager.
func Manifold(config ManifoldConfig) dependency.Manifold {
	s := manifoldState{config: config}
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ClockName,
			config.StateName,
			config.CentralHubName,
		},
		Start:  s.start,
		Output: s.output,
	}
}

// NewWorker wraps NewManager to return worker.Worker for testability.
func NewWorker(config lease.ManagerConfig) (worker.Worker, error) {
	return lease.NewManager(config)
}

// NewStore is a shim to make a raftlease.Store for testability.
func NewStore(config raftlease.StoreConfig) *raftlease.Store {
	return raftlease.NewStore(config)
}

// NewTarget is a shim to construct a raftlease.NotifyTarget for testability.
func NewTarget(st *state.State, logFile io.Writer, errorLog lease.Logger) raftlease.NotifyTarget {
	return st.LeaseNotifyTarget(logFile, errorLog)
}

func makeLogger(path string) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   path,
		MaxSize:    maxLogSizeMB,
		MaxBackups: maxLogs,
		Compress:   true,
	}
}

// primeLogFile ensures the lease log file is created with the
// correct mode and ownership.
func primeLogFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return errors.Trace(err)
	}
	if err := f.Close(); err != nil {
		return errors.Trace(err)
	}
	err = utils.ChownPath(path, "syslog")
	return errors.Trace(err)
}
