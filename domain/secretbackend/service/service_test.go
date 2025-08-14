// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/schema"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	modelerrors "github.com/juju/juju/domain/model/errors"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/domain/secretbackend"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/configschema"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/secrets/provider/vault"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

func ptr[T any](v T) *T {
	return &v
}

type providerWithConfig struct {
	provider.ProviderConfig
	provider.SupportAuthRefresh
	provider.SecretBackendProvider
}

func (providerWithConfig) ConfigSchema() configschema.Fields {
	return configschema.Fields{
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

func (p providerWithConfig) ValidateConfig(oldCfg, newCfg provider.ConfigAttrs, tokenRotate *time.Duration) error {
	if p.Type() == "something" {
		return errors.Errorf("bad config for %q", p.Type())
	}
	return nil
}

func (providerWithConfig) RefreshAuth(_ context.Context, cfg provider.BackendConfig, validFor time.Duration) (*provider.BackendConfig, error) {
	result := cfg
	result.Config["token"] = validFor.String()
	return &result, nil
}

var (
	jujuBackendID  = utils.MustNewUUID().String()
	k8sBackendID   = utils.MustNewUUID().String()
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
				"endpoint":  "http://nowhere",
				"namespace": "my-model",
				"ca-certs":  []string{"cert-data"},
				"token":     "deadbeaf",
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
	testhelpers.IsolationSuite

	mockState                                     *MockState
	mockWatcherFactory                            *MockWatcherFactory
	mockRegistry                                  *MockSecretBackendProvider
	mockSecretProvider, mockSepicalSecretProvider *MockSecretsBackend
	mockStringWatcher                             *MockStringsWatcher

	clock  testclock.AdvanceableClock
	logger logger.Logger
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockState = NewMockState(ctrl)
	s.mockWatcherFactory = NewMockWatcherFactory(ctrl)
	s.mockRegistry = NewMockSecretBackendProvider(ctrl)
	s.mockSecretProvider = NewMockSecretsBackend(ctrl)
	s.mockSepicalSecretProvider = NewMockSecretsBackend(ctrl)
	s.mockStringWatcher = NewMockStringsWatcher(ctrl)

	s.clock = testclock.NewDilatedWallClock(0)
	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}

func (s *serviceSuite) expectGetSecretBackendConfigForAdminDefault(
	modelType string, modelBackend secretbackend.BackendIdentifier, backends ...*secretbackend.SecretBackend,
) {
	modelUUID := coremodel.UUID(jujutesting.ModelTag.Id())
	var builtIn []*secretbackend.SecretBackend

	if modelType == "caas" {
		builtIn = []*secretbackend.SecretBackend{{
			ID:          k8sBackendID,
			Name:        kubernetes.BackendName,
			BackendType: kubernetes.BackendType,
			Config: map[string]any{
				"namespace": "my-model",
				"ca-certs":  "[cert-data]",
				"token":     "deadbeaf",
				"endpoint":  "http://nowhere",
			},
		}}

	} else {
		builtIn = []*secretbackend.SecretBackend{{
			ID:          jujuBackendID,
			Name:        juju.BackendName,
			BackendType: juju.BackendType,
		}}
	}

	s.mockState.EXPECT().ListSecretBackendsForModel(gomock.Any(), modelUUID, true).Return(append(builtIn, backends...), nil)
	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).
		Return(secretbackend.ModelSecretBackend{
			ControllerUUID:    jujutesting.ControllerTag.Id(),
			ModelID:           modelUUID,
			ModelName:         "fred",
			ModelType:         coremodel.ModelType(modelType),
			SecretBackendID:   modelBackend.ID,
			SecretBackendName: modelBackend.Name,
		}, nil)
}

func (s *serviceSuite) TestGetSecretBackendConfigForAdmin(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, tc.Equals, "vault")
			return s.mockRegistry, nil
		},
	)

	modelUUID := coremodel.UUID(jujutesting.ModelTag.Id())
	s.mockState.EXPECT().ListSecretBackendsForModel(gomock.Any(), modelUUID, true).Return([]*secretbackend.SecretBackend{
		{
			ID:          jujuBackendID,
			Name:        juju.BackendName,
			BackendType: juju.BackendType,
		},
		{
			ID:          vaultBackendID,
			Name:        "myvault",
			BackendType: "vault",
			Config: map[string]any{
				"endpoint": "http://vault",
			},
		},
		{
			ID:          k8sBackendID,
			Name:        kubernetes.BackendName,
			BackendType: kubernetes.BackendType,
			Config: map[string]any{
				"namespace": "my-model",
				"ca-certs":  []string{"cert-data"},
				"token":     "deadbeaf",
				"endpoint":  "http://nowhere",
			},
		},
	}, nil)
	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).
		Return(secretbackend.ModelSecretBackend{
			ControllerUUID:    jujutesting.ControllerTag.Id(),
			ModelID:           modelUUID,
			ModelName:         "fred",
			ModelType:         coremodel.CAAS,
			SecretBackendID:   vaultBackendID,
			SecretBackendName: "myvault",
		}, nil)

	info, err := svc.GetSecretBackendConfigForAdmin(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, &provider.ModelBackendConfigInfo{
		ActiveID: vaultBackendID,
		Configs: map[string]provider.ModelBackendConfig{
			jujuBackendID:  jujuBackendConfig,
			k8sBackendID:   k8sBackendConfig,
			vaultBackendID: vaultBackendConfig,
		},
	})
}

func (s *serviceSuite) TestGetSecretBackendConfigForAdminFailedNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, tc.Equals, "vault")
			return s.mockRegistry, nil
		},
	)

	modelUUID := coremodel.UUID(jujutesting.ModelTag.Id())
	s.mockState.EXPECT().ListSecretBackendsForModel(gomock.Any(), modelUUID, true).Return([]*secretbackend.SecretBackend{
		{
			ID:          k8sBackendID,
			Name:        kubernetes.BackendName,
			BackendType: kubernetes.BackendType,
			Config: map[string]any{
				"namespace": "my-model",
				"ca-certs":  "[cert-data]",
				"token":     "deadbeaf",
				"endpoint":  "http://nowhere",
			},
		},
	}, nil)
	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).
		Return(secretbackend.ModelSecretBackend{
			ControllerUUID:    jujutesting.ControllerTag.Id(),
			ModelID:           modelUUID,
			ModelName:         "fred",
			ModelType:         coremodel.CAAS,
			SecretBackendID:   vaultBackendID,
			SecretBackendName: "myvault",
		}, nil)

	_, err := svc.GetSecretBackendConfigForAdmin(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIs, secretbackenderrors.NotFound)
}

func (s *serviceSuite) TestBackendSummaryInfoForModel(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			if backendType != vault.BackendType {
				return s.mockRegistry, nil
			}
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)
	modelUUID := coremodel.GenUUID(c)
	s.mockState.EXPECT().ListSecretBackendsForModel(gomock.Any(), modelUUID, false).Return([]*secretbackend.SecretBackend{
		{
			ID:          vaultBackendID,
			Name:        "myvault",
			BackendType: vault.BackendType,
			Config: map[string]any{
				"endpoint": "http://vault",
				"token":    "deadbeef",
			},
			NumSecrets: 1,
		},
		{
			ID:          "another-vault-id",
			Name:        "another-vault",
			BackendType: vault.BackendType,
			Config: map[string]any{
				"endpoint": "http://another-vault",
			},
			NumSecrets: 2,
		},
		{
			ID:          k8sBackendID,
			Name:        "my-model-local",
			BackendType: kubernetes.BackendType,
			Config: map[string]any{
				"endpoint":  "http://nowhere",
				"namespace": "my-model",
				"token":     "deadbeef",
			},
			NumSecrets: 3,
		},
	}, nil)
	s.mockRegistry.EXPECT().Type().Return(vault.BackendType).AnyTimes()
	s.mockRegistry.EXPECT().NewBackend(&provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
			Config: provider.ConfigAttrs{
				"endpoint": "http://another-vault",
			},
		},
	}).DoAndReturn(func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		s.mockSepicalSecretProvider.EXPECT().Ping().Return(errors.New("boom")).Times(1)
		return s.mockSepicalSecretProvider, nil
	})
	s.mockRegistry.EXPECT().NewBackend(&provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
			Config: provider.ConfigAttrs{
				"endpoint": "http://vault",
				"token":    "deadbeef",
			},
		},
	}).DoAndReturn(func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		s.mockSecretProvider.EXPECT().Ping().Return(nil).Times(1)
		return s.mockSecretProvider, nil
	})

	info, err := svc.BackendSummaryInfoForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.SameContents, []*SecretBackendInfo{
		{
			SecretBackend: coresecrets.SecretBackend{
				ID:          "another-vault-id",
				Name:        "another-vault",
				BackendType: vault.BackendType,
				Config: map[string]interface{}{
					"endpoint": "http://another-vault",
				},
			},
			NumSecrets: 2,
			Status:     "error",
			Message:    "boom",
		},
		{
			SecretBackend: coresecrets.SecretBackend{
				ID:          k8sBackendID,
				Name:        "my-model-local",
				BackendType: kubernetes.BackendType,
				Config: map[string]any{
					"endpoint":  "http://nowhere",
					"namespace": "my-model",
					"token":     "deadbeef",
				},
			},
			NumSecrets: 3,
			Status:     "active",
		},
		{
			SecretBackend: coresecrets.SecretBackend{
				ID:          "vault-backend-id",
				Name:        "myvault",
				BackendType: vault.BackendType,
				Config: map[string]interface{}{
					"endpoint": "http://vault",
				},
			},
			NumSecrets: 1,
			Status:     "active",
		},
	})
}

func (s *serviceSuite) assertBackendSummaryInfo(
	c *tc.C, svc *Service, modelType coremodel.ModelType,
	reveal bool, names []string,
	expected []*SecretBackendInfo,
) {
	backends := []*secretbackend.SecretBackend{
		{
			ID:          vaultBackendID,
			Name:        "myvault",
			BackendType: vault.BackendType,
			Config: map[string]any{
				"endpoint": "http://vault",
				"token":    "deadbeef",
			},
			NumSecrets: 1,
		},
		{
			ID:          "another-vault-id",
			Name:        "another-vault",
			BackendType: vault.BackendType,
			Config: map[string]any{
				"endpoint": "http://another-vault",
			},
			NumSecrets: 2,
		},
	}
	if modelType == coremodel.CAAS {
		backends = append(backends, &secretbackend.SecretBackend{
			ID:          k8sBackendID,
			Name:        "my-model-local",
			BackendType: kubernetes.BackendType,
			Config: map[string]any{
				"endpoint":  "http://nowhere",
				"namespace": "my-model",
				"token":     "deadbeef",
			},
			NumSecrets: 3,
		})
	} else {
		backends = append(backends, &secretbackend.SecretBackend{
			ID:          jujuBackendID,
			Name:        juju.BackendName,
			BackendType: juju.BackendType,
		})
	}
	s.mockState.EXPECT().ListSecretBackends(gomock.Any()).Return(backends, nil)
	s.mockRegistry.EXPECT().Type().Return(vault.BackendType).AnyTimes()
	if set.NewStrings(names...).Contains("myvault") || len(names) == 0 {
		s.mockRegistry.EXPECT().NewBackend(&provider.ModelBackendConfig{
			BackendConfig: provider.BackendConfig{
				BackendType: vault.BackendType,
				Config: provider.ConfigAttrs{
					"endpoint": "http://vault",
					"token":    "deadbeef",
				},
			},
		}).DoAndReturn(func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			s.mockSecretProvider.EXPECT().Ping().Return(nil).Times(1)
			return s.mockSecretProvider, nil
		})
	}
	if set.NewStrings(names...).Contains("another-vault") || len(names) == 0 {
		s.mockRegistry.EXPECT().NewBackend(&provider.ModelBackendConfig{
			BackendConfig: provider.BackendConfig{
				BackendType: vault.BackendType,
				Config: provider.ConfigAttrs{
					"endpoint": "http://another-vault",
				},
			},
		}).DoAndReturn(func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			s.mockSepicalSecretProvider.EXPECT().Ping().Return(errors.New("boom")).Times(1)
			return s.mockSepicalSecretProvider, nil
		})
	}

	info, err := svc.BackendSummaryInfo(c.Context(), reveal, names...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.SameContents, expected)
}

func (s *serviceSuite) TestBackendSummaryInfoWithFilterAllCAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, s.clock,
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
		nil,
		[]*SecretBackendInfo{
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          "another-vault-id",
					Name:        "another-vault",
					BackendType: vault.BackendType,
					Config: map[string]interface{}{
						"endpoint": "http://another-vault",
					},
				},
				NumSecrets: 2,
				Status:     "error",
				Message:    "boom",
			},
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          k8sBackendID,
					Name:        "my-model-local",
					BackendType: kubernetes.BackendType,
					Config: map[string]any{
						"endpoint":  "http://nowhere",
						"namespace": "my-model",
						"token":     "deadbeef",
					},
				},
				NumSecrets: 3,
				Status:     "active",
			},
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          "vault-backend-id",
					Name:        "myvault",
					BackendType: vault.BackendType,
					Config: map[string]interface{}{
						"endpoint": "http://vault",
					},
				},
				NumSecrets: 1,
				Status:     "active",
			},
		},
	)
}

func (s *serviceSuite) TestBackendSummaryInfoWithFilterAllIAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, s.clock,
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
		nil,
		[]*SecretBackendInfo{
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          "another-vault-id",
					Name:        "another-vault",
					BackendType: vault.BackendType,
					Config: map[string]interface{}{
						"endpoint": "http://another-vault",
					},
				},
				NumSecrets: 2,
				Status:     "error",
				Message:    "boom",
			},
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          jujuBackendID,
					Name:        juju.BackendName,
					BackendType: juju.BackendType,
				},
				Status: "active",
			},
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          "vault-backend-id",
					Name:        "myvault",
					BackendType: vault.BackendType,
					Config: map[string]interface{}{
						"endpoint": "http://vault",
					},
				},
				NumSecrets: 1,
				Status:     "active",
			},
		},
	)
}

func (s *serviceSuite) TestBackendSummaryInfoWithFilterNames(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, s.clock,
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
		[]string{"another-vault"},
		[]*SecretBackendInfo{
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          "another-vault-id",
					Name:        "another-vault",
					BackendType: vault.BackendType,
					Config: map[string]interface{}{
						"endpoint": "http://another-vault",
					},
				},
				NumSecrets: 2,
				Status:     "error",
				Message:    "boom",
			},
		},
	)
}

func (s *serviceSuite) TestBackendSummaryInfoWithFilterNamesNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, s.clock,
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
		[]string{"non-existing-vault"},
		[]*SecretBackendInfo{},
	)
}

func (s *serviceSuite) TestBackendConfigInfoLeaderUnit(c *tc.C) {
	s.assertBackendConfigInfoLeaderUnit(c, []string{"backend-id"})
}

func (s *serviceSuite) TestBackendConfigInfoDefaultAdmin(c *tc.C) {
	s.assertBackendConfigInfoLeaderUnit(c, nil)
}

func (s *serviceSuite) assertBackendConfigInfoLeaderUnit(c *tc.C, wanted []string) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			if backendType != vault.BackendType {
				return s.mockRegistry, nil
			}
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	accessor := coresecrets.Accessor{
		Kind: coresecrets.UnitAccessor,
		ID:   "gitlab/0",
	}
	token := NewMockToken(ctrl)

	owned := []*coresecrets.SecretRevisionRef{
		{URI: &coresecrets.URI{ID: "owned-1"}, RevisionID: "owned-rev-1"},
		{URI: &coresecrets.URI{ID: "owned-1"}, RevisionID: "owned-rev-2"},
	}
	ownedRevs := map[string]set.Strings{
		"owned-1": set.NewStrings("owned-rev-1", "owned-rev-2"),
	}
	read := []*coresecrets.SecretRevisionRef{
		{URI: &coresecrets.URI{ID: "read-1"}, RevisionID: "read-rev-1"},
	}
	readRevs := map[string]set.Strings{
		"read-1": set.NewStrings("read-rev-1"),
	}
	adminCfg := provider.ModelBackendConfig{
		ControllerUUID: jujutesting.ControllerTag.Id(),
		ModelUUID:      jujutesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
		},
	}
	backend := secretbackend.BackendIdentifier{
		ID:   "backend-id",
		Name: "backend1",
	}
	s.expectGetSecretBackendConfigForAdminDefault("iaas", backend, []*secretbackend.SecretBackend{{
		ID:          "backend-id",
		Name:        "backend1",
		BackendType: "some-backend",
	}, {
		ID:          "backend-id2",
		Name:        "backend2",
		BackendType: "some-backend2",
	}}...)
	s.mockRegistry.EXPECT().Initialise(gomock.Any()).Return(nil)
	token.EXPECT().Check().Return(nil)

	s.mockRegistry.EXPECT().RestrictedConfig(gomock.Any(), &adminCfg, false, false, accessor, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil)

	listGranted := func(
		ctx context.Context, backendID string, role coresecrets.SecretRole, consumers ...secretservice.SecretAccessor,
	) ([]*coresecrets.SecretRevisionRef, error) {
		c.Assert(backendID, tc.Equals, "backend-id")
		if role == coresecrets.RoleManage {
			c.Assert(consumers, tc.DeepEquals, []secretservice.SecretAccessor{{
				Kind: secretservice.UnitAccessor,
				ID:   "gitlab/0",
			}, {
				Kind: secretservice.ApplicationAccessor,
				ID:   "gitlab",
			}})
			return owned, nil
		}
		c.Assert(consumers, tc.DeepEquals, []secretservice.SecretAccessor{{
			Kind: secretservice.UnitAccessor,
			ID:   "gitlab/0",
		}, {
			Kind: secretservice.ApplicationAccessor,
			ID:   "gitlab",
		}})
		return read, nil
	}
	info, err := svc.BackendConfigInfo(c.Context(), BackendConfigParams{
		GrantedSecretsGetter: listGranted,
		LeaderToken:          token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   accessor.ID,
		},
		ModelUUID:      coremodel.UUID(jujutesting.ModelTag.Id()),
		BackendIDs:     wanted,
		SameController: false,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, &provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: jujutesting.ControllerTag.Id(),
				ModelUUID:      jujutesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
				},
			},
		},
	})
}

func (s *serviceSuite) TestBackendConfigInfoNonLeaderUnit(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			if backendType != vault.BackendType {
				return s.mockRegistry, nil
			}
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	accessor := coresecrets.Accessor{
		Kind: coresecrets.UnitAccessor,
		ID:   "gitlab/0",
	}
	token := NewMockToken(ctrl)

	unitOwned := []*coresecrets.SecretRevisionRef{
		{URI: &coresecrets.URI{ID: "owned-1"}, RevisionID: "owned-rev-1"},
		{URI: &coresecrets.URI{ID: "owned-1"}, RevisionID: "owned-rev-2"},
	}
	appOwned := []*coresecrets.SecretRevisionRef{
		{URI: &coresecrets.URI{ID: "app-owned-1"}, RevisionID: "app-owned-rev-1"},
		{URI: &coresecrets.URI{ID: "app-owned-1"}, RevisionID: "app-owned-rev-2"},
		{URI: &coresecrets.URI{ID: "app-owned-1"}, RevisionID: "app-owned-rev-3"},
	}
	ownedRevs := map[string]set.Strings{
		"owned-1": set.NewStrings("owned-rev-1", "owned-rev-2"),
	}
	read := []*coresecrets.SecretRevisionRef{
		{URI: &coresecrets.URI{ID: "read-1"}, RevisionID: "read-rev-1"},
	}
	readRevs := map[string]set.Strings{
		"read-1":      set.NewStrings("read-rev-1"),
		"app-owned-1": set.NewStrings("app-owned-rev-1", "app-owned-rev-2", "app-owned-rev-3"),
	}
	adminCfg := provider.ModelBackendConfig{
		ControllerUUID: jujutesting.ControllerTag.Id(),
		ModelUUID:      jujutesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
		},
	}
	backend := secretbackend.BackendIdentifier{
		ID:   "backend-id",
		Name: "backend1",
	}
	s.expectGetSecretBackendConfigForAdminDefault("iaas", backend, []*secretbackend.SecretBackend{{
		ID:          "backend-id",
		Name:        "backend1",
		BackendType: "some-backend",
	}, {
		ID:          "backend-id2",
		Name:        "backend2",
		BackendType: "some-backend2",
	}}...)
	s.mockRegistry.EXPECT().Initialise(gomock.Any()).Return(nil)
	token.EXPECT().Check().Return(leadership.NewNotLeaderError("", ""))

	s.mockRegistry.EXPECT().RestrictedConfig(gomock.Any(), &adminCfg, true, false, accessor, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil)

	listGranted := func(
		ctx context.Context, backendID string, role coresecrets.SecretRole, consumers ...secretservice.SecretAccessor,
	) ([]*coresecrets.SecretRevisionRef, error) {
		c.Assert(backendID, tc.Equals, "backend-id")
		if role == coresecrets.RoleManage {
			c.Assert(consumers, tc.DeepEquals, []secretservice.SecretAccessor{{
				Kind: secretservice.UnitAccessor,
				ID:   "gitlab/0",
			}})
			return unitOwned, nil
		}
		if len(consumers) == 1 && consumers[0].Kind == secretservice.ApplicationAccessor && consumers[0].ID == "gitlab" {
			return appOwned, nil
		}
		c.Assert(consumers, tc.DeepEquals, []secretservice.SecretAccessor{{
			Kind: secretservice.UnitAccessor,
			ID:   "gitlab/0",
		}, {
			Kind: secretservice.ApplicationAccessor,
			ID:   "gitlab",
		}})
		return read, nil
	}
	info, err := svc.BackendConfigInfo(c.Context(), BackendConfigParams{
		GrantedSecretsGetter: listGranted,
		LeaderToken:          token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "gitlab/0",
		},
		ModelUUID:      coremodel.UUID(jujutesting.ModelTag.Id()),
		BackendIDs:     []string{"backend-id"},
		SameController: true,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, &provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: jujutesting.ControllerTag.Id(),
				ModelUUID:      jujutesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
				},
			},
		},
	})
}

func (s *serviceSuite) TestDrainBackendConfigInfo(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			if backendType != vault.BackendType {
				return s.mockRegistry, nil
			}
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	accessor := coresecrets.Accessor{
		Kind: coresecrets.UnitAccessor,
		ID:   "gitlab/0",
	}
	token := NewMockToken(ctrl)

	unitOwned := []*coresecrets.SecretRevisionRef{
		{URI: &coresecrets.URI{ID: "owned-1"}, RevisionID: "owned-rev-1"},
		{URI: &coresecrets.URI{ID: "owned-1"}, RevisionID: "owned-rev-2"},
	}
	appOwned := []*coresecrets.SecretRevisionRef{
		{URI: &coresecrets.URI{ID: "app-owned-1"}, RevisionID: "app-owned-rev-1"},
		{URI: &coresecrets.URI{ID: "app-owned-1"}, RevisionID: "app-owned-rev-2"},
		{URI: &coresecrets.URI{ID: "app-owned-1"}, RevisionID: "app-owned-rev-3"},
	}
	ownedRevs := map[string]set.Strings{
		"owned-1": set.NewStrings("owned-rev-1", "owned-rev-2"),
	}
	read := []*coresecrets.SecretRevisionRef{
		{URI: &coresecrets.URI{ID: "read-1"}, RevisionID: "read-rev-1"},
	}
	readRevs := map[string]set.Strings{
		"read-1":      set.NewStrings("read-rev-1"),
		"app-owned-1": set.NewStrings("app-owned-rev-1", "app-owned-rev-2", "app-owned-rev-3"),
	}
	adminCfg := provider.ModelBackendConfig{
		ControllerUUID: jujutesting.ControllerTag.Id(),
		ModelUUID:      jujutesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
		},
	}
	backend := secretbackend.BackendIdentifier{
		ID:   "backend-id",
		Name: "backend1",
	}
	s.expectGetSecretBackendConfigForAdminDefault("iaas", backend, []*secretbackend.SecretBackend{{
		ID:          "backend-id",
		Name:        "backend1",
		BackendType: "some-backend",
	}, {
		ID:          "backend-id2",
		Name:        "backend2",
		BackendType: "some-backend2",
	}}...)
	s.mockRegistry.EXPECT().Initialise(gomock.Any()).Return(nil)
	token.EXPECT().Check().Return(leadership.NewNotLeaderError("", ""))

	s.mockRegistry.EXPECT().RestrictedConfig(gomock.Any(), &adminCfg, true, true, accessor, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil)

	listGranted := func(
		ctx context.Context, backendID string, role coresecrets.SecretRole, consumers ...secretservice.SecretAccessor,
	) ([]*coresecrets.SecretRevisionRef, error) {
		c.Assert(backendID, tc.Equals, "backend-id")
		if role == coresecrets.RoleManage {
			c.Assert(consumers, tc.DeepEquals, []secretservice.SecretAccessor{{
				Kind: secretservice.UnitAccessor,
				ID:   "gitlab/0",
			}})
			return unitOwned, nil
		}
		if len(consumers) == 1 && consumers[0].Kind == secretservice.ApplicationAccessor && consumers[0].ID == "gitlab" {
			return appOwned, nil
		}
		c.Assert(consumers, tc.DeepEquals, []secretservice.SecretAccessor{{
			Kind: secretservice.UnitAccessor,
			ID:   "gitlab/0",
		}, {
			Kind: secretservice.ApplicationAccessor,
			ID:   "gitlab",
		}})
		return read, nil
	}
	info, err := svc.DrainBackendConfigInfo(c.Context(), DrainBackendConfigParams{
		GrantedSecretsGetter: listGranted,
		LeaderToken:          token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "gitlab/0",
		},
		ModelUUID: coremodel.UUID(jujutesting.ModelTag.Id()),
		BackendID: "backend-id",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, &provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: jujutesting.ControllerTag.Id(),
				ModelUUID:      jujutesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
				},
			},
		},
	})
}

func (s *serviceSuite) TestBackendConfigInfoFailedInvalidAccessor(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			if backendType != vault.BackendType {
				return s.mockRegistry, nil
			}
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	_, err := svc.BackendConfigInfo(c.Context(), BackendConfigParams{
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.ApplicationAccessor,
			ID:   "someapp",
		},
		ModelUUID:  coremodel.UUID(jujutesting.ModelTag.Id()),
		BackendIDs: []string{"backend-id"},
	})
	c.Assert(err, tc.ErrorMatches, `secret accessor kind "application" not supported`)
}

func (s *serviceSuite) TestBackendIDs(c *tc.C) {
	defer s.setupMocks(c).Finish()
	backends := []string{vaultBackendID, "another-vault-id"}
	s.mockState.EXPECT().ListSecretBackendIDs(gomock.Any()).Return(backends, nil)

	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			if backendType != vault.BackendType {
				return s.mockRegistry, nil
			}
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)
	result, err := svc.ListBackendIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []string{vaultBackendID, "another-vault-id"})
}

func (s *serviceSuite) TestCreateSecretBackendFailed(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, tc.Equals, "something")
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	err := svc.CreateSecretBackend(c.Context(), coresecrets.SecretBackend{})
	c.Check(err, tc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, "secret backend not valid: missing ID")

	err = svc.CreateSecretBackend(c.Context(), coresecrets.SecretBackend{
		ID: "backend-uuid",
	})
	c.Check(err, tc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, "secret backend not valid: missing name")

	err = svc.CreateSecretBackend(c.Context(), coresecrets.SecretBackend{
		ID:   "backend-uuid",
		Name: juju.BackendName,
	})
	c.Check(err, tc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: reserved name "internal"`)

	err = svc.CreateSecretBackend(c.Context(), coresecrets.SecretBackend{
		ID:   "backend-uuid",
		Name: provider.Auto,
	})
	c.Check(err, tc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: reserved name "auto"`)

	s.mockRegistry.EXPECT().Type().Return("something").AnyTimes()
	err = svc.CreateSecretBackend(c.Context(), coresecrets.SecretBackend{
		ID:          "backend-uuid",
		Name:        "invalid",
		BackendType: "something",
	})
	c.Check(err, tc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: config for provider "something": bad config for "something"`)
}

func (s *serviceSuite) TestCreateSecretBackend(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, tc.Equals, "vault")
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	addedConfig := map[string]interface{}{
		"endpoint":  "http://vault",
		"namespace": "foo",
	}
	now := s.clock.Now()
	s.mockState.EXPECT().CreateSecretBackend(gomock.Any(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   "backend-uuid",
			Name: "myvault",
		},
		BackendType:         vault.BackendType,
		TokenRotateInterval: ptr(200 * time.Minute),
		NextRotateTime:      ptr(now.Add(150 * time.Minute)),
		Config:              convertConfigToString(addedConfig),
	}).Return("backend-uuid", nil)
	s.mockRegistry.EXPECT().NewBackend(&provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
			Config:      addedConfig,
		},
	}).Return(s.mockSecretProvider, nil)
	s.mockRegistry.EXPECT().Type().Return("vault").AnyTimes()
	s.mockSecretProvider.EXPECT().Ping().Return(nil)

	err := svc.CreateSecretBackend(c.Context(), coresecrets.SecretBackend{
		ID:                  "backend-uuid",
		Name:                "myvault",
		BackendType:         vault.BackendType,
		TokenRotateInterval: ptr(200 * time.Minute),
		Config: map[string]interface{}{
			"endpoint": "http://vault",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}
func (s *serviceSuite) TestUpdateSecretBackendFailed(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	arg := UpdateSecretBackendParams{}
	err := svc.UpdateSecretBackend(c.Context(), arg)
	c.Check(err, tc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, "secret backend not valid: both ID and name are missing")

	arg.ID = "backend-uuid"
	arg.NewName = ptr(juju.BackendName)
	err = svc.UpdateSecretBackend(c.Context(), arg)
	c.Check(err, tc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: reserved name "internal"`)

	arg.NewName = ptr(provider.Auto)
	err = svc.UpdateSecretBackend(c.Context(), arg)
	c.Check(err, tc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: reserved name "auto"`)

	arg = UpdateSecretBackendParams{}
	arg.ID = "backend-uuid"
	arg.Name = "myvault"
	err = svc.UpdateSecretBackend(c.Context(), arg)
	c.Check(err, tc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: both ID and name are set`)

	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{ID: "backend-uuid"}).
		Return(&secretbackend.SecretBackend{
			BackendType: "something",
		}, nil)
	s.mockRegistry.EXPECT().Type().Return("something").AnyTimes()
	arg = UpdateSecretBackendParams{}
	arg.ID = "backend-uuid"
	err = svc.UpdateSecretBackend(c.Context(), arg)
	c.Check(err, tc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: config for provider "something": bad config for "something"`)
}

func (s *serviceSuite) assertUpdateSecretBackend(c *tc.C, byName, skipPing bool) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, tc.Equals, "vault")
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)
	identifier := secretbackend.BackendIdentifier{ID: "backend-uuid"}
	if byName {
		identifier = secretbackend.BackendIdentifier{Name: "myvault"}
	}

	updatedConfig := map[string]interface{}{
		"endpoint":        "http://vault",
		"namespace":       "foo",
		"tls-server-name": "server-name",
	}
	now := s.clock.Now()
	if byName {
		s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{Name: "myvault"}).Return(&secretbackend.SecretBackend{
			ID:          "backend-uuid",
			Name:        "myvault",
			BackendType: "vault",
			Config: map[string]any{
				"endpoint": "http://vault",
			},
		}, nil)
	} else {
		s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{ID: "backend-uuid"}).Return(&secretbackend.SecretBackend{
			ID:          "backend-uuid",
			Name:        "myvault",
			BackendType: "vault",
			Config: map[string]any{
				"endpoint": "http://vault",
			},
		}, nil)
	}
	s.mockState.EXPECT().UpdateSecretBackend(gomock.Any(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier:   identifier,
		NewName:             ptr("new-name"),
		TokenRotateInterval: ptr(200 * time.Minute),
		NextRotateTime:      ptr(now.Add(150 * time.Minute)),
		Config:              convertConfigToString(updatedConfig),
	}).Return("", nil)
	s.mockRegistry.EXPECT().Type().Return("vault").AnyTimes()
	if !skipPing {
		s.mockRegistry.EXPECT().NewBackend(&provider.ModelBackendConfig{
			BackendConfig: provider.BackendConfig{
				BackendType: vault.BackendType,
				Config:      updatedConfig,
			},
		}).Return(s.mockSecretProvider, nil)
		s.mockSecretProvider.EXPECT().Ping().Return(nil)
	}

	arg := UpdateSecretBackendParams{
		SkipPing: skipPing,
		Reset:    []string{"namespace"},
	}
	arg.BackendIdentifier = identifier
	arg.NewName = ptr("new-name")
	arg.TokenRotateInterval = ptr(200 * time.Minute)
	arg.Config = map[string]string{
		"tls-server-name": "server-name",
	}

	err := svc.UpdateSecretBackend(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateSecretBackend(c *tc.C) {
	s.assertUpdateSecretBackend(c, false, false)
}

func (s *serviceSuite) TestUpdateSecretBackendByName(c *tc.C) {
	s.assertUpdateSecretBackend(c, true, false)
}

func (s *serviceSuite) TestUpdateSecretBackendWithForce(c *tc.C) {
	s.assertUpdateSecretBackend(c, false, true)
}

func (s *serviceSuite) TestUpdateSecretBackendWithForceByName(c *tc.C) {
	s.assertUpdateSecretBackend(c, true, true)
}

func (s *serviceSuite) TestDeleteSecretBackend(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)
	s.mockState.EXPECT().DeleteSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{ID: "backend-uuid"}, false).Return(nil)
	err := svc.DeleteSecretBackend(c.Context(), DeleteSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{ID: "backend-uuid"},
		DeleteInUse:       false,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestRotateBackendToken(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{ID: "backend-uuid"}).Return(&secretbackend.SecretBackend{
		ID:                  "backend-uuid",
		Name:                "myvault",
		BackendType:         vault.BackendType,
		TokenRotateInterval: ptr(200 * time.Minute),
		Config: map[string]any{
			"endpoint": "http://vault",
		},
	}, nil)
	s.mockState.EXPECT().UpdateSecretBackend(gomock.Any(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: "backend-uuid",
		},
		Config: map[string]string{
			"endpoint": "http://vault",
			"token":    "3h20m0s",
		},
	}).Return("", nil)

	now := s.clock.Now()
	nextRotateTime := now.Add(150 * time.Minute)
	s.mockState.EXPECT().SecretBackendRotated(gomock.Any(), "backend-uuid", nextRotateTime).Return(nil)

	err := svc.RotateBackendToken(c.Context(), "backend-uuid")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestRotateBackendTokenRetry(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{ID: "backend-uuid"}).Return(&secretbackend.SecretBackend{
		ID:                  "backend-uuid",
		Name:                "myvault",
		BackendType:         vault.BackendType,
		TokenRotateInterval: ptr(200 * time.Minute),
		Config: map[string]any{
			"endpoint": "http://vault",
		},
	}, nil)
	s.mockState.EXPECT().UpdateSecretBackend(gomock.Any(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: "backend-uuid",
		},
		Config: map[string]string{
			"endpoint": "http://vault",
			"token":    "3h20m0s",
		},
	}).Return("", errors.New("BOOM"))

	now := s.clock.Now()
	// On error, try again after a short time.
	nextRotateTime := now.Add(2 * time.Minute)
	s.mockState.EXPECT().SecretBackendRotated(gomock.Any(), "backend-uuid", nextRotateTime).Return(nil)

	err := svc.RotateBackendToken(c.Context(), "backend-uuid")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestWatchSecretBackendRotationChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backendID1 := uuid.MustNewUUID().String()
	backendID2 := uuid.MustNewUUID().String()
	nextRotateTime1 := time.Now().Add(12 * time.Hour)
	nextRotateTime2 := time.Now().Add(24 * time.Hour)

	svc := newWatchableService(
		s.mockState, s.logger, s.mockWatcherFactory, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)
	ch := make(chan []string)
	s.mockStringWatcher.EXPECT().Changes().Return(ch).AnyTimes()
	s.mockStringWatcher.EXPECT().Wait().Return(nil).AnyTimes()
	s.mockStringWatcher.EXPECT().Kill().AnyTimes()

	s.mockState.EXPECT().InitialWatchStatementForSecretBackendRotationChanges().Return("table", "SELECT * FROM table")
	s.mockWatcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(s.mockStringWatcher, nil)
	s.mockState.EXPECT().GetSecretBackendRotateChanges(gomock.Any(), backendID1, backendID2).Return([]watcher.SecretBackendRotateChange{
		{
			ID:              backendID1,
			Name:            "my-backend1",
			NextTriggerTime: nextRotateTime1,
		},
		{
			ID:              backendID2,
			Name:            "my-backend2",
			NextTriggerTime: nextRotateTime2,
		},
	}, nil)

	w, err := svc.WatchSecretBackendRotationChanges(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
	defer workertest.CleanKill(c, w)

	wC := watchertest.NewSecretBackendRotateWatcherC(c, w)

	select {
	case ch <- []string{backendID1, backendID2}:
	case <-time.After(jujutesting.ShortWait):
		c.Fatalf("timed out waiting for sending the initial changes")
	}

	wC.AssertChanges(
		watcher.SecretBackendRotateChange{
			ID:              backendID1,
			Name:            "my-backend1",
			NextTriggerTime: nextRotateTime1,
		},
		watcher.SecretBackendRotateChange{
			ID:              backendID2,
			Name:            "my-backend2",
			NextTriggerTime: nextRotateTime2,
		},
	)
	wC.AssertNoChange()
}

func (s *serviceSuite) TestGetModelSecretBackendFailedModelNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := coremodel.GenUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).Return(secretbackend.ModelSecretBackend{}, modelerrors.NotFound)

	_, err := svc.GetModelSecretBackend(c.Context())
	c.Assert(err, tc.ErrorMatches, `getting model secret backend detail for "`+modelUUID.String()+`": model not found`)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *serviceSuite) TestGetModelSecretBackendCAAS(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := coremodel.GenUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).Return(secretbackend.ModelSecretBackend{
		SecretBackendName: "backend-name",
		ModelType:         coremodel.CAAS,
	}, nil)

	backendID, err := svc.GetModelSecretBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendID, tc.Equals, "backend-name")
}

func (s *serviceSuite) TestGetModelSecretBackendIAAS(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := coremodel.GenUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).Return(secretbackend.ModelSecretBackend{
		SecretBackendName: "backend-name",
		ModelType:         coremodel.IAAS,
	}, nil)

	backendID, err := svc.GetModelSecretBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendID, tc.Equals, "backend-name")
}

func (s *serviceSuite) TestGetModelSecretBackendCAASAuto(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := coremodel.GenUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).Return(secretbackend.ModelSecretBackend{
		SecretBackendName: "kubernetes",
		ModelType:         coremodel.CAAS,
	}, nil)

	backendID, err := svc.GetModelSecretBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendID, tc.Equals, "auto")
}

func (s *serviceSuite) TestGetModelSecretBackendIAASAuto(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := coremodel.GenUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).Return(secretbackend.ModelSecretBackend{
		SecretBackendName: "internal",
		ModelType:         coremodel.IAAS,
	}, nil)

	backendID, err := svc.GetModelSecretBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendID, tc.Equals, "auto")
}

func (s *serviceSuite) TestSetModelSecretBackendFailedEmptyBackendName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := coremodel.GenUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	err := svc.SetModelSecretBackend(c.Context(), "")
	c.Assert(err, tc.ErrorMatches, `missing backend name`)
}

func (s *serviceSuite) TestSetModelSecretBackendFailedReservedNameKubernetes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := coremodel.GenUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	err := svc.SetModelSecretBackend(c.Context(), "kubernetes")
	c.Assert(err, tc.ErrorMatches, `secret backend name "kubernetes" not valid`)
	c.Assert(err, tc.ErrorIs, secretbackenderrors.NotValid)
}

func (s *serviceSuite) TestSetModelSecretBackendFailedReservedNameInternal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := coremodel.GenUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	err := svc.SetModelSecretBackend(c.Context(), "internal")
	c.Assert(err, tc.ErrorMatches, `secret backend name "internal" not valid`)
	c.Assert(err, tc.ErrorIs, secretbackenderrors.NotValid)
}

func (s *serviceSuite) TestSetModelSecretBackendFailedUnkownModelType(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := coremodel.GenUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelType(gomock.Any(), modelUUID).Return("bad-type", nil)

	err := svc.SetModelSecretBackend(c.Context(), "auto")
	c.Assert(err, tc.ErrorMatches, `setting model secret backend for unsupported model type "bad-type" for model "`+modelUUID.String()+`"`)
}

func (s *serviceSuite) TestSetModelSecretBackendFailedModelNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := coremodel.GenUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelType(gomock.Any(), modelUUID).Return("", modelerrors.NotFound)

	err := svc.SetModelSecretBackend(c.Context(), "auto")
	c.Assert(err, tc.ErrorMatches, `getting model type for "`+modelUUID.String()+`": model not found`)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *serviceSuite) TestSetModelSecretBackendFailedSecretBackendNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := coremodel.GenUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelType(gomock.Any(), modelUUID).Return(coremodel.CAAS, nil)
	s.mockState.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "kubernetes").Return(secretbackenderrors.NotFound)

	err := svc.SetModelSecretBackend(c.Context(), "auto")
	c.Assert(err, tc.ErrorMatches, `setting model secret backend for "`+modelUUID.String()+`": secret backend not found`)
	c.Assert(err, tc.ErrorIs, secretbackenderrors.NotFound)
}

func (s *serviceSuite) TestSetModelSecretBackend(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := coremodel.GenUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "backend-name").Return(nil)

	err := svc.SetModelSecretBackend(c.Context(), "backend-name")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetModelSecretBackendCAASAuto(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := coremodel.GenUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelType(gomock.Any(), modelUUID).Return(coremodel.CAAS, nil)
	s.mockState.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "kubernetes").Return(nil)

	err := svc.SetModelSecretBackend(c.Context(), "auto")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetModelSecretBackendIAASAuto(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := coremodel.GenUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelType(gomock.Any(), modelUUID).Return(coremodel.IAAS, nil)
	s.mockState.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "internal").Return(nil)

	err := svc.SetModelSecretBackend(c.Context(), "auto")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestWatchModelSecretBackendChanged(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	svc := newWatchableService(
		s.mockState, s.logger, s.mockWatcherFactory, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)
	modelUUID := coremodel.UUID(jujutesting.ModelTag.Id())
	ch := make(chan struct{})
	go func() {
		// send the initial change.
		ch <- struct{}{}
		// send the 1st change.
		ch <- struct{}{}
	}()

	mockNotifyWatcher := NewMockNotifyWatcher(ctrl)
	mockNotifyWatcher.EXPECT().Changes().Return(ch).AnyTimes()
	mockNotifyWatcher.EXPECT().Wait().Return(nil).AnyTimes()
	mockNotifyWatcher.EXPECT().Kill().AnyTimes()

	s.mockState.EXPECT().NamespaceForWatchModelSecretBackend().Return("model_secret_backend")
	s.mockWatcherFactory.EXPECT().NewNotifyWatcher(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockNotifyWatcher, nil)

	w, err := svc.WatchModelSecretBackendChanged(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
	defer workertest.CleanKill(c, w)

	wc := watchertest.NewNotifyWatcherC(c, w)

	wc.AssertNChanges(2)
}

func (s *serviceSuite) assertGetSecretsToDrain(c *tc.C, backendID string, expectedRevisions ...RevisionInfo) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	svc := newService(
		s.mockState, s.logger, s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			if backendType != vault.BackendType {
				return s.mockRegistry, nil
			}
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	modelUUID := coremodel.UUID(jujutesting.ModelTag.Id())
	s.mockState.EXPECT().GetInternalAndActiveBackendUUIDs(gomock.Any(), modelUUID).Return(jujuBackendID, backendID, nil)

	revisions := []coresecrets.SecretExternalRevision{
		{
			// External backend.
			Revision: 666,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-666",
			},
		}, {
			// Internal backend.
			Revision: 667,
		},
		{
			// k8s backend.
			Revision: 668,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  k8sBackendID,
				RevisionID: "rev-668",
			},
		},
	}

	results, err := svc.GetRevisionsToDrain(c.Context(), modelUUID, revisions)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, expectedRevisions)
}

func (s *serviceSuite) TestGetRevisionsToDrainAutoIAAS(c *tc.C) {
	s.assertGetSecretsToDrain(c, jujuBackendID,
		// External backend.
		RevisionInfo{
			Revision: 666,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-666",
			},
		},
		// k8s backend.
		RevisionInfo{
			Revision: 668,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  k8sBackendID,
				RevisionID: "rev-668",
			},
		},
	)
}

func (s *serviceSuite) TestGetRevisionsToDrainAutoCAAS(c *tc.C) {
	s.assertGetSecretsToDrain(c, k8sBackendID,
		// External backend.
		RevisionInfo{
			Revision: 666,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-666",
			},
		},
		// Internal backend.
		RevisionInfo{
			Revision: 667,
		},
	)
}

func (s *serviceSuite) TestGetRevisionsToDrainExternal(c *tc.C) {
	s.assertGetSecretsToDrain(c, "backend-id",
		// Internal backend.
		RevisionInfo{
			Revision: 667,
		},
		// k8s backend.
		RevisionInfo{
			Revision: 668,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  k8sBackendID,
				RevisionID: "rev-668",
			},
		},
	)
}
