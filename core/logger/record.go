// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
)

// LogRecord defines a single Juju log message as returned by
// LogTailer.
type LogRecord struct {
	Time time.Time

	// origin fields
	ModelUUID string
	Entity    string

	// logging-specific fields
	Level    loggo.Level
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

func (r *LogRecord) MarshalJSON() ([]byte, error) {
	jrec := logRecordJSON{
		ModelUUID: r.ModelUUID,
		Time:      r.Time,
		Entity:    r.Entity,
		Level:     r.Level.String(),
		Module:    r.Module,
		Location:  r.Location,
		Message:   r.Message,
		Labels:    r.Labels,
	}
	return json.Marshal(jrec)
}

func (r *LogRecord) UnmarshalJSON(data []byte) error {
	var jrec logRecordJSON
	if err := json.Unmarshal(data, &jrec); err != nil {
		return errors.Trace(err)
	}
	level, ok := loggo.ParseLevel(jrec.Level)
	if !ok {
		return fmt.Errorf("log level %q %w", jrec.Level, errors.NotValid)
	}
	r.Time = jrec.Time
	r.Entity = jrec.Entity
	r.Level = level
	r.Module = jrec.Module
	r.Location = jrec.Location
	r.Message = jrec.Message
	r.Labels = jrec.Labels
	return nil
}
