// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"strings"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/internal/errors"
)

// replayTable holds only schema-generated statements and typed row slices.
type replayTable struct {
	name, deleteSQL, insertSQL string
	sample                     any
	insertRows                 func(context.Context, *sqlair.TX, *sqlair.Statement) error
}

// loadTables deletes before inserting so replacing parent rows cannot collide
// with the target's children. Foreign keys remain enabled; checks are deferred
// for this transaction and verified before its commit. No connection PRAGMAs
// need restoring after success, cancellation or rollback.
func (st *State) loadTables(ctx context.Context, tables []replayTable) error {
	type statements struct {
		delete, insert *sqlair.Statement
	}
	prepared := make([]statements, len(tables))
	for i, table := range tables {
		var err error
		prepared[i].delete, err = st.Prepare(table.deleteSQL)
		if err != nil {
			return errors.Capture(err)
		}
		prepared[i].insert, err = st.Prepare(table.insertSQL, table.sample)
		if err != nil {
			return errors.Errorf("preparing table %q: %w", table.name, err)
		}
	}
	deferFK, err := st.Prepare("PRAGMA defer_foreign_keys = ON")
	if err != nil {
		return errors.Capture(err)
	}
	type placement struct {
		NodeID string `db:"node_id"`
	}
	getNode, err := st.Prepare(`SELECT osp.node_id AS &placement.node_id
FROM object_store_placement AS osp LIMIT 1`, placement{})
	if err != nil {
		return errors.Capture(err)
	}
	deletePlacements, err := st.Prepare("DELETE FROM object_store_placement AS osp")
	if err != nil {
		return errors.Capture(err)
	}
	insertPlacements, err := st.Prepare(`
INSERT INTO object_store_placement (uuid, node_id)
SELECT osm.uuid AS uuid, $placement.node_id AS node_id
FROM object_store_metadata AS osm`, placement{})
	if err != nil {
		return errors.Capture(err)
	}
	type violation struct {
		Table  string `db:"table_name"`
		Parent string `db:"parent_name"`
	}
	checkFK, err := st.Prepare(`
SELECT fkc."table" AS &violation.table_name,
       fkc.parent AS &violation.parent_name
FROM pragma_foreign_key_check AS fkc LIMIT 11`, violation{})
	if err != nil {
		return errors.Capture(err)
	}
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, deferFK).Run(); err != nil {
			return errors.Capture(err)
		}
		node := placement{NodeID: "0"}
		if err := tx.Query(ctx, getNode).Get(&node); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		if err := tx.Query(ctx, deletePlacements).Run(); err != nil {
			return errors.Capture(err)
		}
		for i, table := range tables {
			if err := tx.Query(ctx, prepared[i].delete).Run(); err != nil {
				return errors.Errorf("deleting table %q: %w", table.name, err)
			}
		}
		for i, table := range tables {
			if err := table.insertRows(ctx, tx, prepared[i].insert); err != nil {
				return errors.Errorf("inserting table %q: %w", table.name, err)
			}
		}
		if err := tx.Query(ctx, insertPlacements, node).Run(); err != nil {
			return errors.Capture(err)
		}
		var violations []violation
		if err := tx.Query(ctx, checkFK).GetAll(&violations); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		if len(violations) > 0 {
			var descriptions []string
			for _, v := range violations {
				descriptions = append(descriptions, v.Table+" row referencing "+v.Parent)
			}
			return errors.Errorf("loaded database fails foreign key integrity: %s", strings.Join(descriptions, "; "))
		}
		return nil
	}))
}

// SQLair bulk VALUES bindings do not support expressions for column defaults.
// Execute the generated INSERT SELECT once per typed row instead.
func insertRows[T any](ctx context.Context, tx *sqlair.TX, stmt *sqlair.Statement, rows []T) error {
	for _, row := range rows {
		if err := tx.Query(ctx, stmt, row).Run(); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}
