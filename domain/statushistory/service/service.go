// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"bufio"
	"context"
	"encoding/json"
	"os"

	"github.com/juju/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/statushistory"
)

// Logger is the interface for logging.
type Logger interface {
	Infof(string, ...any)
}

// State describes retrieval and persistence methods for the status history.
type State interface {
	// Model returns the read-only model for status history.
	Model(context.Context) (statushistory.ReadOnlyModel, error)
}

// Service provides the API for working with the status history.
type Service struct {
	st     State
	logDir string
	logger Logger
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, logDir string, logger Logger) *Service {
	return &Service{
		st:     st,
		logDir: logDir,
		logger: logger,
	}
}

// GetStatusHistory returns the status history for the specified kind.
func (s *Service) GetStatusHistory(ctx context.Context, kind status.HistoryKind) ([]statushistory.History, error) {
	model, err := s.st.Model(ctx)
	if err != nil {
		return nil, err
	}

	modelPrefix := logger.ModelFilePrefix(model.Owner, model.Name)
	path := logger.ModelLogFile(s.logDir, model.UUID, modelPrefix)

	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var histories []statushistory.History

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		bytes := scanner.Bytes()
		if len(bytes) == 0 {
			continue
		}

		var line logLine
		if err := json.Unmarshal(bytes, &line); err != nil {
			s.logger.Infof("failed to unmarshal status history: %v", err)
			continue
		}

		if line.Kind != kind {
			continue
		}

		histories = append(histories, statushistory.History{
			Kind: line.Kind,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, errors.Trace(err)
	}

	return histories, nil
}

// logLine represents a line in the status history log.
type logLine struct {
	Kind status.HistoryKind `json:"kind"`
}
