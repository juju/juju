// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
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
		err := controller.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := createBackend(ctx, tx, juju.BackendName, domainsecretbackend.BackendTypeController)
			if err != nil {
				return errors.Trace(err)
			}
			if modelType == coremodel.CAAS {
				err := createBackend(ctx, tx, kubernetes.BackendName, domainsecretbackend.BackendTypeKubernetes)
				if err != nil {
					return errors.Trace(err)
				}
			}
			return nil
		})
		return domain.CoerceError(err)
	}
}

func createBackend(ctx context.Context, tx *sqlair.TX, backendName string, backendType domainsecretbackend.BackendType) error {
	backendUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}
	upsertBackendStmt, err := sqlair.Prepare(`
INSERT INTO secret_backend
    (uuid, name, backend_type_id)
VALUES ($SecretBackend.*)`, state.SecretBackend{})
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, upsertBackendStmt, state.SecretBackend{
		ID:                  backendUUID.String(),
		Name:                backendName,
		BackendTypeID:       backendType,
		TokenRotateInterval: internaldatabase.NullDuration{},
	}).Run()
	return errors.Annotatef(err, "cannot create secret backend %q", backendName)
}
