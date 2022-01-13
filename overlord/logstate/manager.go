// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logstate

import (
	"database/sql"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/version/v2"
)

type State interface {
	DB() *sql.DB
}

type LogManager struct {
	state State
}

func NewManager(s State) *LogManager {
	return &LogManager{
		state: s,
	}
}

func (m *LogManager) Ensure() error {
	// TODO: (stickupkid) - Prepare all the queries, so they become faster when
	// executing.
	return nil
}

type Line struct {
	ID        int64
	Time      time.Time
	ModelUUID string
	Entity    string
	Version   version.Number
	Level     loggo.Level
	Module    string
	Location  string
	Message   string
	Labels    []string
}

func (m *LogManager) AppendLines(lines []Line) error {
	stmt, err := m.state.DB().Prepare(sqlInsertLogEntry)
	if err != nil {
		return errors.Trace(err)
	}

	for _, r := range lines {
		if _, err = stmt.Exec(sqlInsertLogEntry,
			r.Time,
			r.Entity,
			r.Version.String(),
			r.Module,
			r.Location,
			r.Level,
			r.Message,
			strings.Join(r.Labels, ","),
		); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

const (
	sqlInsertLogEntry = "INSERT INTO logs (ts, entity, version, module, location, level, message, labels) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
)
