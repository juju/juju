// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"encoding/json"
	"time"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// LogRecord defines a single Juju log message as returned by
// LogTailer.
type LogRecord struct {
	Time time.Time

	// origin fields
	ModelUUID string
	Entity    string

	// logging-specific fields
	Level    Level
	Module   string
	Location string
	Message  string
	Labels   map[string]string
}

type logRecordJSON struct {
	ModelUUID string            `json:"model-uuid,omitempty"`
	Time      time.Time         `json:"timestamp"`
	Entity    string            `json:"entity"`
	Level     string            `json:"level"`
	Module    string            `json:"module"`
	Location  string            `json:"location"`
	Message   string            `json:"message"`
	Labels    map[string]string `json:"labels,omitempty"`
}

func (r LogRecord) MarshalJSON() ([]byte, error) {
	return json.Marshal(logRecordJSON{
		ModelUUID: r.ModelUUID,
		Time:      r.Time,
		Entity:    r.Entity,
		Level:     r.Level.String(),
		Module:    r.Module,
		Location:  r.Location,
		Message:   r.Message,
		Labels:    r.Labels,
	})
}

func (r *LogRecord) UnmarshalJSON(data []byte) error {
	var rec logRecordJSON
	if err := json.Unmarshal(data, &rec); err != nil {
		return errors.Capture(err)
	}
	level, ok := ParseLevelFromString(rec.Level)
	if !ok {
		return errors.Errorf("log level %q %w", rec.Level, coreerrors.NotValid)
	}
	r.Time = rec.Time
	r.Entity = rec.Entity
	r.Level = level
	r.Module = rec.Module
	r.Location = rec.Location
	r.Message = rec.Message
	r.Labels = rec.Labels
	r.ModelUUID = rec.ModelUUID
	return nil
}
