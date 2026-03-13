// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/internal/errors"
)

func (st *State) removeBuiltInSecretBackendForModel(ctx context.Context, tx *sqlair.TX, modelName string) error {
	type backend entityUUID
	type term entityName
	search := term{Name: secretbackend.MakeBuiltInK8sSecretBackendName(modelName)}
	uuidStmt, err := st.Prepare(`
SELECT &backend.uuid 
FROM   secret_backend
WHERE  name = $term.name
AND    origin_id = 0 -- built-in secret backend
`, backend{}, term{})
	if err != nil {
		return errors.Capture(err)
	}

	tables := []string{
		"DELETE FROM secret_backend_config WHERE backend_uuid = $backend.uuid",
		"DELETE FROM secret_backend_rotation WHERE backend_uuid = $backend.uuid",
		"DELETE FROM secret_backend_reference WHERE secret_backend_uuid = $backend.uuid",
		"DELETE FROM model_secret_backend WHERE secret_backend_uuid = $backend.uuid",
		"DELETE FROM secret_backend WHERE uuid = $backend.uuid",
	}
	var backendUUID backend
	err = tx.Query(ctx, uuidStmt, search).Get(&backendUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil
	} else if err != nil {
		return errors.Errorf("getting UUID for built-in secret backend %q: %w", search.Name, err)
	}

	for _, table := range tables {
		stmt, err := st.Prepare(table, backend{})
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, stmt, backendUUID).Run(); err != nil {
			return errors.Errorf(
				"deleting reference to built-in secret backend %q in table %q: %w",
				search.Name, table, err,
			)
		}
	}

	return nil
}
