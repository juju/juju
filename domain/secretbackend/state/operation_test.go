// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secretbackend"
	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/uuid"
)

func (s *stateSuite) TestUpsertOperationBuild(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)

	params := secretbackend.UpsertSecretBackendParams{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
		Config: map[string]interface{}{
			"key1": "value1",
		},
	}
	op := upsertOperation{UpsertSecretBackendParams: params}
	err := op.build()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestUpsertOperationBuildFailed(c *gc.C) {
	op := upsertOperation{}
	err := op.build()
	c.Assert(err, jc.ErrorIs, backenderrors.NotValid)
	c.Assert(err, gc.ErrorMatches, "secret backend not valid: ID is missing")

	op = upsertOperation{
		UpsertSecretBackendParams: secretbackend.UpsertSecretBackendParams{
			ID: "id-xx",
			Config: map[string]interface{}{
				"": "value1",
			},
		},
	}
	err = op.build()
	c.Assert(err, jc.ErrorIs, backenderrors.NotValid)
	c.Assert(err, gc.ErrorMatches, `secret backend not valid: empty config key for "id-xx"`)

	op = upsertOperation{
		UpsertSecretBackendParams: secretbackend.UpsertSecretBackendParams{
			ID: "id-xx",
			Config: map[string]interface{}{
				"key1": "",
			},
		},
	}
	err = op.build()
	c.Assert(err, jc.ErrorIs, backenderrors.NotValid)
	c.Assert(err, gc.ErrorMatches, `secret backend not valid: empty config value for "id-xx"`)
}

func (s *stateSuite) assertPrepareData(c *gc.C, hasExisting bool, f func(*secretbackend.UpsertSecretBackendParams), expectedErr string) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)

	params := secretbackend.UpsertSecretBackendParams{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
		Config: map[string]interface{}{
			"key1": "value1",
		},
	}
	if hasExisting {
		db := s.DB()
		q := `
INSERT INTO secret_backend
    (uuid, name, backend_type, token_rotate_interval)
VALUES (?, ?, ?, ?)`
		_, err := db.ExecContext(
			context.Background(), q,
			params.ID, params.Name, params.BackendType, params.TokenRotateInterval,
		)
		c.Assert(err, jc.ErrorIsNil)

		q = `
INSERT INTO secret_backend_rotation
    (backend_uuid, next_rotation_time)
VALUES (?, ?)`
		_, err = db.ExecContext(
			context.Background(), q,
			params.ID, params.NextRotateTime,
		)
		c.Assert(err, jc.ErrorIsNil)

		q = `
INSERT INTO secret_backend_config
    (backend_uuid, name, content)
VALUES (?, ?, ?)`
		_, err = db.ExecContext(
			context.Background(), q,
			params.ID, "key1", "value1",
		)
		c.Assert(err, jc.ErrorIsNil)

		s.assertSecretBackend(c, coresecrets.SecretBackend{
			ID:                  backendID,
			Name:                "my-backend",
			BackendType:         "vault",
			TokenRotateInterval: &rotateInternal,
			Config: map[string]interface{}{
				"key1": "value1",
			},
		}, &nextRotateTime)
	}

	f(&params)
	_ = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		op := upsertOperation{UpsertSecretBackendParams: params}
		err := op.build()
		c.Assert(err, jc.ErrorIsNil)
		err = op.prepareData(ctx, tx)
		if expectedErr == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, expectedErr)
		}
		return nil
	})
}

func (s *stateSuite) TestUpsertOperationInsert(c *gc.C) {
	s.assertPrepareData(c, false,
		func(params *secretbackend.UpsertSecretBackendParams) {
			params.Name = ""
		},
		`secret backend not valid: name is missing`,
	)

	s.assertPrepareData(c, false,
		func(params *secretbackend.UpsertSecretBackendParams) {
			params.BackendType = ""
		},
		`secret backend not valid: type is missing`,
	)
}

func (s *stateSuite) TestUpsertOperationUpdateFailedTypeImmutable(c *gc.C) {
	s.assertPrepareData(c, true,
		func(params *secretbackend.UpsertSecretBackendParams) {
			params.BackendType = "kubernetes"
		},
		`cannot change backend type from "vault" to "kubernetes" because backend type is immutable`,
	)
}
