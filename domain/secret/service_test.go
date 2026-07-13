// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret_test

import (
	"context"
	"database/sql"
	"encoding/json"
	stdtesting "testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	applicationservice "github.com/juju/juju/domain/application/service"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	"github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/secret"
	"github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/domain/secret/state"
	"github.com/juju/juju/environs"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	"github.com/juju/juju/internal/secrets/provider/juju"
	coretesting "github.com/juju/juju/internal/testing"
)

type serviceSuite struct {
	testing.ControllerModelSuite

	modelUUID coremodel.UUID
	svc       *service.SecretService

	secretBackendState *secret.MockSecretBackendState
}

func TestServiceSuite(t *stdtesting.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.modelUUID = modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-model")

	err := s.ModelTxnRunner(c, s.modelUUID.String()).StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, "test", "prod", "iaas", "test-model", "ec2")
		`, s.modelUUID, coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteSecretInternal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := s.createSecret(c, map[string]string{"foo": "bar"})

	err := s.svc.DeleteSecret(c.Context(), uri, secret.DeleteSecretParams{
		Accessor: secret.SecretAccessor{
			Kind: secret.ModelAccessor,
			ID:   s.modelUUID.String(),
		},
		Revisions: []int{1},
	})
	c.Assert(err, tc.ErrorIsNil)

	s.verifySecretRemovalJob(c, uri, 1)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.secretBackendState = secret.NewMockSecretBackendState(ctrl)

	s.svc = service.NewSecretService(
		state.NewState(func(ctx context.Context) (database.TxnRunner, error) {
			return s.ModelTxnRunner(c, s.modelUUID.String()), nil
		}, loggertesting.WrapCheckLog(c), clock.WallClock),
		s.secretBackendState,
		nil,
		loggertesting.WrapCheckLog(c),
	)

	return ctrl
}

func (s *serviceSuite) createSecret(c *tc.C, data map[string]string) *coresecrets.URI {
	ctx := c.Context()

	s.secretBackendState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelUUID).Return(
		"internal",
		&provider.ModelBackendConfig{
			ControllerUUID: coretesting.ControllerTag.Id(),
			ModelUUID:      s.modelUUID.String(),
			ModelName:      "test-model",
			BackendConfig: provider.BackendConfig{
				BackendType: juju.BackendType,
			},
		}, nil,
	)
	s.secretBackendState.EXPECT().AddSecretBackendReference(
		gomock.Any(), nil, s.modelUUID, gomock.Any(), gomock.Any(),
	).Return(func() error { return nil }, nil)

	uri := coresecrets.NewURI()
	err := s.svc.CreateUserSecret(ctx, uri, service.CreateUserSecretParams{
		UpdateUserSecretParams: service.UpdateUserSecretParams{
			Accessor: secret.SecretAccessor{
				Kind: secret.ModelAccessor,
				ID:   s.modelUUID.String(),
			},
			Data:     data,
			Checksum: "checksum-1",
		},
		Version: 1,
	})
	c.Assert(err, tc.ErrorIsNil)
	return uri
}

func (s *serviceSuite) verifySecretRemovalJob(c *tc.C, uri *coresecrets.URI, revision int) {
	q := `SELECT arg FROM removal WHERE entity_uuid = $1`
	type jobArgs struct {
		Revisions []int `json:"revisions"`
	}
	var arg jobArgs
	err := s.ModelTxnRunner(c, s.modelUUID.String()).StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(c.Context(), q, uri.String())
		var a string
		if err := row.Scan(&a); err != nil {
			return err
		}
		return json.Unmarshal([]byte(a), &arg)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(arg.Revisions, tc.SameContents, []int{revision})
}

type serviceProvider struct {
	applicationservice.Provider
	applicationservice.CAASProvider
}

func (serviceProvider) ConstraintsValidator(_ context.Context) (constraints.Validator, error) {
	return constraints.NewValidator(), nil
}

func (serviceProvider) PrecheckInstance(_ context.Context, _ environs.PrecheckInstanceParams) error {
	return nil
}
