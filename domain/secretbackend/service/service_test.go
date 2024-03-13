// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	time "time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/cloud"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/secrets/provider/vault"
	jujutesting "github.com/juju/juju/testing"
)

func ptr[T any](v T) *T {
	return &v
}

type providerWithConfig struct {
	provider.ProviderConfig
	provider.SupportAuthRefresh
	provider.SecretBackendProvider
}

func (providerWithConfig) ConfigSchema() environschema.Fields {
	return environschema.Fields{
		"token": {
			Secret: true,
		},
	}
}

func (providerWithConfig) ConfigDefaults() schema.Defaults {
	return schema.Defaults{
		"namespace": "foo",
	}
}

func (p providerWithConfig) ValidateConfig(oldCfg, newCfg provider.ConfigAttrs) error {
	if p.Type() == "something" {
		return errors.NotValidf("config for provider %q", p.Type())
	}
	return nil
}

func (providerWithConfig) RefreshAuth(cfg provider.BackendConfig, validFor time.Duration) (*provider.BackendConfig, error) {
	result := cfg
	result.Config["token"] = validFor.String()
	return &result, nil
}

var (
	jujuBackendID  = jujutesting.ControllerTag.Id()
	k8sBackendID   = jujutesting.ModelTag.Id()
	vaultBackendID = "vault-backend-id"

	jujuBackendConfig = provider.ModelBackendConfig{
		ControllerUUID: jujutesting.ControllerTag.Id(),
		ModelUUID:      jujutesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: juju.BackendType,
		},
	}
	k8sBackendConfig = provider.ModelBackendConfig{
		ControllerUUID: jujutesting.ControllerTag.Id(),
		ModelUUID:      jujutesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: kubernetes.BackendType,
			Config: provider.ConfigAttrs{
				"endpoint":            "http://nowhere",
				"ca-certs":            []string{"cert-data"},
				"credential":          `{"auth-type":"access-key","Attributes":{"foo":"bar"}}`,
				"is-controller-cloud": true,
			},
		},
	}
	vaultBackendConfig = provider.ModelBackendConfig{
		ControllerUUID: jujutesting.ControllerTag.Id(),
		ModelUUID:      jujutesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
			Config: provider.ConfigAttrs{
				"endpoint": "http://vault",
			},
		},
	}
)

type serviceSuite struct {
	testing.IsolationSuite

	mockState                      *MockState
	mockWatcherFactory             *MockWatcherFactory
	mockRegistry                   *MockSecretBackendProvider
	mockSecretProvider             *MockSecretsBackend
	mockSecretBackendRotateWatcher *MockSecretBackendRotateWatcher
	mockModelService               *MockModelGetter

	mockClock *MockClock
	logger    Logger
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockState = NewMockState(ctrl)
	s.mockWatcherFactory = NewMockWatcherFactory(ctrl)
	s.mockRegistry = NewMockSecretBackendProvider(ctrl)
	s.mockSecretProvider = NewMockSecretsBackend(ctrl)
	s.mockSecretBackendRotateWatcher = NewMockSecretBackendRotateWatcher(ctrl)
	s.mockModelService = NewMockModelGetter(ctrl)

	s.mockClock = NewMockClock(ctrl)
	s.logger = jujutesting.NewCheckLogger(c)

	return ctrl
}

func (s *serviceSuite) assertGetSecretBackendConfigForAdminDefault(
	c *gc.C, svc *Service, modelType string, backendName string, expected *provider.ModelBackendConfigInfo,
) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var cld cloud.Cloud
	var cred cloud.Credential
	if modelType == "caas" {
		cld = cloud.Cloud{
			Name:              "test",
			Type:              "kubernetes",
			Endpoint:          "http://nowhere",
			CACertificates:    []string{"cert-data"},
			IsControllerCloud: true,
		}
		cred = cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{"foo": "bar"})
	}

	s.mockState.EXPECT().ListSecretBackends(gomock.Any()).Return([]secretbackend.SecretBackendInfo{{
		SecretBackend: coresecrets.SecretBackend{ID: vaultBackendID,
			Name:        "myvault",
			BackendType: vault.BackendType,
			Config: map[string]interface{}{
				"endpoint": "http://vault",
			},
		},
	}}, nil)
	s.mockModelService.EXPECT().GetModel(gomock.Any(), model.UUID(jujutesting.ModelTag.Id())).
		Return(&coremodel.Model{
			UUID:      jujutesting.ModelTag.Id(),
			Name:      "fred",
			ModelType: coremodel.ModelType(modelType),
		}, nil)
	s.mockModelService.EXPECT().GetSecretBackend(gomock.Any(), model.UUID(jujutesting.ModelTag.Id())).
		Return(model.SecretBackendIdentifier{
			UUID: vaultBackendID,
			Name: backendName,
		}, nil)

	info, err := svc.GetSecretBackendConfigForAdmin(
		context.Background(),
		model.UUID(jujutesting.ModelTag.Id()), s.mockModelService, cld, cred,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)
}

func (s *serviceSuite) TestGetSecretBackendConfigForAdminDefaultIAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, gc.Equals, "vault")
			return s.mockRegistry, nil
		},
	)
	s.assertGetSecretBackendConfigForAdminDefault(c, svc, "iaas", "auto",
		&provider.ModelBackendConfigInfo{
			ActiveID: jujuBackendID,
			Configs: map[string]provider.ModelBackendConfig{
				jujuBackendID:  jujuBackendConfig,
				vaultBackendID: vaultBackendConfig,
			},
		},
	)
}

func (s *serviceSuite) TestGetSecretBackendConfigForAdminDefaultCAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, gc.Equals, "vault")
			return s.mockRegistry, nil
		},
	)
	s.assertGetSecretBackendConfigForAdminDefault(c, svc, "caas", "auto",
		&provider.ModelBackendConfigInfo{
			ActiveID: k8sBackendID,
			Configs: map[string]provider.ModelBackendConfig{
				k8sBackendID:   k8sBackendConfig,
				jujuBackendID:  jujuBackendConfig,
				vaultBackendID: vaultBackendConfig,
			},
		},
	)
}

func (s *serviceSuite) TestGetSecretBackendConfigForAdminInternalIAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, gc.Equals, "vault")
			return s.mockRegistry, nil
		},
	)
	s.assertGetSecretBackendConfigForAdminDefault(c, svc, "iaas", "internal",
		&provider.ModelBackendConfigInfo{
			ActiveID: jujuBackendID,
			Configs: map[string]provider.ModelBackendConfig{
				jujuBackendID:  jujuBackendConfig,
				vaultBackendID: vaultBackendConfig,
			},
		},
	)
}

func (s *serviceSuite) TestGetSecretBackendConfigForAdminInternalCAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, gc.Equals, "vault")
			return s.mockRegistry, nil
		},
	)
	s.assertGetSecretBackendConfigForAdminDefault(c, svc, "caas", "internal",
		&provider.ModelBackendConfigInfo{
			ActiveID: jujuBackendID,
			Configs: map[string]provider.ModelBackendConfig{
				k8sBackendID:   k8sBackendConfig,
				jujuBackendID:  jujuBackendConfig,
				vaultBackendID: vaultBackendConfig,
			},
		},
	)
}

func (s *serviceSuite) TestGetSecretBackendConfigForAdminExternalIAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, gc.Equals, "vault")
			return s.mockRegistry, nil
		},
	)
	s.assertGetSecretBackendConfigForAdminDefault(c, svc, "iaas", "myvault",
		&provider.ModelBackendConfigInfo{
			ActiveID: vaultBackendID,
			Configs: map[string]provider.ModelBackendConfig{
				jujuBackendID:  jujuBackendConfig,
				vaultBackendID: vaultBackendConfig,
			},
		},
	)
}

func (s *serviceSuite) TestGetSecretBackendConfigForAdminExternalCAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, gc.Equals, "vault")
			return s.mockRegistry, nil
		},
	)
	s.assertGetSecretBackendConfigForAdminDefault(c, svc, "caas", "myvault",
		&provider.ModelBackendConfigInfo{
			ActiveID: vaultBackendID,
			Configs: map[string]provider.ModelBackendConfig{
				k8sBackendID:   k8sBackendConfig,
				jujuBackendID:  jujuBackendConfig,
				vaultBackendID: vaultBackendConfig,
			},
		},
	)
}

func (s *serviceSuite) TestGetSecretBackendConfigLegacy(c *gc.C) {
	c.Skip("TODO: wait for secret DqLite support")
}

func (s *serviceSuite) TestGetSecretBackendConfig(c *gc.C) {
	c.Skip("TODO: wait for secret DqLite support")
}

func (s *serviceSuite) TestGetSecretBackendConfigForDrain(c *gc.C) {
	c.Skip("TODO: wait for secret DqLite support")
}

func (s *serviceSuite) assertBackendSummaryInfo(
	c *gc.C, svc *Service, modelType coremodel.ModelType,
	reveal bool, filter secretbackend.SecretBackendFilter,
	expected []secretbackend.SecretBackendInfo,
) {
	s.mockState.EXPECT().ListSecretBackends(gomock.Any()).Return([]secretbackend.SecretBackendInfo{
		{
			SecretBackend: coresecrets.SecretBackend{
				ID:          vaultBackendID,
				Name:        "myvault",
				BackendType: vault.BackendType,
				Config: map[string]interface{}{
					"endpoint": "http://vault",
					"token":    "deadbeef",
				},
			},
		},
		{
			SecretBackend: coresecrets.SecretBackend{
				ID:          "another-vault-id",
				Name:        "another-vault",
				BackendType: vault.BackendType,
				Config: map[string]interface{}{
					"endpoint": "http://another-vault",
				},
			},
		},
	}, nil)

	if filter.All {
		s.mockModelService.EXPECT().GetModel(gomock.Any(), model.UUID(jujutesting.ModelTag.Id())).
			Return(&coremodel.Model{
				UUID:      jujutesting.ModelTag.Id(),
				Name:      "fred",
				ModelType: modelType,
			}, nil)
	}

	s.mockRegistry.EXPECT().Type().Return(vault.BackendType).AnyTimes()
	pingCount := 0
	if set.NewStrings(filter.Names...).Contains("myvault") || filter.All {
		s.mockRegistry.EXPECT().NewBackend(&provider.ModelBackendConfig{
			BackendConfig: provider.BackendConfig{
				BackendType: vault.BackendType,
				Config: provider.ConfigAttrs{
					"endpoint": "http://vault",
					"token":    "deadbeef",
				},
			},
		}).Return(s.mockSecretProvider, nil)
		pingCount++
	}
	if set.NewStrings(filter.Names...).Contains("another-vault") || filter.All {
		s.mockRegistry.EXPECT().NewBackend(&provider.ModelBackendConfig{
			BackendConfig: provider.BackendConfig{
				BackendType: vault.BackendType,
				Config: provider.ConfigAttrs{
					"endpoint": "http://another-vault",
				},
			},
		}).Return(s.mockSecretProvider, nil)
		pingCount++
	}
	// We only ping external backends - the vault is the one we currently support.
	s.mockSecretProvider.EXPECT().Ping().Return(nil).Times(pingCount)

	cld := cloud.Cloud{
		Name:              "test",
		Type:              "kubernetes",
		Endpoint:          "http://nowhere",
		CACertificates:    []string{"cert-data"},
		IsControllerCloud: true,
	}
	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{"foo": "bar"})
	info, err := svc.BackendSummaryInfo(context.Background(),
		model.UUID(jujutesting.ModelTag.Id()),
		s.mockModelService, cld, cred, reveal, filter,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.DeepEquals, expected)
}

func (s *serviceSuite) TestBackendSummaryInfoWithFilterAllCAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			if backendType != vault.BackendType {
				return s.mockRegistry, nil
			}
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)
	s.assertBackendSummaryInfo(
		c, svc, coremodel.CAAS, false,
		secretbackend.SecretBackendFilter{All: true},
		[]secretbackend.SecretBackendInfo{
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          "vault-backend-id",
					Name:        "myvault",
					BackendType: vault.BackendType,
					Config: map[string]interface{}{
						"endpoint": "http://vault",
					},
				},
			},
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          "another-vault-id",
					Name:        "another-vault",
					BackendType: vault.BackendType,
					Config: map[string]interface{}{
						"endpoint": "http://another-vault",
					},
				},
			},
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          jujutesting.ControllerTag.Id(),
					Name:        juju.BackendName,
					BackendType: juju.BackendType,
				},
			},
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          "deadbeef-0bad-400d-8000-4b1d0d06f00d",
					Name:        "fred-local",
					BackendType: "kubernetes",
					Config: map[string]interface{}{
						"ca-certs":            []string{"cert-data"},
						"credential":          `{"auth-type":"access-key","Attributes":{"foo":"bar"}}`,
						"endpoint":            "http://nowhere",
						"is-controller-cloud": true,
					},
				}, NumSecrets: 1,
			},
		},
	)
}

func (s *serviceSuite) TestBackendSummaryInfoWithFilterAllIAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			if backendType != vault.BackendType {
				return s.mockRegistry, nil
			}
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)
	s.assertBackendSummaryInfo(
		c, svc, coremodel.IAAS, false,
		secretbackend.SecretBackendFilter{All: true},
		[]secretbackend.SecretBackendInfo{
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          "vault-backend-id",
					Name:        "myvault",
					BackendType: vault.BackendType,
					Config: map[string]interface{}{
						"endpoint": "http://vault",
					},
				},
			},
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          "another-vault-id",
					Name:        "another-vault",
					BackendType: vault.BackendType,
					Config: map[string]interface{}{
						"endpoint": "http://another-vault",
					},
				},
			},
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          jujutesting.ControllerTag.Id(),
					Name:        juju.BackendName,
					BackendType: juju.BackendType,
				},
			},
		},
	)
}

func (s *serviceSuite) TestBackendSummaryInfoWithFilterNames(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			if backendType != vault.BackendType {
				return s.mockRegistry, nil
			}
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)
	s.assertBackendSummaryInfo(
		c, svc, coremodel.IAAS, false,
		secretbackend.SecretBackendFilter{
			Names: []string{"another-vault"},
		},
		[]secretbackend.SecretBackendInfo{
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          "another-vault-id",
					Name:        "another-vault",
					BackendType: vault.BackendType,
					Config: map[string]interface{}{
						"endpoint": "http://another-vault",
					},
				},
			},
		},
	)
}

func (s *serviceSuite) TestBackendSummaryInfoWithFilterNamesNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			if backendType != vault.BackendType {
				return s.mockRegistry, nil
			}
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)
	s.assertBackendSummaryInfo(
		c, svc, coremodel.IAAS, false,
		secretbackend.SecretBackendFilter{
			Names: []string{"non-existing-vault"},
		},
		[]secretbackend.SecretBackendInfo{},
	)
}

func (s *serviceSuite) TestCheckSecretBackend(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, gc.Equals, "vault")
			return s.mockRegistry, nil
		},
	)
	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), "backend-uuid").Return(&coresecrets.SecretBackend{
		ID:          "backend-uuid",
		Name:        "myvault",
		BackendType: vault.BackendType,
		Config: map[string]interface{}{
			"endpoint": "http://vault",
		},
	}, nil)

	s.mockRegistry.EXPECT().Type().Return(vault.BackendType)
	s.mockRegistry.EXPECT().NewBackend(&provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
			Config: provider.ConfigAttrs{
				"endpoint": "http://vault",
			},
		},
	}).Return(s.mockSecretProvider, nil)
	s.mockSecretProvider.EXPECT().Ping().Return(nil)
	err := svc.CheckSecretBackend(context.Background(), "backend-uuid")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreateSecretBackendFailed(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, gc.Equals, "something")
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	err := svc.CreateSecretBackend(context.Background(), coresecrets.SecretBackend{})
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err, gc.ErrorMatches, "missing backend ID")

	err = svc.CreateSecretBackend(context.Background(), coresecrets.SecretBackend{
		ID: "backend-uuid",
	})
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err, gc.ErrorMatches, "missing backend name")

	err = svc.CreateSecretBackend(context.Background(), coresecrets.SecretBackend{
		ID:   "backend-uuid",
		Name: juju.BackendName,
	})
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err, gc.ErrorMatches, `backend "internal" not valid`)

	err = svc.CreateSecretBackend(context.Background(), coresecrets.SecretBackend{
		ID:   "backend-uuid",
		Name: provider.Auto,
	})
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err, gc.ErrorMatches, `backend "auto" not valid`)

	s.mockRegistry.EXPECT().Type().Return("something").AnyTimes()
	err = svc.CreateSecretBackend(context.Background(), coresecrets.SecretBackend{
		ID:          "backend-uuid",
		Name:        "invalid",
		BackendType: "something",
	})
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(errors.Cause(err), gc.ErrorMatches, `config for provider "something" not valid`)
}

func (s *serviceSuite) TestCreateSecretBackend(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, gc.Equals, "vault")
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	addedConfig := map[string]interface{}{
		"endpoint":  "http://vault",
		"namespace": "foo",
	}
	now := time.Now()
	s.mockClock.EXPECT().Now().Return(now)
	s.mockState.EXPECT().CreateSecretBackend(gomock.Any(), secretbackend.CreateSecretBackendParams{
		ID:                  "backend-uuid",
		Name:                "myvault",
		BackendType:         vault.BackendType,
		TokenRotateInterval: ptr(200 * time.Minute),
		NextRotateTime:      ptr(now.Add(150 * time.Minute)),
		Config:              addedConfig,
	}).Return("backend-uuid", nil)
	s.mockRegistry.EXPECT().NewBackend(&provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
			Config:      addedConfig,
		},
	}).Return(s.mockSecretProvider, nil)
	s.mockRegistry.EXPECT().Type().Return("vault").AnyTimes()
	s.mockSecretProvider.EXPECT().Ping().Return(nil)

	err := svc.CreateSecretBackend(context.Background(), coresecrets.SecretBackend{
		ID:                  "backend-uuid",
		Name:                "myvault",
		BackendType:         vault.BackendType,
		TokenRotateInterval: ptr(200 * time.Minute),
		Config: map[string]interface{}{
			"endpoint": "http://vault",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}
func (s *serviceSuite) TestUpdateSecretBackendFailed(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	err := svc.UpdateSecretBackend(context.Background(), coresecrets.SecretBackend{}, false)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err, gc.ErrorMatches, "missing backend ID")

	err = svc.UpdateSecretBackend(context.Background(), coresecrets.SecretBackend{
		ID: "backend-uuid",
	}, false)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err, gc.ErrorMatches, "missing backend name")

	err = svc.UpdateSecretBackend(context.Background(), coresecrets.SecretBackend{
		ID:   "backend-uuid",
		Name: juju.BackendName,
	}, false)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err, gc.ErrorMatches, `backend "internal" not valid`)

	err = svc.UpdateSecretBackend(context.Background(), coresecrets.SecretBackend{
		ID:   "backend-uuid",
		Name: provider.Auto,
	}, false)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err, gc.ErrorMatches, `backend "auto" not valid`)

	s.mockState.EXPECT().GetSecretBackendByName(gomock.Any(), "invalid").Return(&coresecrets.SecretBackend{}, nil)
	s.mockRegistry.EXPECT().Type().Return("something").AnyTimes()
	err = svc.UpdateSecretBackend(context.Background(), coresecrets.SecretBackend{
		ID:          "backend-uuid",
		Name:        "invalid",
		BackendType: "something",
	}, false)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(errors.Cause(err), gc.ErrorMatches, `config for provider "something" not valid`)
}

func (s *serviceSuite) assertUpdateSecretBackend(c *gc.C, force bool) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, gc.Equals, "vault")
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	updatedConfig := map[string]interface{}{
		"endpoint":        "http://vault",
		"namespace":       "foo",
		"tls-server-name": "server-name",
	}
	now := time.Now()
	s.mockClock.EXPECT().Now().Return(now)
	s.mockState.EXPECT().GetSecretBackendByName(gomock.Any(), "myvault").Return(&coresecrets.SecretBackend{
		ID:          "backend-uuid",
		Name:        "myvault",
		BackendType: "vault",
		Config: map[string]interface{}{
			"endpoint": "http://vault",
		},
	}, nil)
	s.mockState.EXPECT().UpdateSecretBackend(gomock.Any(), secretbackend.UpdateSecretBackendParams{
		ID:                  "backend-uuid",
		NameChange:          ptr("myvault"),
		TokenRotateInterval: ptr(200 * time.Minute),
		NextRotateTime:      ptr(now.Add(150 * time.Minute)),
		Config:              updatedConfig,
	}).Return(nil)
	s.mockRegistry.EXPECT().Type().Return("vault").AnyTimes()
	if !force {
		s.mockRegistry.EXPECT().NewBackend(&provider.ModelBackendConfig{
			BackendConfig: provider.BackendConfig{
				BackendType: vault.BackendType,
				Config:      updatedConfig,
			},
		}).Return(s.mockSecretProvider, nil)
		s.mockSecretProvider.EXPECT().Ping().Return(nil)
	}
	err := svc.UpdateSecretBackend(context.Background(), coresecrets.SecretBackend{
		ID:                  "backend-uuid",
		Name:                "myvault",
		BackendType:         vault.BackendType,
		TokenRotateInterval: ptr(200 * time.Minute),
		Config: map[string]interface{}{
			"tls-server-name": "server-name",
		},
	}, force, "namespace")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateSecretBackend(c *gc.C) {
	s.assertUpdateSecretBackend(c, false)
}

func (s *serviceSuite) TestUpdateSecretBackendWithForce(c *gc.C) {
	s.assertUpdateSecretBackend(c, true)
}

func (s *serviceSuite) TestDeleteSecretBackend(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)
	s.mockState.EXPECT().DeleteSecretBackend(gomock.Any(), "backend-uuid", false).Return(nil)
	err := svc.DeleteSecretBackend(context.Background(), "backend-uuid", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGetSecretBackendByName(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)
	s.mockState.EXPECT().GetSecretBackendByName(gomock.Any(), "myvault").Return(&coresecrets.SecretBackend{
		ID:   "backend-uuid",
		Name: "myvault",
	}, nil)
	result, err := svc.GetSecretBackendByName(context.Background(), "myvault")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, &coresecrets.SecretBackend{
		ID:   "backend-uuid",
		Name: "myvault",
	})
}

func (s *serviceSuite) TestRotateBackendToken(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), "backend-uuid").Return(&coresecrets.SecretBackend{
		ID:                  "backend-uuid",
		Name:                "myvault",
		BackendType:         vault.BackendType,
		TokenRotateInterval: ptr(200 * time.Minute),
		Config: map[string]interface{}{
			"endpoint": "http://vault",
		},
	}, nil)
	s.mockState.EXPECT().UpdateSecretBackend(gomock.Any(), secretbackend.UpdateSecretBackendParams{
		ID: "backend-uuid",
		Config: map[string]interface{}{
			"endpoint": "http://vault",
			"token":    "3h20m0s",
		},
	}).Return(nil)

	now := time.Now()
	s.mockClock.EXPECT().Now().Return(now)
	nextRotateTime := now.Add(150 * time.Minute)
	s.mockState.EXPECT().SecretBackendRotated(gomock.Any(), "backend-uuid", nextRotateTime).Return(nil)

	err := svc.RotateBackendToken(context.Background(), "backend-uuid")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRotateBackendTokenRetry(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), "backend-uuid").Return(&coresecrets.SecretBackend{
		ID:                  "backend-uuid",
		Name:                "myvault",
		BackendType:         vault.BackendType,
		TokenRotateInterval: ptr(200 * time.Minute),
		Config: map[string]interface{}{
			"endpoint": "http://vault",
		},
	}, nil)
	s.mockState.EXPECT().UpdateSecretBackend(gomock.Any(), secretbackend.UpdateSecretBackendParams{
		ID: "backend-uuid",
		Config: map[string]interface{}{
			"endpoint": "http://vault",
			"token":    "3h20m0s",
		},
	}).Return(errors.New("BOOM"))

	now := time.Now()
	s.mockClock.EXPECT().Now().Return(now)
	// On error, try again after a short time.
	nextRotateTime := now.Add(2 * time.Minute)
	s.mockState.EXPECT().SecretBackendRotated(gomock.Any(), "backend-uuid", nextRotateTime).Return(nil)

	err := svc.RotateBackendToken(context.Background(), "backend-uuid")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestWatchSecretBackendRotationChanges(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewWatchableService(
		s.mockState, s.logger, s.mockWatcherFactory, jujutesting.ControllerTag.Id(), s.mockClock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)
	s.mockState.EXPECT().WatchSecretBackendRotationChanges(s.mockWatcherFactory).
		Return(s.mockSecretBackendRotateWatcher, nil)
	w, err := svc.WatchSecretBackendRotationChanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.Equals, s.mockSecretBackendRotateWatcher)
}
