// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
)

const (
	logLevel  = loggo.INFO
	logModule = "statushistory"
)

// Config defines the attributes used to create a status history worker.
type Config struct {
	ModelLogger logger.ModelLogger
	Clock       clock.Clock
}

// statusHistoryWorker is a worker which provides access to a status history
// for a given model.
type statusHistoryWorker struct {
	tomb        tomb.Tomb
	modelLogger logger.ModelLogger
	clock       clock.Clock
}

// NewWorker returns a new worker which provides access to a status history
// for a given model.
func NewWorker(cfg Config) (worker.Worker, error) {
	w := &statusHistoryWorker{
		modelLogger: cfg.ModelLogger,
		clock:       cfg.Clock,
	}

	w.tomb.Go(w.loop)

	return w, nil
}

// Kill implements Worker.Kill()
func (w *statusHistoryWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements Worker.Wait()
func (w *statusHistoryWorker) Wait() error {
	return w.tomb.Wait()
}

// StatusHistorySetterForModel returns a status history setter for a given
// model.
func (w *statusHistoryWorker) StatusHistorySetterForModel(modelUUID, modelName, modelOwner string) (status.StatusHistorySetter, error) {
	logger, err := w.modelLogger.GetLogger(modelUUID, modelName, modelOwner)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &statusHistorySetter{
		logger:    logger,
		clock:     w.clock,
		modelUUID: modelUUID,
	}, nil
}

func (w *statusHistoryWorker) loop() error {
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	}
}

type statusHistorySetter struct {
	modelUUID string
	logger    logger.Logger
	clock     clock.Clock

	mutex            sync.RWMutex
	fileLocation     string
	fileLocationOnce sync.Once
}

// SetStatusHistory sets a status history for a given entity.
func (s *statusHistorySetter) SetStatusHistory(kind status.HistoryKind, status status.Status, entityID string) error {
	if !kind.Valid() {
		return errors.Errorf("invalid history kind %q", kind)
	}

	return s.logger.Log([]logger.LogRecord{{
		Time:      s.clock.Now(),
		ModelUUID: s.modelUUID,
		Entity:    entityID,
		Level:     logLevel,
		Module:    logModule,
		Location:  s.location(),
		Message:   fmt.Sprintf("status history: %s - %s status", kind, status.String()),
		Labels: map[string]string{
			"domain": "status",
			"kind":   kind.String(),
			"id":     entityID,
			"status": status.String(),
		},
	}})
}

func (s *statusHistorySetter) location() string {
	s.fileLocationOnce.Do(func() {
		s.mutex.Lock()
		defer s.mutex.Unlock()
		s.fileLocation = location()
	})

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.fileLocation
}

func location() string {
	// Get caller frame for the set status history call.
	var pcs [1]uintptr
	n := runtime.Callers(6, pcs[:])
	if n < 1 {
		return "unknown"
	}

	fn := runtime.FuncForPC(pcs[0])
	file, line := fn.FileLine(pcs[0])

	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}
