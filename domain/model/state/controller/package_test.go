// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/uuid"
)

type preparer struct{}

func (p preparer) Prepare(query string, args ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, args...)
}

// CreateControllerModel creates a controller model in the database for use
// in tests.
func CreateControllerModel(c *tc.C, runner database.TxnRunner, controllerModelUUID coremodel.UUID, userUUID user.UUID) {
	// Before we can create the model, we need to create a controller model.
	// This ensures that we
	err := runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		err := Create(c.Context(), preparer{}, tx, controllerModelUUID, coremodel.IAAS, model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Name:          "controller",
			Qualifier:     "prod",
			AdminUsers:    []user.UUID{userUUID},
			SecretBackend: juju.BackendName,
		})
		if err != nil {
			return err
		}

		type controllerInfo struct {
			UUID          string `db:"uuid"`
			ModelUUID     string `db:"model_uuid"`
			TargetVersion string `db:"target_version"`
		}

		info := controllerInfo{
			UUID:          uuid.MustNewUUID().String(),
			ModelUUID:     controllerModelUUID.String(),
			TargetVersion: "4.0.0",
		}

		stmt, err := sqlair.Prepare(`INSERT INTO controller (uuid, model_uuid, target_version) VALUES ($controllerInfo.*)`, info)
		if err != nil {
			return err
		}

		if err := tx.Query(ctx, stmt, info).Run(); err != nil {
			return err
		}

		activator := GetActivator()
		return activator(ctx, preparer{}, tx, controllerModelUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
}

// CreateModel creates a model in the database for use in tests.
func CreateModel(c *tc.C, runner database.TxnRunnerFactory, modelUUID coremodel.UUID, userUUID user.UUID) {
	modelSt := NewState(runner)
	err := modelSt.Create(
		c.Context(),
		modelUUID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Name:          "my-test-model",
			Qualifier:     "prod",
			AdminUsers:    []user.UUID{userUUID},
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	err = modelSt.Activate(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}
