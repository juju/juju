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
	"github.com/juju/juju/overlord/state"
	"github.com/juju/loggo"
	"github.com/juju/version/v2"
)

type State interface {
	PrepareStatement(context.Context, string) (*sql.Stmt, error)
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

// StartUp the LogManager preparing the statements required for appending lines.
func (m *LogManager) StartUp(ctx context.Context) error {
	stmt, err := m.state.PrepareStatement(ctx, sqlInsertLogEntry)
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
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, stmt := range m.statements {
		_ = stmt.Close()
	}
	m.statements = nil
	return nil
}

// Line defines a log line that is normalized to the logstate. It prevents
// writing types from external packages into the database (see charm.Charm as
// an example).
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
func (m *LogManager) AppendLines(txn state.Txn, lines []Line) error {
	stmt, err := m.getStatement(txn, sqlInsertLogEntry)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = stmt.Close() }()

	for _, r := range lines {
		_, err := stmt.ExecContext(context.Background(),
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

func (m *LogManager) getStatement(txn state.Txn, key string) (*sql.Stmt, error) {
	m.mutex.Lock()
	stmt, ok := m.statements[key]
	if !ok {
		m.mutex.Unlock()

		// The following should never happen in production and is classified as
		// a programmatic error that should be picked up in tests.
		return nil, errors.Errorf("missing SQL statement: %s", key)
	}
	m.mutex.Unlock()
	// Return a transaction-specific prepared statement from an existing
	// prepared statement.
	return txn.StmtContext(context.Background(), stmt), nil
}

const (
	sqlInsertLogEntry = "INSERT INTO logs (ts, entity, version, module, location, level, message, labels) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
)
