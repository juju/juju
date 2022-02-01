// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actionstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/juju/overlord/state"
	"github.com/juju/names/v4"
)

type State interface {
	PrepareStatement(context.Context, string) (*sql.Stmt, error)
}

type ActionManager struct {
	state State

	mutex      sync.Mutex
	statements map[string]*sql.Stmt
}

func NewManager(s State) *ActionManager {
	mgr := &ActionManager{
		state:      s,
		statements: make(map[string]*sql.Stmt),
	}
	return mgr
}

// StartUp the ActionManager preparing the statements required for appending lines.
func (m *ActionManager) StartUp(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	statements := []string{
		sqlSelectActionByTag,
		sqlSelectActionLogsByID,
		sqlSelectActionResultByID,
	}
	for _, statement := range statements {
		stmt, err := m.state.PrepareStatement(ctx, statement)
		if err != nil {
			return errors.Trace(err)
		}

		m.statements[sqlSelectActionByTag] = stmt
	}

	return nil
}

func (m *ActionManager) Ensure() error {
	return nil
}

func (m *ActionManager) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, stmt := range m.statements {
		_ = stmt.Close()
	}
	m.statements = nil
	return nil
}

func (m *ActionManager) ActionByTag(txn state.Txn, tag names.ActionTag) (*Action, error) {
	stmt, err := m.getStatement(txn, sqlSelectActionByTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() { _ = stmt.Close() }()

	rows, err := stmt.QueryContext(context.Background(), tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var (
		rawParameters string
		status        string
		action        = new(Action)
	)
	for rows.Next() {
		if err := rows.Scan(&action.ID, &action.Receiver, &action.Name, &rawParameters, &action.Operation, &status, &action.Message, &action.Enqueued, &action.Started, &action.Completed); err != nil {
			return nil, errors.Trace(err)
		}

		if rows.Next() {
			return nil, errors.Errorf("expected only one action for: %v", tag.Id())
		}
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Trace(err)
	}

	// Unpack the action parameters.
	if err := json.Unmarshal([]byte(rawParameters), &action.Parameters); err != nil {
		return nil, errors.Trace(err)
	}
	action.Status = ActionStatus(status)

	// Get the logs and results.
	action.Logs, err = m.getActionLogsByID(txn, tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	action.Results, err = m.getActionResultByID(txn, tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return action, nil
}

func (m *ActionManager) getActionLogsByID(txn state.Txn, id string) ([]ActionMessage, error) {
	stmt, err := m.getStatement(txn, sqlSelectActionLogsByID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() { _ = stmt.Close() }()

	rows, err := txn.QueryContext(context.Background(), id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var messages []ActionMessage
	for rows.Next() {
		var message ActionMessage
		if err := rows.Scan(&message.Message, &message.Timestamp); err != nil {
			return nil, errors.Trace(err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Trace(err)
	}

	return messages, nil
}

func (m *ActionManager) getActionResultByID(txn state.Txn, id string) (map[string]interface{}, error) {
	stmt, err := m.getStatement(txn, sqlSelectActionResultByID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() { _ = stmt.Close() }()

	rows, err := txn.QueryContext(context.Background(), id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var raw string
	var result map[string]interface{}
	for rows.Next() {
		if err := rows.Scan(&raw); err != nil {
			return nil, errors.Trace(err)
		}
		if rows.Next() {
			return nil, errors.Errorf("expected only one action result for: %v", id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Trace(err)
	}

	// Unpack the action result.
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, errors.Trace(err)
	}

	return result, nil
}

func (m *ActionManager) getStatement(txn state.Txn, key string) (*sql.Stmt, error) {
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
	sqlSelectActionByTag = `
SELECT 
	id, 
	receiver,
	name,
	parameters_json,
	operation
	status,
	message,
	enqueued,
	started,
	completed
FROM actions
WHERE id=?
`
	sqlSelectActionLogsByID = `
SELECT
	id,
	output,
	timestamp
FROM actions_logs
WHERE action_id=?
`
	sqlSelectActionResultByID = `
SELECT
	result_json
FROM actions_logs
WHERE action_id=?
`
)
