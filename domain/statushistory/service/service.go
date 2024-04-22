// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/statushistory"
)

var (
	statusHistoryNamespace = []byte("statushistory")
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
	defer file.Close()

	var histories []statushistory.History

	// Read the log file line by line.
	// TODO (stickupkid): If this operation becomes expensive, invert the file
	// and read from the end to the beginning. Most likely this is what the
	// caller will want to do anyway.
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		b := scanner.Bytes()
		if len(b) == 0 {
			continue
		}

		// Sniff the line to determine if it is a status history line.
		if !bytes.Contains(b, statusHistoryNamespace) {
			continue
		}

		var line logLine
		if err := json.Unmarshal(b, &line); err != nil {
			s.logger.Infof("failed to unmarshal status history: %s %v", string(b), err)
			continue
		}

		// We're only interested in status history modules that have labels.
		// If it doesn't match this criteria, skip to the next line.
		if line.Module != string(statusHistoryNamespace) || len(line.Labels) == 0 {
			continue
		}

		var labels logLineLabels
		if err := json.Unmarshal(line.Labels, &labels); err != nil {
			s.logger.Infof("failed to unmarshal status history labels: %v", err)
			continue
		}

		// We're only interested in status history lines that match the
		// specified kind.
		if !matchesKind(kind, labels.Kind) {
			continue
		}

		histories = append(histories, statushistory.History{
			Timestamp: line.Timestamp,
			Kind:      labels.Kind,
			Status:    labels.Status,
			Message:   line.Message,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, errors.Trace(err)
	}

	return histories, nil
}

func matchesKind(requested, observed status.HistoryKind) bool {
	// If the requested kind matches the observed kind, return true.
	if requested == observed {
		return true
	}

	// Unit kinds are special in that you also get additional information about
	// the agent history.
	if requested == status.KindUnit && observed == status.KindUnitAgent {
		return true
	}

	return false
}

// logLine represents a line in the status history log.
type logLine struct {
	Timestamp time.Time       `json:"timestamp"`
	Entity    string          `json:"entity"`
	Level     string          `json:"level"`
	Module    string          `json:"module"`
	Labels    json.RawMessage `json:"labels"`
	Message   string          `json:"message"`
}

type logLineLabels struct {
	Domain string             `json:"domain"`
	Kind   status.HistoryKind `json:"kind"`
	Status status.Status      `json:"status"`
}
