// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/unitless"
	unitlessinternal "github.com/juju/juju/domain/unitless/internal"
	"github.com/juju/juju/internal/errors"
)

type applicationUUID struct {
	UUID string `db:"uuid"`
}

type applicationUUIDs []string

type scriptletSource struct {
	ApplicationName string `db:"application_name"`
	Life            int    `db:"life_id"`
	Path            string `db:"path"`
	Content         []byte `db:"content"`
}

// State provides persistence operations for unitless applications.
type State struct {
	*domain.StateBase
}

// NewState returns a new unitless state using the supplied transaction runner
// factory.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetScriptletApplication returns the scriptlet application associated with an
// application.
func (st *State) GetScriptletApplication(
	ctx context.Context,
	applicationID string,
) (unitlessinternal.ScriptletApplication, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return unitlessinternal.ScriptletApplication{}, errors.Capture(err)
	}

	ident := applicationUUID{UUID: applicationID}
	stmt, err := st.Prepare(`
SELECT cs.path AS &scriptletSource.path,
       cs.content AS &scriptletSource.content,
       a.name AS &scriptletSource.application_name,
       a.life_id AS &scriptletSource.life_id
FROM application AS a
JOIN charm_scriptlet AS cs ON cs.charm_uuid = a.charm_uuid
WHERE a.uuid = $applicationUUID.uuid
ORDER BY cs.path;
`, scriptletSource{}, ident)
	if err != nil {
		return unitlessinternal.ScriptletApplication{}, errors.Errorf("preparing scriptlet application query: %w", err)
	}

	var rows []scriptletSource
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident).GetAll(&rows)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return unitlessinternal.ScriptletApplication{}, errors.Errorf("getting scriptlet application: %w", err)
	}
	if len(rows) == 0 {
		return unitlessinternal.ScriptletApplication{}, nil
	}

	return unitlessinternal.ScriptletApplication{
		UUID: applicationID,
		Name: rows[0].ApplicationName,
		Life: rows[0].Life,
		Sources: transform.Slice(rows, func(row scriptletSource) unitlessinternal.ScriptSource {
			return unitlessinternal.ScriptSource{
				LoadPath: row.Path,
				Source:   string(row.Content),
			}
		}),
	}, nil
}

// InitialWatchStatementScriptletApplications returns the initial query and
// namespace used to watch applications backed by scriptlets.
func (st *State) InitialWatchStatementScriptletApplications() (string, eventsource.NamespaceQuery) {
	query := func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		stmt, err := st.Prepare(`
SELECT DISTINCT a.uuid AS &applicationUUID.uuid
FROM application AS a
JOIN charm_scriptlet AS cs ON cs.charm_uuid = a.charm_uuid;
`, applicationUUID{})
		if err != nil {
			return nil, errors.Errorf("preparing initial scriptlet applications query: %w", err)
		}

		var rows []applicationUUID
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt).GetAll(&rows)
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return err
		})
		if err != nil {
			return nil, errors.Errorf("getting initial scriptlet applications: %w", err)
		}
		return transform.Slice(rows, func(row applicationUUID) string {
			return row.UUID
		}), nil
	}
	return "application", query
}

// FilterScriptletApplications returns the application UUIDs whose charms have
// scriptlet sources.
func (st *State) FilterScriptletApplications(ctx context.Context, applicationIDs []string) ([]string, error) {
	if len(applicationIDs) == 0 {
		return nil, nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	ids := applicationUUIDs(applicationIDs)
	stmt, err := st.Prepare(`
SELECT DISTINCT a.uuid AS &applicationUUID.uuid
FROM application AS a
JOIN charm_scriptlet AS cs ON cs.charm_uuid = a.charm_uuid
WHERE a.uuid IN ($applicationUUIDs[:]);
`, applicationUUID{}, ids)
	if err != nil {
		return nil, errors.Errorf("preparing scriptlet applications filter: %w", err)
	}

	var rows []applicationUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ids).GetAll(&rows)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Errorf("filtering scriptlet applications: %w", err)
	}
	return transform.Slice(rows, func(row applicationUUID) string {
		return row.UUID
	}), nil
}

// GetScriptletEvent returns the event payload for an application event.
func (st *State) GetScriptletEvent(
	context.Context,
	string,
	string,
) (unitless.Event, error) {
	return unitless.Event{}, nil
}
