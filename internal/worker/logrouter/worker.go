// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logrouter

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	corelogger "github.com/juju/juju/core/logger"
	internalerrors "github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/logsender"
)

const (
	defaultBackendBufferSize = 1000
	defaultConvergeTimeout   = time.Second * 60
	defaultRestartDelay      = time.Second * 1
	backendDrainID           = "drain"
)

// BackendType identifies the active log delivery backend.
type BackendType string

const (
	// BackendTypeLogSink forwards records to the controller log sink.
	BackendTypeLogSink BackendType = "logsink"
	// BackendTypeLoki forwards records to a Loki push endpoint.
	BackendTypeLoki BackendType = "loki"
	// BackendTypeDrain discards records locally.
	BackendTypeDrain BackendType = "drain-only"
)

// Agent describes the agent configuration methods used by the router.
type Agent interface {
	// CurrentConfig returns the latest agent configuration snapshot.
	CurrentConfig() agent.Config
}

// Backend is a worker that accepts log records.
type Backend interface {
	worker.Worker
	worker.Reporter
	// LogRecords returns the channel used to submit log records.
	LogRecords() logsender.LogRecordCh
}

// BackendFunc constructs a backend worker.
type BackendFunc func(BackendType, ConfigSnapshot) (Backend, error)

// Metrics records logrouter events that are exported elsewhere.
type Metrics interface {
	// IncConfigApplyErrors records a backend configuration failure.
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
	RestartDelay    time.Duration

	NewBackend BackendFunc

	// RemoveLegacyLogSinkWriter is called when switching to Loki
	// backend mode. It should remove the legacy "logsink" writer
	// from the default loggo context. It must be idempotent.
	RemoveLegacyLogSinkWriter func()

	// AddLegacyLogSinkWriter is called when switching to LogSink
	// backend mode. It should add the legacy "logsink" writer to
	// the default loggo context. It must be idempotent.
	AddLegacyLogSinkWriter func() error
}

// ConfigSnapshot is the local logging destination configuration.
type ConfigSnapshot struct {
	Mode               BackendType
	Endpoint           string
	CACertificate      string
	InsecureSkipVerify *bool
	ControllerUUID     string
	ModelUUID          string
	AgentID            string
	OrgID              string
}

func (s ConfigSnapshot) sameBackend(other ConfigSnapshot) bool {
	if s.Mode == BackendTypeDrain && other.Mode == BackendTypeDrain {
		return true
	}
	if s.InsecureSkipVerify != nil && other.InsecureSkipVerify != nil {
		if *(s.InsecureSkipVerify) != *(other.InsecureSkipVerify) {
			return false
		}
	} else if s.InsecureSkipVerify == nil || other.InsecureSkipVerify == nil {
		if s.InsecureSkipVerify != other.InsecureSkipVerify {
			return false
		}
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
		return nil, internalerrors.Capture(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:  "log-router",
		Clock: config.Clock,
		IsFatal: func(err error) bool {
			return false
		},
		ShouldRestart: internalworker.ShouldRunnerRestart,
		RestartDelay:  config.RestartDelay,
		Logger:        internalworker.WrapLogger(config.Logger),
	})
	if err != nil {
		return nil, internalerrors.Capture(err)
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
		return nil, internalerrors.Capture(err)
	}
	return w, nil
}

type logRouter struct {
	catacomb catacomb.Catacomb

	runner        *worker.Runner
	config        WorkerConfig
	configChanges chan struct{}
	backendSeq    int

	activeMu        sync.RWMutex
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

// Report returns the active log routing backend and reports for all active
// backend workers.
func (w *logRouter) Report(ctx context.Context) map[string]any {
	activeBackendID, activeSnapshot, _ := w.activeBackend()
	report := map[string]any{
		"activeBackend":   string(w.reportBackendType(activeBackendID, activeSnapshot)),
		"activeBackendID": activeBackendID,
		"backends":        w.backendReports(ctx, activeBackendID),
	}
	return report
}

func (w *logRouter) loop() error {
	ctx, cancel := context.WithCancel(w.catacomb.Context(context.Background()))
	defer cancel()

	watcher := newConfigWatcherWorker(
		w.config.AgentConfigChanged,
		w.configChanges,
	)
	if err := w.catacomb.Add(watcher); err != nil {
		return internalerrors.Capture(err)
	}

	drainSnapshot := ConfigSnapshot{Mode: BackendTypeDrain}
	drainRecords, err := w.startBackend(ctx, backendDrainID, drainSnapshot)
	if err != nil {
		return w.dyingError(ctx, internalerrors.Capture(err))
	}
	w.drainRecords = drainRecords
	w.setActiveBackend(backendDrainID, drainSnapshot, drainRecords)

	next := w.currentSnapshot()
	_, activeSnapshot, _ := w.activeBackend()
	if !next.sameBackend(activeSnapshot) {
		err = w.switchBackend(ctx, next)
		if err != nil {
			return internalerrors.Capture(err)
		}
	} else {
		w.manageLegacyLogSinkWriter(ctx, activeSnapshot)
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
			_, activeSnapshot, _ := w.activeBackend()
			if !next.sameBackend(activeSnapshot) {
				err = w.switchBackend(ctx, next)
				if err != nil {
					return internalerrors.Capture(err)
				}
			}
			if err := w.send(ctx, rec); err != nil {
				return internalerrors.Capture(err)
			}

		case <-w.configChanges:
			next := w.currentSnapshot()
			_, activeSnapshot, _ := w.activeBackend()
			if next.sameBackend(activeSnapshot) {
				continue
			}
			err = w.switchBackend(ctx, next)
			if err != nil {
				return internalerrors.Capture(err)
			}
		}
	}
}

func (w *logRouter) switchBackend(
	ctx context.Context,
	next ConfigSnapshot,
) error {
	activeBackendID, _, _ := w.activeBackend()
	if activeBackendID != "" && activeBackendID != backendDrainID {
		w.stopBackend(activeBackendID)
	}

	if next.Mode == BackendTypeDrain {
		w.setActiveBackend(backendDrainID, next, w.drainRecords)
		w.manageLegacyLogSinkWriter(ctx, next)
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
		w.setActiveBackend(backendDrainID, next, w.drainRecords)
		return nil
	}

	w.setActiveBackend(backendID, next, records)
	w.manageLegacyLogSinkWriter(ctx, next)
	return nil
}

func (w *logRouter) nextBackendID() string {
	w.backendSeq++
	return "backend-" + strconv.Itoa(w.backendSeq)
}

func (w *logRouter) currentSnapshot() ConfigSnapshot {
	cfg := w.config.Agent.CurrentConfig()
	snapshot := ConfigSnapshot{
		Endpoint:           cfg.LokiEndpoint(),
		CACertificate:      cfg.LokiCACert(),
		InsecureSkipVerify: cfg.LokiInsecureSkipVerify(),
		ControllerUUID:     cfg.Controller().Id(),
		ModelUUID:          cfg.Model().Id(),
		AgentID:            cfg.Tag().String(),
		OrgID:              cfg.LokiOrgID(),
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
		return nil, internalerrors.Capture(err)
	}

	backend, err := w.runner.Worker(id, ctx.Done())
	if err != nil {
		_ = w.runner.StopWorker(id)
		return nil, internalerrors.Capture(err)
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
	records, err := w.currentBackendRecords(ctx)
	if err != nil {
		return w.dyingError(ctx, internalerrors.Capture(err))
	}
	activeBackendID, _, _ := w.activeBackend()
	if activeBackendID == backendDrainID {
		return w.dyingError(ctx, sendRecord(ctx, records, record))
	}
	if ok, err := trySendRecord(ctx, records, record); ok || err != nil {
		return w.dyingError(ctx, err)
	}
	return w.dyingError(ctx, sendRecord(ctx, w.drainRecords, record))
}

func (w *logRouter) currentBackendRecords(
	ctx context.Context,
) (logsender.LogRecordCh, error) {
	activeBackendID, _, _ := w.activeBackend()
	if activeBackendID == backendDrainID {
		return w.drainRecords, nil
	}

	backend, err := w.runner.Worker(activeBackendID, ctx.Done())
	if err != nil {
		return nil, err
	}
	recordBackend, ok := backend.(Backend)
	if !ok {
		return nil, errors.NotValidf("logrouter backend %q", activeBackendID)
	}

	records := recordBackend.LogRecords()
	w.setActiveRecords(records)
	return records, nil
}

func (w *logRouter) activeBackend() (string, ConfigSnapshot, logsender.LogRecordCh) {
	w.activeMu.RLock()
	defer w.activeMu.RUnlock()
	return w.activeBackendID, w.activeSnapshot, w.activeRecords
}

func (w *logRouter) setActiveBackend(id string, snapshot ConfigSnapshot, records logsender.LogRecordCh) {
	w.activeMu.Lock()
	defer w.activeMu.Unlock()
	w.activeBackendID = id
	w.activeSnapshot = snapshot
	w.activeRecords = records
}

func (w *logRouter) setActiveRecords(records logsender.LogRecordCh) {
	w.activeMu.Lock()
	defer w.activeMu.Unlock()
	w.activeRecords = records
}

func (w *logRouter) backendReports(ctx context.Context, activeBackendID string) map[string]any {
	reports := make(map[string]any)
	for _, id := range w.activeBackendIDs(activeBackendID) {
		backend, err := w.runner.Worker(id, ctx.Done())
		if err != nil {
			reports[id] = map[string]any{"error": err.Error()}
			continue
		}
		reporter, ok := backend.(worker.Reporter)
		if !ok {
			reports[id] = map[string]any{"error": "backend does not implement reporter"}
			continue
		}
		reports[id] = reporter.Report(ctx)
	}
	return reports
}

func (w *logRouter) activeBackendIDs(activeBackendID string) []string {
	if activeBackendID == "" {
		return nil
	}
	if activeBackendID == backendDrainID {
		return []string{backendDrainID}
	}
	return []string{backendDrainID, activeBackendID}
}

func (w *logRouter) reportBackendType(activeBackendID string, snapshot ConfigSnapshot) BackendType {
	if activeBackendID == backendDrainID {
		return BackendTypeDrain
	}
	return snapshot.Mode
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

func (w *logRouter) manageLegacyLogSinkWriter(ctx context.Context, next ConfigSnapshot) {
	switch next.Mode {
	case BackendTypeLoki:
		w.config.RemoveLegacyLogSinkWriter()
	case BackendTypeLogSink, BackendTypeDrain:
		if err := w.config.AddLegacyLogSinkWriter(); err != nil {
			w.config.Logger.Warningf(
				ctx,
				"failed to add legacy logsink writer: %v",
				err,
			)
		}
	}
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
