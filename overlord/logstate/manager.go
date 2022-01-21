// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logstate

import (
	"context"
	"database/sql"
	"strings"
	"sync"
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

	mutex      sync.Mutex
	statements map[string]*sql.Stmt
}

func NewManager(s State) *LogManager {
	mgr := &LogManager{
		state:      s,
		statements: make(map[string]*sql.Stmt),
	}
	return mgr
}

func (m *LogManager) StartUp(ctx context.Context) error {
	db := m.state.DB()
	stmt, err := db.PrepareContext(ctx, sqlInsertLogEntry)
	if err != nil {
		return errors.Trace(err)
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.statements[sqlInsertLogEntry] = stmt

	return nil
}

func (m *LogManager) Ensure() error {
	return nil
}

func (m *LogManager) Stop() error {
	for _, stmt := range m.statements {
		_ = stmt.Close()
	}
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

// AppendLines appends the log lines to the given log manager.
func (m *LogManager) AppendLines(ctx context.Context, lines []Line) error {
	stmt := m.getStatement(sqlInsertLogEntry)
	for _, r := range lines {
		_, err := stmt.ExecContext(ctx,
			r.Time.In(time.UTC).Format("2006-01-02 15:04:05"),
			r.Entity,
			r.Version.String(),
			r.Module,
			r.Location,
			r.Level,
			r.Message,
			strings.Join(r.Labels, ","),
		)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (m *LogManager) getStatement(key string) *sql.Stmt {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	stmt, ok := m.statements[key]

	if !ok {
		panic("missing SQL statement")
	}
	return stmt
}

const (
	sqlInsertLogEntry = "INSERT INTO logs (ts, entity, version, module, location, level, message, labels) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
)
