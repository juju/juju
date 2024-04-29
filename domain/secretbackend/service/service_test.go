// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	"github.com/juju/worker/v4/workertest"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/secretbackend"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/secrets/provider/vault"
	"github.com/juju/juju/internal/uuid"
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
		return fmt.Errorf("bad config for %q", p.Type())
	}
	return nil
}

func (providerWithConfig) RefreshAuth(cfg provider.BackendConfig, validFor time.Duration) (*provider.BackendConfig, error) {
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

	mockState                                     *MockState
	mockWatcherFactory                            *MockWatcherFactory
	mockRegistry                                  *MockSecretBackendProvider
	mockSecretProvider, mockSepicalSecretProvider *MockSecretsBackend
	mockStringWatcher                             *MockStringsWatcher

	clock  testclock.AdvanceableClock
	logger Logger
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockState = NewMockState(ctrl)
	s.mockWatcherFactory = NewMockWatcherFactory(ctrl)
	s.mockRegistry = NewMockSecretBackendProvider(ctrl)
	s.mockSecretProvider = NewMockSecretsBackend(ctrl)
	s.mockSepicalSecretProvider = NewMockSecretsBackend(ctrl)
	s.mockStringWatcher = NewMockStringsWatcher(ctrl)

	s.clock = testclock.NewDilatedWallClock(0)
	s.logger = jujutesting.NewCheckLogger(c)

	return ctrl
}

func (s *serviceSuite) assertGetSecretBackendConfigForAdminDefault(
	c *gc.C, svc *Service, modelType string, backendName string, expected *provider.ModelBackendConfigInfo,
) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := coremodel.UUID(jujutesting.ModelTag.Id())
	var builtIn []*secretbackend.SecretBackend

	if modelType == "caas" {
		cld := cloud.Cloud{
			Name:              "test",
			Type:              "kubernetes",
			Endpoint:          "http://nowhere",
			CACertificates:    []string{"cert-data"},
			IsControllerCloud: true,
		}
		cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{"foo": "bar"})
		builtIn = []*secretbackend.SecretBackend{{
			ID:          k8sBackendID,
			Name:        kubernetes.BackendName,
			BackendType: kubernetes.BackendType,
			Config: map[string]string{
				"ca-certs":            "[cert-data]",
				"credential":          `{"auth-type":"access-key","Attributes":{"foo":"bar"}}`,
				"endpoint":            "http://nowhere",
				"is-controller-cloud": "true",
			},
		}}
		s.mockState.EXPECT().GetControllerModelCloudAndCredential(gomock.Any()).Return(cld, cred, nil)
	} else {
		builtIn = []*secretbackend.SecretBackend{{
			ID:          jujuBackendID,
			Name:        juju.BackendName,
			BackendType: juju.BackendType,
		}}
	}

	s.mockState.EXPECT().ListSecretBackendsForModel(gomock.Any(), modelUUID, true).Return(append(builtIn, &secretbackend.SecretBackend{
		ID:          vaultBackendID,
		Name:        "myvault",
		BackendType: vault.BackendType,
		Config: map[string]string{
			"endpoint": "http://vault",
		},
	}), nil)
	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).
		Return(secretbackend.ModelSecretBackend{
			ID:              modelUUID,
			Name:            "fred",
			Type:            coremodel.ModelType(modelType),
			SecretBackendID: vaultBackendID,
		}, nil)
	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{ID: vaultBackendID}).
		Return(&secretbackend.SecretBackend{Name: backendName}, nil)

	info, err := svc.GetSecretBackendConfigForAdmin(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)
}

func (s *serviceSuite) TestGetSecretBackendConfigForAdminInternalIAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
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
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, gc.Equals, "vault")
			return s.mockRegistry, nil
		},
	)
	s.assertGetSecretBackendConfigForAdminDefault(c, svc, "caas", "kubernetes",
		&provider.ModelBackendConfigInfo{
			ActiveID: k8sBackendID,
			Configs: map[string]provider.ModelBackendConfig{
				k8sBackendID:   k8sBackendConfig,
				vaultBackendID: vaultBackendConfig,
			},
		},
	)
}

func (s *serviceSuite) TestGetSecretBackendConfigForAdminExternalIAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
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
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
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
				vaultBackendID: vaultBackendConfig,
			},
		},
	)
}

func (s *serviceSuite) TestGetSecretBackendConfig(c *gc.C) {
	c.Skip("TODO: wait for secret DqLite support")
}

func (s *serviceSuite) assertBackendSummaryInfo(
	c *gc.C, svc *Service, modelType coremodel.ModelType,
	reveal bool, names []string,
	expected []*SecretBackendInfo,
) {
	backends := []*secretbackend.SecretBackend{
		{
			ID:          vaultBackendID,
			Name:        "myvault",
			BackendType: vault.BackendType,
			Config: map[string]string{
				"endpoint": "http://vault",
				"token":    "deadbeef",
			},
		},
		{
			ID:          "another-vault-id",
			Name:        "another-vault",
			BackendType: vault.BackendType,
			Config: map[string]string{
				"endpoint": "http://another-vault",
			},
		},
	}
	if modelType == coremodel.CAAS {
		cld := cloud.Cloud{
			Name:              "test",
			Type:              "kubernetes",
			Endpoint:          "http://nowhere",
			CACertificates:    []string{"cert-data"},
			IsControllerCloud: true,
		}
		cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{"foo": "bar"})
		s.mockState.EXPECT().GetControllerModelCloudAndCredential(gomock.Any()).Return(cld, cred, nil)
		backends = append(backends, &secretbackend.SecretBackend{
			ID:          k8sBackendID,
			Name:        kubernetes.BackendName,
			BackendType: kubernetes.BackendType,
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

	info, err := svc.BackendSummaryInfo(context.Background(), reveal, names...)
	sort.Slice(info, func(i, j int) bool {
		return info[i].Name < info[j].Name
	})
	sort.Slice(expected, func(i, j int) bool {
		return expected[i].Name < expected[j].Name
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("info: \n%s", pretty.Sprint(info))
	c.Logf("expected: \n%s", pretty.Sprint(expected))
	c.Assert(info, jc.DeepEquals, expected)
}

func (s *serviceSuite) TestBackendSummaryInfoWithFilterAllCAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
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
				Status:  "error",
				Message: "boom",
			},
			{
				SecretBackend: coresecrets.SecretBackend{
					ID:          k8sBackendID,
					Name:        "model-local",
					BackendType: kubernetes.BackendType,
					Config: map[string]interface{}{
						"ca-certs":            []string{"cert-data"},
						"credential":          `{"auth-type":"access-key","Attributes":{"foo":"bar"}}`,
						"endpoint":            "http://nowhere",
						"is-controller-cloud": true,
					},
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
				Status: "active",
			},
		},
	)
}

func (s *serviceSuite) TestBackendSummaryInfoWithFilterAllIAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
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
				Status:  "error",
				Message: "boom",
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
				Status: "active",
			},
		},
	)
}

func (s *serviceSuite) TestBackendSummaryInfoWithFilterNames(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
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
				Status:  "error",
				Message: "boom",
			},
		},
	)
}

func (s *serviceSuite) TestBackendSummaryInfoWithFilterNamesNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
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

func (s *serviceSuite) TestPingSecretBackend(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, gc.Equals, "vault")
			return s.mockRegistry, nil
		},
	)
	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{Name: "myvault"}).Return(&secretbackend.SecretBackend{
		ID:          "backend-uuid",
		Name:        "myvault",
		BackendType: vault.BackendType,
		Config: map[string]string{
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
	err := svc.PingSecretBackend(context.Background(), "myvault")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreateSecretBackendFailed(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, gc.Equals, "something")
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	err := svc.CreateSecretBackend(context.Background(), coresecrets.SecretBackend{})
	c.Check(err, jc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, "secret backend not valid: missing ID")

	err = svc.CreateSecretBackend(context.Background(), coresecrets.SecretBackend{
		ID: "backend-uuid",
	})
	c.Check(err, jc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, "secret backend not valid: missing name")

	err = svc.CreateSecretBackend(context.Background(), coresecrets.SecretBackend{
		ID:   "backend-uuid",
		Name: juju.BackendName,
	})
	c.Check(err, jc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: reserved name "internal"`)

	err = svc.CreateSecretBackend(context.Background(), coresecrets.SecretBackend{
		ID:   "backend-uuid",
		Name: provider.Auto,
	})
	c.Check(err, jc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: reserved name "auto"`)

	s.mockRegistry.EXPECT().Type().Return("something").AnyTimes()
	err = svc.CreateSecretBackend(context.Background(), coresecrets.SecretBackend{
		ID:          "backend-uuid",
		Name:        "invalid",
		BackendType: "something",
	})
	c.Check(err, jc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(errors.Cause(err), gc.ErrorMatches, `secret backend not valid: config for provider "something": bad config for "something"`)
}

func (s *serviceSuite) TestCreateSecretBackend(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
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
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)

	arg := UpdateSecretBackendParams{}
	err := svc.UpdateSecretBackend(context.Background(), arg)
	c.Check(err, jc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, "secret backend not valid: both ID and name are missing")

	arg.ID = "backend-uuid"
	arg.NewName = ptr(juju.BackendName)
	err = svc.UpdateSecretBackend(context.Background(), arg)
	c.Check(err, jc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: reserved name "internal"`)

	arg.NewName = ptr(provider.Auto)
	err = svc.UpdateSecretBackend(context.Background(), arg)
	c.Check(err, jc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: reserved name "auto"`)

	arg = UpdateSecretBackendParams{}
	arg.ID = "backend-uuid"
	arg.Name = "myvault"
	err = svc.UpdateSecretBackend(context.Background(), arg)
	c.Check(err, jc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: both ID and name are set`)

	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{ID: "backend-uuid"}).
		Return(&secretbackend.SecretBackend{
			BackendType: "something",
		}, nil)
	s.mockRegistry.EXPECT().Type().Return("something").AnyTimes()
	arg = UpdateSecretBackendParams{}
	arg.ID = "backend-uuid"
	err = svc.UpdateSecretBackend(context.Background(), arg)
	c.Check(err, jc.ErrorIs, secretbackenderrors.NotValid)
	c.Check(errors.Cause(err), gc.ErrorMatches, `secret backend not valid: config for provider "something": bad config for "something"`)
}

func (s *serviceSuite) assertUpdateSecretBackend(c *gc.C, byName, skipPing bool) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			c.Assert(backendType, gc.Equals, "vault")
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
			Config: map[string]string{
				"endpoint": "http://vault",
			},
		}, nil)
	} else {
		s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{ID: "backend-uuid"}).Return(&secretbackend.SecretBackend{
			ID:          "backend-uuid",
			Name:        "myvault",
			BackendType: "vault",
			Config: map[string]string{
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

	err := svc.UpdateSecretBackend(context.Background(), arg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateSecretBackend(c *gc.C) {
	s.assertUpdateSecretBackend(c, false, false)
}

func (s *serviceSuite) TestUpdateSecretBackendByName(c *gc.C) {
	s.assertUpdateSecretBackend(c, true, false)
}

func (s *serviceSuite) TestUpdateSecretBackendWithForce(c *gc.C) {
	s.assertUpdateSecretBackend(c, false, true)
}

func (s *serviceSuite) TestUpdateSecretBackendWithForceByName(c *gc.C) {
	s.assertUpdateSecretBackend(c, true, true)
}

func (s *serviceSuite) TestDeleteSecretBackend(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)
	s.mockState.EXPECT().DeleteSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{ID: "backend-uuid"}, false).Return(nil)
	err := svc.DeleteSecretBackend(context.Background(), DeleteSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{ID: "backend-uuid"},
		DeleteInUse:       false,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGetSecretBackendByName(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
		func(backendType string) (provider.SecretBackendProvider, error) {
			return providerWithConfig{
				SecretBackendProvider: s.mockRegistry,
			}, nil
		},
	)
	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{Name: "myvault"}).Return(&secretbackend.SecretBackend{
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
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
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
		Config: map[string]string{
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

	err := svc.RotateBackendToken(context.Background(), "backend-uuid")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRotateBackendTokenRetry(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
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
		Config: map[string]string{
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

	err := svc.RotateBackendToken(context.Background(), "backend-uuid")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestWatchSecretBackendRotationChanges(c *gc.C) {
	defer s.setupMocks(c).Finish()

	backendID1 := uuid.MustNewUUID().String()
	backendID2 := uuid.MustNewUUID().String()
	nextRotateTime1 := time.Now().Add(12 * time.Hour)
	nextRotateTime2 := time.Now().Add(24 * time.Hour)

	svc := newWatchableService(
		s.mockState, s.logger, s.mockWatcherFactory, jujutesting.ControllerTag.Id(), s.clock,
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

	s.PatchValue(&InitialNamespaceChanges, func(selectAll string) eventsource.NamespaceQuery {
		c.Assert(selectAll, gc.Equals, "SELECT * FROM table")
		return nil
	})
	s.mockState.EXPECT().InitialWatchStatement().Return("table", "SELECT * FROM table")
	s.mockWatcherFactory.EXPECT().NewNamespaceWatcher("table", changestream.All, gomock.Any()).Return(s.mockStringWatcher, nil)
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

	w, err := svc.WatchSecretBackendRotationChanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)

	wC := watchertest.NewSecretBackendRotateWatcherC(c, w)

	select {
	case ch <- []string{backendID1, backendID2}:
	case <-time.After(jujutesting.ShortWait):
		c.Fatalf("timed out waiting for the initial changes")
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

func (s *serviceSuite) assertGetSecretsToDrain(c *gc.C, backendID string, expectedRevisions ...RevisionInfo) {
	defer s.setupMocks(c).Finish()
	svc := newService(
		s.mockState, s.logger, jujutesting.ControllerTag.Id(), s.clock,
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
	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).
		Return(secretbackend.ModelSecretBackend{
			SecretBackendID: backendID,
		}, nil)
	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{Name: juju.BackendName}).
		Return(&secretbackend.SecretBackend{
			ID: jujuBackendID,
		}, nil)

	revisions := []*coresecrets.SecretRevisionMetadata{
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

	results, err := svc.GetRevisionsToDrain(context.Background(), modelUUID, revisions)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedRevisions)
}

func (s *serviceSuite) TestGetRevisionsToDrainAutoIAAS(c *gc.C) {
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

func (s *serviceSuite) TestGetRevisionsToDrainAutoCAAS(c *gc.C) {
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

func (s *serviceSuite) TestGetRevisionsToDrainExternal(c *gc.C) {
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
