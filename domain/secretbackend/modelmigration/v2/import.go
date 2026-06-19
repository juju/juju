// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v2

import (
	"context"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/domain/secretbackend/state"
	"github.com/juju/juju/internal/errors"
)

// ImportSecretBackendReferences records the model's per-revision secret
// backend references. The model's own secret backend is set during
// [github.com/juju/juju/domain/model/modelmigration/v2.BootstrapImportedModel];
// this only links existing secret revisions to their backend by the target's
// backend ID.
func ImportSecretBackendReferences(
	ctx context.Context, controllerDB database.TxnRunnerFactory, logger logger.Logger,
	modelUUID coremodel.UUID, refs []coremodelmigration.SecretBackendReference,
) error {
	if len(refs) == 0 {
		return nil
	}

	st := state.NewState(controllerDB, logger)
	backendIDs := make(map[string]string)
	for _, ref := range refs {
		backendID, ok := backendIDs[ref.BackendName]
		if !ok {
			backend, err := st.GetSecretBackend(ctx, secretbackend.BackendIdentifier{Name: ref.BackendName})
			if err != nil {
				return errors.Errorf("looking up secret backend %q: %w", ref.BackendName, err)
			}
			backendID = backend.ID
			backendIDs[ref.BackendName] = backendID
		}

		if _, err := st.AddSecretBackendReference(
			ctx, &secrets.ValueRef{BackendID: backendID}, modelUUID, ref.SecretRevisionUUID, ref.SecretID,
		); err != nil {
			return errors.Errorf("adding secret backend reference for revision %q: %w", ref.SecretRevisionUUID, err)
		}
	}
	return nil
}
