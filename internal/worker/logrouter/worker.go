// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logrouter

import (
	"context"
	"strconv"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	corelogger "github.com/juju/juju/core/logger"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/logsender"
)

const (
	defaultBackendBufferSize = 1000
	defaultConvergeTimeout   = time.Second * 60
	backendDrainID           = "drain"
)

type BackendType string

const (
	BackendTypeLogSink BackendType = "logsink"
	BackendTypeLoki    BackendType = "loki"
	BackendTypeDrain   BackendType = "drain-only"
)

// Agent describes the agent configuration methods used by the router.
type Agent interface {
	CurrentConfig() agent.Config
}

// Backend is a worker that accepts log records.
type Backend interface {
	worker.Worker
	LogRecords() logsender.LogRecordCh
}

// BackendFunc constructs a backend worker.
type BackendFunc func(BackendType, ConfigSnapshot) (Backend, error)

// Metrics records logrouter events that are exported elsewhere.
type Metrics interface {
	IncConfigApplyErrors()
}

// WorkerConfig contains logrouter worker configuration.
type WorkerConfig struct {
	Agent              Agent
	LogSource          logsender.LogRecordCh
	AgentConfigChanged *voyeur.Value
	Logger             corelogger.Logger
	Clock              clock.Clock
	Metrics            Metrics

	DrainOnly       bool
	ConvergeTimeout time.Duration

	NewBackend BackendFunc
}

// ConfigSnapshot is the local logging destination configuration.
type ConfigSnapshot struct {
	Mode           BackendType
	Endpoint       string
	CACertificate  string
	ControllerUUID string
	ModelUUID      string
	AgentID        string
}

func (s ConfigSnapshot) sameBackend(other ConfigSnapshot) bool {
	if s.Mode == BackendTypeDrain && other.Mode == BackendTypeDrain {
		return true
	}
	return s.Mode == other.Mode &&
		s.Endpoint == other.Endpoint &&
		s.CACertificate == other.CACertificate
}

// Validate checks that the worker configuration is usable.
func (c *WorkerConfig) Validate() error {
	if c.Agent == nil {
		return errors.NotValidf("nil Agent")
	}
	if c.LogSource == nil {
		return errors.NotValidf("nil LogSource")
	}
	if c.AgentConfigChanged == nil {
		return errors.NotValidf("nil AgentConfigChanged")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if c.ConvergeTimeout <= 0 {
		return errors.NotValidf("non-positive ConvergeTimeout")
	}
	if c.NewBackend == nil {
		return errors.NotValidf("nil NewBackend")
	}
	return nil
}

// NewWorker returns a logrouter worker.
func NewWorker(config WorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:  "log-router",
		Clock: config.Clock,
		IsFatal: func(err error) bool {
			return false
		},
		ShouldRestart: internalworker.ShouldRunnerRestart,
		RestartDelay:  time.Second,
		Logger:        internalworker.WrapLogger(config.Logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &logRouter{
		config:        config,
		runner:        runner,
		configChanges: make(chan struct{}),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "log-router",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.runner,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type logRouter struct {
	catacomb catacomb.Catacomb

	runner        *worker.Runner
	config        WorkerConfig
	configChanges chan struct{}
	backendSeq    int

	activeBackendID string
	activeSnapshot  ConfigSnapshot
	activeRecords   logsender.LogRecordCh
	drainRecords    logsender.LogRecordCh
}

// Kill stops the worker.
func (w *logRouter) Kill() {
	w.catacomb.Kill(nil)
}

// Wait blocks until the worker exits.
func (w *logRouter) Wait() error {
	return w.catacomb.Wait()
}

func (w *logRouter) loop() error {
	ctx, cancel := context.WithCancel(w.catacomb.Context(context.Background()))
	defer cancel()

	watcher := newConfigWatcherWorker(
		w.config.AgentConfigChanged,
		w.configChanges,
	)
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	drainSnapshot := ConfigSnapshot{Mode: BackendTypeDrain}
	drainRecords, err := w.startBackend(ctx, backendDrainID, drainSnapshot)
	if err != nil {
		return w.dyingError(ctx, errors.Trace(err))
	}
	w.drainRecords = drainRecords
	w.activeBackendID = backendDrainID
	w.activeSnapshot = drainSnapshot
	w.activeRecords = drainRecords

	next := w.currentSnapshot()
	if !next.sameBackend(w.activeSnapshot) {
		err = w.switchBackend(ctx, next)
		if err != nil {
			return errors.Trace(err)
		}
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case rec, ok := <-w.config.LogSource:
			if !ok {
				return nil
			}
			next := w.currentSnapshot()
			if !next.sameBackend(w.activeSnapshot) {
				err = w.switchBackend(ctx, next)
				if err != nil {
					return errors.Trace(err)
				}
			}
			if err := w.send(ctx, rec); err != nil {
				return errors.Trace(err)
			}

		case <-w.configChanges:
			next := w.currentSnapshot()
			if next.sameBackend(w.activeSnapshot) {
				continue
			}
			err = w.switchBackend(ctx, next)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (w *logRouter) switchBackend(
	ctx context.Context,
	next ConfigSnapshot,
) error {
	if w.activeBackendID != "" && w.activeBackendID != backendDrainID {
		w.stopBackend(w.activeBackendID)
	}

	if next.Mode == BackendTypeDrain {
		w.activeBackendID = backendDrainID
		w.activeSnapshot = next
		w.activeRecords = w.drainRecords
		return nil
	}

	convergeCtx, cancel := context.WithTimeout(ctx, w.config.ConvergeTimeout)
	defer cancel()

	backendID := w.nextBackendID()
	records, err := w.startBackend(convergeCtx, backendID, next)
	if err != nil {
		if ctx.Err() != nil {
			return w.dyingError(ctx, err)
		}

		w.warnConfigApply(ctx, next, err)
		w.activeBackendID = backendDrainID
		w.activeSnapshot = next
		w.activeRecords = w.drainRecords
		return nil
	}

	w.activeBackendID = backendID
	w.activeSnapshot = next
	w.activeRecords = records
	return nil
}

func (w *logRouter) nextBackendID() string {
	w.backendSeq++
	return "backend-" + strconv.Itoa(w.backendSeq)
}

func (w *logRouter) currentSnapshot() ConfigSnapshot {
	cfg := w.config.Agent.CurrentConfig()
	snapshot := ConfigSnapshot{
		Endpoint:       cfg.LokiEndpoint(),
		CACertificate:  cfg.LokiCACert(),
		ControllerUUID: cfg.Controller().Id(),
		ModelUUID:      cfg.Model().Id(),
		AgentID:        cfg.Tag().String(),
	}
	switch {
	case w.config.DrainOnly:
		snapshot.Mode = BackendTypeDrain
	case snapshot.Endpoint != "":
		snapshot.Mode = BackendTypeLoki
	default:
		snapshot.Mode = BackendTypeLogSink
	}
	return snapshot
}

func (w *logRouter) startBackend(ctx context.Context, id string, snapshot ConfigSnapshot) (logsender.LogRecordCh, error) {
	err := w.runner.StartWorker(ctx, id, func(context.Context) (worker.Worker, error) {
		return w.newBackend(snapshot)
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	backend, err := w.runner.Worker(id, ctx.Done())
	if err != nil {
		_ = w.runner.StopWorker(id)
		return nil, errors.Trace(err)
	}

	recordBackend, ok := backend.(Backend)
	if !ok {
		_ = w.runner.StopWorker(id)
		return nil, errors.NotValidf("logrouter backend %q", id)
	}

	return recordBackend.LogRecords(), nil
}

func (w *logRouter) newBackend(snapshot ConfigSnapshot) (Backend, error) {
	return w.config.NewBackend(snapshot.Mode, snapshot)
}

func (w *logRouter) send(
	ctx context.Context,
	record *logsender.LogRecord,
) error {
	if w.activeBackendID == backendDrainID {
		return w.dyingError(ctx, sendRecord(ctx, w.drainRecords, record))
	}
	if w.activeRecords != nil {
		if ok, err := trySendRecord(ctx, w.activeRecords, record); ok || err != nil {
			return w.dyingError(ctx, err)
		}
	}
	return w.dyingError(ctx, sendRecord(ctx, w.drainRecords, record))
}

func (w *logRouter) dyingError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return w.catacomb.ErrDying()
	default:
		return err
	}
}

func (w *logRouter) warnConfigApply(ctx context.Context, snapshot ConfigSnapshot, err error) {
	w.config.Logger.Warningf(
		ctx,
		"failed to apply log router config for %q within %s: %v",
		snapshot.Mode, w.config.ConvergeTimeout, err,
	)
	if w.config.Metrics != nil {
		w.config.Metrics.IncConfigApplyErrors()
	}
}

func trySendRecord(ctx context.Context, records logsender.LogRecordCh, record *logsender.LogRecord) (bool, error) {
	select {
	case records <- record:
		return true, nil
	case <-ctx.Done():
		return false, worker.ErrDead
	default:
		return false, nil
	}
}

func sendRecord(ctx context.Context, records logsender.LogRecordCh, record *logsender.LogRecord) error {
	select {
	case records <- record:
	case <-ctx.Done():
		return worker.ErrDead
	}
	return nil
}

func (w *logRouter) stopBackend(id string) {
	_ = w.runner.StopWorker(id)
}

type configWatcherWorker struct {
	tomb    tomb.Tomb
	watch   *voyeur.Watcher
	changes chan<- struct{}
}

func newConfigWatcherWorker(value *voyeur.Value, changes chan<- struct{}) worker.Worker {
	w := &configWatcherWorker{
		watch:   value.Watch(),
		changes: changes,
	}
	w.tomb.Go(w.loop)
	return w
}

func (w *configWatcherWorker) Kill() {
	w.watch.Close()
	w.tomb.Kill(nil)
}

func (w *configWatcherWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *configWatcherWorker) loop() error {
	defer w.watch.Close()
	for {
		if !w.watch.Next() {
			return nil
		}
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case w.changes <- struct{}{}:
		}
	}
}
