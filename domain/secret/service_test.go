// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret_test

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain"
	applicationservice "github.com/juju/juju/domain/application/service"
	applicationstate "github.com/juju/juju/domain/application/state"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	"github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/domain/secret/state"
	domaintesting "github.com/juju/juju/domain/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
)

type serviceSuite struct {
	testing.ControllerModelSuite

	modelUUID coremodel.UUID
	svc       *service.SecretService

	secretBackendState *secret.MockSecretBackendState
}

func TestServiceSuite(t *stdtesting.T) { tc.Run(t, &serviceSuite{}) }
func (s *serviceSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.modelUUID = modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-model")

	err := s.ModelTxnRunner(c, s.modelUUID.String()).StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type)
			VALUES (?, ?, "test", "iaas", "test-model", "ec2")
		`, s.modelUUID, coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteSecretInternal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelUUID, gomock.Any())
	uri := s.createSecret(c, map[string]string{"foo": "bar"}, nil)

	err := s.svc.DeleteSecret(c.Context(), uri, service.DeleteSecretParams{
		Accessor: service.SecretAccessor{
			Kind: service.UnitAccessor,
			ID:   "mariadb/0",
		},
		Revisions: []int{1},
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.svc.GetSecret(c.Context(), uri)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *serviceSuite) TestDeleteSecretExternal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ref := &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), ref, s.modelUUID, gomock.Any())
	uri := s.createSecret(c, nil, ref)

	err := s.svc.DeleteSecret(c.Context(), uri, service.DeleteSecretParams{
		Accessor: service.SecretAccessor{
			Kind: service.UnitAccessor,
			ID:   "mariadb/0",
		},
		Revisions: []int{1},
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.svc.GetSecret(c.Context(), uri)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.secretBackendState = secret.NewMockSecretBackendState(ctrl)

	s.svc = service.NewSecretService(
		state.NewState(func() (database.TxnRunner, error) {
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
	st := applicationstate.NewState(func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(c, s.modelUUID.String()), nil
	}, clock.WallClock, loggertesting.WrapCheckLog(c))

	appService := applicationservice.NewProviderService(
		st,
		domaintesting.NoopLeaderEnsurer(),
		corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
			return storage.NotImplementedProviderRegistry{}
		}),
		s.modelUUID,
		nil,
		func(ctx context.Context) (applicationservice.Provider, error) {
			return serviceProvider{}, nil
		},
		func(ctx context.Context) (applicationservice.SupportedFeatureProvider, error) {
			return serviceProvider{}, nil
		},
		func(ctx context.Context) (applicationservice.CAASApplicationProvider, error) {
			return serviceProvider{}, nil
		},
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	u := applicationservice.AddUnitArg{}
	_, err := appService.CreateApplication(ctx, "mariadb", &stubCharm{}, corecharm.Origin{
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
	err = s.svc.CreateCharmSecret(ctx, uri, service.CreateCharmSecretParams{
		UpdateCharmSecretParams: service.UpdateCharmSecretParams{
			Data:     data,
			ValueRef: valueRef,
		},
		Version: 1,
		CharmOwner: service.CharmSecretOwner{
			Kind: service.UnitOwner,
			ID:   "mariadb/0",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	return uri
}

type serviceProvider struct {
	applicationservice.Provider
	applicationservice.SupportedFeatureProvider
	applicationservice.CAASApplicationProvider
}

func (serviceProvider) ConstraintsValidator(ctx context.Context) (constraints.Validator, error) {
	return nil, nil
}
