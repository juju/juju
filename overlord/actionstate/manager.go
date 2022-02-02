// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actionstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

	// Prepare any statements that we intended to use at a later date.
	statements := []string{
		sqlInsertAction,
		sqlSelectActionByTag,
		sqlSelectActionsByName,
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

// ActionByTag returns one action by tag.
// If no action is found, then it returns a NotFound error.
func (m *ActionManager) ActionByTag(txn state.Txn, tag names.ActionTag) (*Action, error) {
	stmt, err := m.getStatement(txn, sqlSelectActionByTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() { _ = stmt.Close() }()

	return m.getAction(txn, stmt, tag.Id())
}

// ActionsByName returns a slice of actions that have the same name.
func (m *ActionManager) ActionsByName(txn state.Txn, name string) ([]*Action, error) {
	stmt, err := m.getStatement(txn, sqlSelectActionsByName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() { _ = stmt.Close() }()

	return m.getActions(txn, stmt, name)
}

// AddAction adds an action, returning the given action.
func (m *ActionManager) AddAction(txn state.Txn, receiver names.Tag, operationID, actionName string, payload map[string]interface{}) (*Action, error) {
	// Marshal the payload first, before attempting to construct any query.
	// TODO (stickupkid): We might consider moving the marshalling outside of
	// the method because of retries.
	payloadData, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.Annotatef(err, "marshalling action payload")
	}

	stmt, err := m.getStatement(txn, sqlInsertAction)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() { _ = stmt.Close() }()

	// TODO (stickupkid): Ensure the operation also exists and we insert the
	// action notifications.
	result, err := stmt.ExecContext(context.Background(), receiver.Id(), actionName, string(payloadData), operationID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	modified, err := result.RowsAffected()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if modified != 1 {
		return nil, errors.Errorf("expected one action to be inserted: %d", modified)
	}

	// Get the ID, so we can return the action.
	id, err := result.LastInsertId()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return m.ActionByTag(txn, names.NewActionTag(fmt.Sprintf("%d", id)))
}

// CancelActionByTag cancels an action by tag and returns the action that
// was canceled.
func (m *ActionManager) CancelActionByTag(txn state.Txn, tag names.ActionTag) (*Action, error) {
	return nil, errors.NotImplementedf("CancelActionByTag")
}

func (m *ActionManager) getAction(txn state.Txn, stmt *sql.Stmt, args ...interface{}) (*Action, error) {
	actions, err := m.getActions(txn, stmt, args...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if num := len(actions); num > 1 {
		return nil, errors.Errorf("expected only one action, located: %d", num)
	} else if num == 0 {
		return nil, errors.NotFoundf("action %v", args)
	}
	return actions[0], nil
}

func (m *ActionManager) getActions(txn state.Txn, stmt *sql.Stmt, args ...interface{}) ([]*Action, error) {
	rows, err := stmt.QueryContext(context.Background(), args...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var actions []*Action
	for rows.Next() {
		var (
			rawParameters string
			status        string
			action        = new(Action)
		)

		if err := rows.Scan(&action.ID, &action.Receiver, &action.Name, &rawParameters, &action.Operation, &status, &action.Message, &action.Enqueued, &action.Started, &action.Completed); err != nil {
			return nil, errors.Trace(err)
		}

		// Unpack the action parameters.
		if err := json.Unmarshal([]byte(rawParameters), &action.Parameters); err != nil {
			return nil, errors.Trace(err)
		}
		action.Status = ActionStatus(status)

		// Get the logs and results.
		action.Logs, err = m.getActionLogsByID(txn, action.ID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		action.Results, err = m.getActionResultByID(txn, action.ID)
		if err != nil {
			return nil, errors.Trace(err)
		}

		actions = append(actions, action)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Trace(err)
	}

	return actions, nil
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
	sqlInsertAction = `
INSERT INTO actions (
	receiver, name, parameters_json, operation,
	enqueued, status
)
VALUES
	(?, ?, ?, ?, DateTime('now'), 'pending')
`
	sqlSelectActionByTag = `
SELECT
	id,
	receiver,
	name,
	parameters_json,
	operation status,
	message,
	enqueued,
	started,
	completed
FROM
	actions
WHERE
	id = ?
  
`
	sqlSelectActionsByName = `
SELECT
	id,
	receiver,
	name,
	parameters_json,
	operation,
	status,
	message,
	enqueued,
	started,
	completed
FROM
	actions
WHERE
	name = ?
`
	sqlSelectActionLogsByID = `
SELECT
	id,
	output,
	timestamp
FROM
	actions_logs
WHERE
	action_id = ?
  
`
	sqlSelectActionResultByID = `
SELECT
	result_json
FROM
	actions_results
WHERE
	action_id = ?
`
)
