// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	domainsecretbackend "github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/domain/secretbackend/state"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/uuid"
)

// CreateDefaultBackends inserts the initial secret backends during bootstrap.
func CreateDefaultBackends(modelType coremodel.ModelType) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controller, model database.TxnRunner) error {
		uuid1, err := uuid.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		uuid2, err := uuid.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Trace(controller.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			upsertBackendStmt, err := sqlair.Prepare(`
INSERT INTO secret_backend
    (uuid, name, backend_type_id)
VALUES ($SecretBackend.*)`, state.SecretBackend{})
			if err != nil {
				return errors.Trace(err)
			}
			err = tx.Query(ctx, upsertBackendStmt, state.SecretBackend{
				ID:                  uuid1.String(),
				Name:                juju.BackendName,
				BackendTypeID:       domainsecretbackend.BackendTypeController,
				TokenRotateInterval: internaldatabase.NullDuration{},
			}).Run()
			if modelType == coremodel.IAAS {
				return nil
			}

			if err != nil {
				return errors.Annotate(err, "cannot create controller secret backend")
			}
			err = tx.Query(ctx, upsertBackendStmt, state.SecretBackend{
				ID:                  uuid2.String(),
				Name:                kubernetes.BackendName,
				BackendTypeID:       domainsecretbackend.BackendTypeKubernetes,
				TokenRotateInterval: internaldatabase.NullDuration{},
			}).Run()
			return errors.Annotate(err, "cannot create kubernetes secret backend")
		}))
	}
}
