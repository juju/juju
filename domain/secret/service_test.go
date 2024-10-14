// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret_test

import (
	"context"
	"database/sql"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	jujuversion "github.com/juju/juju/core/version"
	applicationservice "github.com/juju/juju/domain/application/service"
	applicationstate "github.com/juju/juju/domain/application/state"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	"github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/domain/secret/state"
	secretbackendstate "github.com/juju/juju/domain/secretbackend/state"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
)

type serviceSuite struct {
	testing.ControllerModelSuite

	modelUUID coremodel.UUID
	svc       *service.SecretService

	backendConfigGetter service.BackendAdminConfigGetter

	secretsBackendProvider        *secret.MockSecretBackendProvider
	secretsBackend                *secret.MockSecretsBackend
	secretBackendReferenceMutator *secret.MockSecretBackendReferenceMutator
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.modelUUID = modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-model")
	err := s.ModelTxnRunner(c, s.modelUUID.String()).StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, target_agent_version, name, type, cloud, cloud_type)
			VALUES (?, ?, ?, "test", "iaas", "test-model", "ec2")
		`, s.modelUUID, coretesting.ControllerTag.Id(), jujuversion.Current.String())
		return err
	})
	s.backendConfigGetter = func(context.Context) (*provider.ModelBackendConfigInfo, error) {
		return backendConfigs, nil
	}
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.secretsBackendProvider = secret.NewMockSecretBackendProvider(ctrl)
	s.secretsBackend = secret.NewMockSecretsBackend(ctrl)
	s.secretBackendReferenceMutator = secret.NewMockSecretBackendReferenceMutator(ctrl)

	s.svc = service.NewSecretService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(c, s.modelUUID.String()), nil }, loggertesting.WrapCheckLog(c)),
		secretbackendstate.NewState(func() (database.TxnRunner, error) { return s.ControllerTxnRunner(), nil }, loggertesting.WrapCheckLog(c)),
		loggertesting.WrapCheckLog(c),
		service.SecretServiceParams{BackendAdminConfigGetter: s.backendConfigGetter}).
		WithProviderGetter(func(string) (provider.SecretBackendProvider, error) { return s.secretsBackendProvider, nil }).
		WithBackendRefMutator(s.secretBackendReferenceMutator)

	return ctrl
}

type successfulToken struct{}

func (t successfulToken) Check() error {
	return nil
}

func (s *serviceSuite) createSecret(c *gc.C, data map[string]string, valueRef *coresecrets.ValueRef) (*coresecrets.URI, string) {
	ctx := context.Background()
	appService := applicationservice.NewApplicationService(
		applicationstate.NewApplicationState(
			func() (database.TxnRunner, error) { return s.ModelTxnRunner(c, s.modelUUID.String()), nil }, loggertesting.WrapCheckLog(c)),
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(c, s.modelUUID.String()), nil }, loggertesting.WrapCheckLog(c)),
		applicationservice.ApplicationServiceParams{
			StorageRegistry:               storage.NotImplementedProviderRegistry{},
			BackendAdminConfigGetter:      service.NotImplementedBackendConfigGetter,
			SecretBackendReferenceDeleter: applicationservice.NotImplementedSecretDeleter{},
		},
		loggertesting.WrapCheckLog(c),
	)
	u := applicationservice.AddUnitArg{
		UnitName: "mariadb/0",
	}
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
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)

	var revisionID string
	err = s.ModelTxnRunner(c, s.modelUUID.String()).StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT uuid FROM secret_revision WHERE secret_id = ?
		`, uri.ID).Scan(&revisionID)
	})
	c.Assert(err, jc.ErrorIsNil)
	return uri, revisionID
}

func (s *serviceSuite) TestDeleteSecretInternal(c *gc.C) {
	defer s.setup(c).Finish()

	s.secretBackendReferenceMutator.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelUUID, gomock.Any())
	uri, revisionID := s.createSecret(c, map[string]string{"foo": "bar"}, nil)

	s.secretBackendReferenceMutator.EXPECT().RemoveSecretBackendReference(gomock.Any(), []string{revisionID})

	err := s.svc.DeleteSecret(context.Background(), uri, service.DeleteSecretParams{
		LeaderToken: successfulToken{},
		Accessor: service.SecretAccessor{
			Kind: service.UnitAccessor,
			ID:   "mariadb/0",
		},
		Revisions: []int{1},
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.svc.GetSecret(context.Background(), uri)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}

var backendConfigs = &provider.ModelBackendConfigInfo{
	ActiveID: "backend-id",
	Configs: map[string]provider.ModelBackendConfig{
		"backend-id": {
			ControllerUUID: coretesting.ControllerTag.Id(),
			ModelUUID:      coretesting.ModelTag.Id(),
			ModelName:      "some-model",
			BackendConfig: provider.BackendConfig{
				BackendType: "active-type",
				Config:      map[string]interface{}{"foo": "active-type"},
			},
		},
		"other-backend-id": {
			ControllerUUID: coretesting.ControllerTag.Id(),
			ModelUUID:      coretesting.ModelTag.Id(),
			ModelName:      "some-model",
			BackendConfig: provider.BackendConfig{
				BackendType: "other-type",
				Config:      map[string]interface{}{"foo": "other-type"},
			},
		},
	},
}

func (s *serviceSuite) TestDeleteSecretExternal(c *gc.C) {
	defer s.setup(c).Finish()

	ref := &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}
	s.secretBackendReferenceMutator.EXPECT().AddSecretBackendReference(gomock.Any(), ref, s.modelUUID, gomock.Any())
	uri, revisionID := s.createSecret(c, nil, ref)

	s.secretBackendReferenceMutator.EXPECT().RemoveSecretBackendReference(gomock.Any(), []string{revisionID})

	s.secretsBackendProvider.EXPECT().Type().Return("active-type").AnyTimes()
	s.secretsBackendProvider.EXPECT().NewBackend(ptr(backendConfigs.Configs["backend-id"])).DoAndReturn(func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		return s.secretsBackend, nil
	})
	s.secretsBackend.EXPECT().DeleteContent(gomock.Any(), "rev-id")
	s.secretsBackendProvider.EXPECT().CleanupSecrets(gomock.Any(), ptr(backendConfigs.Configs["backend-id"]), "mariadb/0", provider.SecretRevisions{
		uri.ID: set.NewStrings("rev-id"),
	})

	err := s.svc.DeleteSecret(context.Background(), uri, service.DeleteSecretParams{
		LeaderToken: successfulToken{},
		Accessor: service.SecretAccessor{
			Kind: service.UnitAccessor,
			ID:   "mariadb/0",
		},
		Revisions: []int{1},
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.svc.GetSecret(context.Background(), uri)
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
}
