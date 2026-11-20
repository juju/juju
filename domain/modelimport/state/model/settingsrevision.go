// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/canonical/sqlair"

	domainrelation "github.com/juju/juju/domain/relation"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/internal/errors"
)

// InitialiseRelationSettingsRevisions assigns revisions to legacy bags after
// import. Repeated calls leave already-versioned bags and their counters alone.
func (st *State) InitialiseRelationSettingsRevisions(ctx context.Context) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.initialiseRelationSettingsRevisions(ctx, tx)
	}))
}

func (st *State) initialiseRelationSettingsRevisions(ctx context.Context, tx *sqlair.TX) error {
	type settingsRevision struct {
		UUID         string `db:"uuid"`
		RelationUUID string `db:"relation_uuid"`
		Revision     int64  `db:"revision"`
	}
	for _, statements := range []struct{ query, update string }{{
		query: `
SELECT rash.relation_endpoint_uuid AS &settingsRevision.uuid,
       re.relation_uuid AS &settingsRevision.relation_uuid
FROM relation_application_settings_hash AS rash
JOIN relation_endpoint AS re ON rash.relation_endpoint_uuid = re.uuid
WHERE rash.revision = 0
`,
		update: `
UPDATE relation_application_settings_hash
SET revision = $settingsRevision.revision
WHERE relation_endpoint_uuid = $settingsRevision.uuid
`,
	}, {
		query: `
SELECT rush.relation_unit_uuid AS &settingsRevision.uuid,
       re.relation_uuid AS &settingsRevision.relation_uuid
FROM relation_unit_settings_hash AS rush
JOIN relation_unit AS ru ON rush.relation_unit_uuid = ru.uuid
JOIN relation_endpoint AS re ON ru.relation_endpoint_uuid = re.uuid
WHERE rush.revision = 0
`,
		update: `
UPDATE relation_unit_settings_hash
SET revision = $settingsRevision.revision
WHERE relation_unit_uuid = $settingsRevision.uuid
`,
	}} {
		query, err := st.Prepare(statements.query, settingsRevision{})
		if err != nil {
			return errors.Capture(err)
		}
		update, err := st.Prepare(statements.update, settingsRevision{})
		if err != nil {
			return errors.Capture(err)
		}
		var bags []settingsRevision
		if err := tx.Query(ctx, query).GetAll(&bags); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		for _, bag := range bags {
			namespace := domainsequence.MakePrefixNamespace(domainrelation.SettingsSequenceNamespace, bag.RelationUUID)
			revision, err := sequencestate.NextValueWithStart(ctx, st, tx, namespace, 1)
			if err != nil {
				return errors.Capture(err)
			}
			bag.Revision = int64(revision)
			if err := tx.Query(ctx, update, bag).Run(); err != nil {
				return errors.Capture(err)
			}
		}
	}
	return nil
}
