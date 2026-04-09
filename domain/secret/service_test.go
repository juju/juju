// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret_test

import (
	"context"
	"database/sql"
	"encoding/json"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain"
	applicationservice "github.com/juju/juju/domain/application/service"
	applicationstorageservice "github.com/juju/juju/domain/application/service/storage"
	applicationstate "github.com/juju/juju/domain/application/state"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	"github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/secret"
	"github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/domain/secret/state"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/environs"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalstorage "github.com/juju/juju/internal/storage"
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

	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelUUID, gomock.Any(), gomock.Any())
	uri := s.createSecret(c, map[string]string{"foo": "bar"}, nil)

	err := s.svc.DeleteSecret(c.Context(), uri, secret.DeleteSecretParams{
		Accessor: secret.SecretAccessor{
			Kind: secret.UnitAccessor,
			ID:   "mariadb/0",
		},
		Revisions: []int{1},
	})
	c.Assert(err, tc.ErrorIsNil)

	s.verifySecretRemovalJob(c, uri, 1)
}

func (s *serviceSuite) TestDeleteSecretExternal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ref := &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), ref, s.modelUUID, gomock.Any(), gomock.Any())
	uri := s.createSecret(c, nil, ref)

	err := s.svc.DeleteSecret(c.Context(), uri, secret.DeleteSecretParams{
		Accessor: secret.SecretAccessor{
			Kind: secret.UnitAccessor,
			ID:   "mariadb/0",
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
		}, loggertesting.WrapCheckLog(c)),
		s.secretBackendState,
		nil,
		loggertesting.WrapCheckLog(c),
	)

	return ctrl
}

func (s *serviceSuite) createSecret(c *tc.C, data map[string]string, valueRef *coresecrets.ValueRef) *coresecrets.URI {
	ctx := c.Context()
	st := applicationstate.NewState(func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(c, s.modelUUID.String()), nil
	}, s.modelUUID, clock.WallClock, loggertesting.WrapCheckLog(c))
	storageProviderRegistryGetter := corestorage.ConstModelStorageRegistry(
		func() internalstorage.ProviderRegistry {
			return internalstorage.NotImplementedProviderRegistry{}
		},
	)
	storageSvc := applicationstorageservice.NewService(
		st,
		applicationstorageservice.NewStoragePoolProvider(
			storageProviderRegistryGetter, st,
		),
		loggertesting.WrapCheckLog(c),
	)

	appService := applicationservice.NewProviderService(
		st,
		storageSvc,
		domaintesting.NoopLeaderEnsurer(),
		nil,
		func(ctx context.Context) (applicationservice.Provider, error) {
			return serviceProvider{}, nil
		},
		func(ctx context.Context) (applicationservice.CAASProvider, error) {
			return serviceProvider{}, nil
		},
		func(ctx context.Context) (applicationservice.CloudInfoProvider, error) {
			return nil, coreerrors.NotSupported
		},
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		s.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	u := applicationservice.AddIAASUnitArg{}
	_, err := appService.CreateIAASApplication(ctx, "mariadb", &stubCharm{}, corecharm.Origin{
		Source: corecharm.Local,
		Platform: corecharm.Platform{
			Channel:      "24.04",
			OS:           "ubuntu",
			Architecture: "amd64",
		},
	}, applicationservice.AddApplicationArgs{
		ReferenceName: "mariadb",
	}, u)
	c.Assert(err, tc.ErrorIsNil)

	uri := coresecrets.NewURI()
	err = s.svc.CreateCharmSecret(ctx, uri, secret.CreateCharmSecretParams{
		UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
			Data:     data,
			ValueRef: valueRef,
		},
		Version: 1,
		CharmOwner: secret.CharmSecretOwner{
			Kind: secret.UnitCharmSecretOwner,
			ID:   "mariadb/0",
		},
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
