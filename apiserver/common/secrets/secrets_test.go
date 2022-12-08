// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/apiserver/common/secrets/mocks"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/leadership"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
	"github.com/juju/juju/secrets/provider/juju"
	"github.com/juju/juju/secrets/provider/kubernetes"
	"github.com/juju/juju/secrets/provider/vault"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type secretsSuite struct {
	testing.IsolationSuite
	coretesting.JujuOSEnvSuite
}

var _ = gc.Suite(&secretsSuite{})

func (s *secretsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
}

func (s *secretsSuite) TestAdminBackendConfigInfoDefaultIAAS(c *gc.C) {
	s.assertAdminBackendConfigInfoDefault(c, state.ModelTypeIAAS)
}

func (s *secretsSuite) TestAdminBackendConfigInfoDefaultCAAS(c *gc.C) {
	s.assertAdminBackendConfigInfoDefault(c, state.ModelTypeCAAS)
}

func (s *secretsSuite) assertAdminBackendConfigInfoDefault(c *gc.C, modelType state.ModelType) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)
	backendState := mocks.NewMockSecretBackendsStorage(ctrl)
	secretsState := mocks.NewMockSecretsStore(ctrl)

	s.PatchValue(&secrets.GetSecretBackendsState, func(secrets.Model) state.SecretBackendsStorage { return backendState })
	s.PatchValue(&secrets.GetSecretsState, func(secrets.Model) state.SecretsStore { return secretsState })

	cfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"secret-backend": "auto",
	})
	model.EXPECT().ControllerUUID().Return(coretesting.ControllerTag.Id()).AnyTimes()
	model.EXPECT().UUID().Return(coretesting.ModelTag.Id()).AnyTimes()
	model.EXPECT().Name().Return("fred").AnyTimes()
	model.EXPECT().Config().Return(cfg, nil)
	model.EXPECT().Type().Return(modelType)
	if modelType == state.ModelTypeCAAS {
		cld := cloud.Cloud{
			Name:              "test",
			Type:              "kubernetes",
			Endpoint:          "http://nowhere",
			CACertificates:    []string{"cert-data"},
			IsControllerCloud: true,
		}
		cred := mocks.NewMockCredential(ctrl)
		cred.EXPECT().AuthType().Return("access-key")
		cred.EXPECT().Attributes().Return(map[string]string{"foo": "bar"})
		model.EXPECT().Cloud().Return(cld, nil)
		model.EXPECT().CloudCredential().Return(cred, nil)
	}

	backendState.EXPECT().ListSecretBackends().Return([]*coresecrets.SecretBackend{{
		ID:          "some-id",
		Name:        "myvault",
		BackendType: vault.BackendType,
		Config: map[string]interface{}{
			"endpoint": "http://vault",
		},
	}}, nil)
	expectedConfigs := map[string]provider.BackendConfig{
		"some-id": {BackendType: vault.BackendType,
			Config: map[string]interface{}{
				"endpoint": "http://vault",
			},
		},
	}
	var activeID string
	if modelType == state.ModelTypeIAAS {
		activeID = coretesting.ControllerTag.Id()
		expectedConfigs[coretesting.ControllerTag.Id()] = provider.BackendConfig{BackendType: juju.BackendType}
	} else {
		activeID = coretesting.ModelTag.Id()
		expectedConfigs[coretesting.ModelTag.Id()] = provider.BackendConfig{
			BackendType: kubernetes.BackendType,
			Config: provider.ConfigAttrs{
				"endpoint":            "http://nowhere",
				"ca-certs":            []string{"cert-data"},
				"credential":          `{"auth-type":"access-key","Attributes":{"foo":"bar"}}`,
				"is-controller-cloud": true,
			},
		}
	}
	info, gotID, err := secrets.AdminBackendConfigInfo(model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotID, gc.Equals, activeID)
	c.Assert(info, jc.DeepEquals, &provider.ModelBackendConfigInfo{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		Configs:        expectedConfigs,
	})
}

func (s *secretsSuite) TestBackendConfigInfoLeaderUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("gitlab/0")
	model := mocks.NewMockModel(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	token := mocks.NewMockToken(ctrl)
	p := mocks.NewMockSecretBackendProvider(ctrl)
	backendState := mocks.NewMockSecretBackendsStorage(ctrl)
	secretsState := mocks.NewMockSecretsStore(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return p, nil })
	s.PatchValue(&secrets.GetSecretsState, func(secrets.Model) state.SecretsStore { return secretsState })
	s.PatchValue(&secrets.GetSecretBackendsState, func(secrets.Model) state.SecretBackendsStorage { return backendState })

	owned := []*coresecrets.SecretMetadata{
		{URI: &coresecrets.URI{ID: "owned-1"}},
	}
	ownedRevs := map[string]set.Strings{
		"owned-1": set.NewStrings("owned-rev-1", "owned-rev-2"),
	}
	read := []*coresecrets.SecretMetadata{
		{URI: &coresecrets.URI{ID: "read-1"}},
	}
	readRevs := map[string]set.Strings{
		"read-1": set.NewStrings("read-rev-1"),
	}
	modelCfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"secret-backend": "backend-name",
	})
	adminCfg := provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
		},
	}
	model.EXPECT().ControllerUUID().Return(coretesting.ControllerTag.Id()).AnyTimes()
	model.EXPECT().UUID().Return(coretesting.ModelTag.Id()).AnyTimes()
	model.EXPECT().Name().Return("fred").AnyTimes()
	gomock.InOrder(
		model.EXPECT().Config().Return(modelCfg, nil),
		backendState.EXPECT().ListSecretBackends().Return([]*coresecrets.SecretBackend{{
			ID:          "backend-id",
			Name:        "backend-name",
			BackendType: "some-backend",
		}}, nil),
		p.EXPECT().Initialise(gomock.Any()).Return(nil),
		leadershipChecker.EXPECT().LeadershipCheck("gitlab", "gitlab/0").Return(token),
		token.EXPECT().Check().Return(nil),

		secretsState.EXPECT().ListSecrets(state.SecretsFilter{
			OwnerTags: []names.Tag{
				unitTag, names.NewApplicationTag("gitlab"),
			},
		}).Return(owned, nil),
		secretsState.EXPECT().ListSecretRevisions(&coresecrets.URI{ID: "owned-1"}).
			Return([]*coresecrets.SecretRevisionMetadata{{
				Revision: 1,
				ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "owned-rev-1"},
			}, {
				Revision: 2,
				ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "owned-rev-2"},
			}}, nil),
		secretsState.EXPECT().ListSecrets(state.SecretsFilter{
			ConsumerTags: []names.Tag{unitTag, names.NewApplicationTag("gitlab")},
		}).Return(read, nil),
		secretsState.EXPECT().ListSecretRevisions(&coresecrets.URI{ID: "read-1"}).
			Return([]*coresecrets.SecretRevisionMetadata{{
				Revision: 1,
				ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "read-rev-1"},
			}}, nil),
		p.EXPECT().RestrictedConfig(&adminCfg, unitTag, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil),
	)

	info, err := secrets.BackendConfigInfo(model, unitTag, leadershipChecker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &provider.ModelBackendConfigInfo{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		ActiveID:       "backend-id",
		Configs: map[string]provider.BackendConfig{
			"backend-id": {
				BackendType: "some-backend",
			},
		},
	})
}

func (s *secretsSuite) TestBackendConfigInfoNonLeaderUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("gitlab/0")
	model := mocks.NewMockModel(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	token := mocks.NewMockToken(ctrl)
	p := mocks.NewMockSecretBackendProvider(ctrl)
	backendState := mocks.NewMockSecretBackendsStorage(ctrl)
	secretsState := mocks.NewMockSecretsStore(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return p, nil })
	s.PatchValue(&secrets.GetSecretsState, func(secrets.Model) state.SecretsStore { return secretsState })
	s.PatchValue(&secrets.GetSecretBackendsState, func(secrets.Model) state.SecretBackendsStorage { return backendState })

	unitOwned := []*coresecrets.SecretMetadata{
		{URI: &coresecrets.URI{ID: "owned-1"}},
	}
	appOwned := []*coresecrets.SecretMetadata{
		{URI: &coresecrets.URI{ID: "app-owned-1"}},
	}
	ownedRevs := map[string]set.Strings{
		"owned-1": set.NewStrings("owned-rev-1", "owned-rev-2"),
	}
	read := []*coresecrets.SecretMetadata{
		{URI: &coresecrets.URI{ID: "read-1"}},
	}
	readRevs := map[string]set.Strings{
		"read-1":      set.NewStrings("read-rev-1"),
		"app-owned-1": set.NewStrings("app-owned-rev-1", "app-owned-rev-2", "app-owned-rev-3"),
	}
	modelCfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"secret-backend": "backend-name",
	})
	adminCfg := provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
		},
	}
	model.EXPECT().ControllerUUID().Return(coretesting.ControllerTag.Id()).AnyTimes()
	model.EXPECT().UUID().Return(coretesting.ModelTag.Id()).AnyTimes()
	model.EXPECT().Name().Return("fred").AnyTimes()
	gomock.InOrder(
		model.EXPECT().Config().Return(modelCfg, nil),
		backendState.EXPECT().ListSecretBackends().Return([]*coresecrets.SecretBackend{{
			ID:          "backend-id",
			Name:        "backend-name",
			BackendType: "some-backend",
		}}, nil),
		p.EXPECT().Initialise(gomock.Any()).Return(nil),
		leadershipChecker.EXPECT().LeadershipCheck("gitlab", "gitlab/0").Return(token),
		token.EXPECT().Check().Return(leadership.NewNotLeaderError("", "")),

		secretsState.EXPECT().ListSecrets(state.SecretsFilter{
			OwnerTags: []names.Tag{unitTag},
		}).Return(unitOwned, nil),
		secretsState.EXPECT().ListSecretRevisions(&coresecrets.URI{ID: "owned-1"}).
			Return([]*coresecrets.SecretRevisionMetadata{{
				Revision: 1,
				ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "owned-rev-1"},
			}, {
				Revision: 2,
				ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "owned-rev-2"},
			}}, nil),
		secretsState.EXPECT().ListSecrets(state.SecretsFilter{
			ConsumerTags: []names.Tag{unitTag, names.NewApplicationTag("gitlab")},
		}).Return(read, nil),
		secretsState.EXPECT().ListSecretRevisions(&coresecrets.URI{ID: "read-1"}).
			Return([]*coresecrets.SecretRevisionMetadata{{
				Revision: 1,
				ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "read-rev-1"},
			}}, nil),
		secretsState.EXPECT().ListSecrets(state.SecretsFilter{
			OwnerTags: []names.Tag{names.NewApplicationTag("gitlab")},
		}).Return(appOwned, nil),
		secretsState.EXPECT().ListSecretRevisions(&coresecrets.URI{ID: "app-owned-1"}).
			Return([]*coresecrets.SecretRevisionMetadata{{
				Revision: 1,
				ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "app-owned-rev-1"},
			}, {
				Revision: 2,
				ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "app-owned-rev-2"},
			}, {
				Revision: 3,
				ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "app-owned-rev-3"},
			}}, nil),
		p.EXPECT().RestrictedConfig(&adminCfg, unitTag, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil),
	)

	info, err := secrets.BackendConfigInfo(model, unitTag, leadershipChecker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &provider.ModelBackendConfigInfo{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		ActiveID:       "backend-id",
		Configs: map[string]provider.BackendConfig{
			"backend-id": {
				BackendType: "some-backend",
			},
		},
	})
}

func (s *secretsSuite) TestBackendConfigInfoAppTagLogin(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appTag := names.NewApplicationTag("gitlab")
	model := mocks.NewMockModel(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	p := mocks.NewMockSecretBackendProvider(ctrl)
	backendState := mocks.NewMockSecretBackendsStorage(ctrl)
	secretsState := mocks.NewMockSecretsStore(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return p, nil })
	s.PatchValue(&secrets.GetSecretsState, func(secrets.Model) state.SecretsStore { return secretsState })
	s.PatchValue(&secrets.GetSecretBackendsState, func(secrets.Model) state.SecretBackendsStorage { return backendState })

	owned := []*coresecrets.SecretMetadata{
		{URI: &coresecrets.URI{ID: "owned-1"}},
	}
	ownedRevs := map[string]set.Strings{
		"owned-1": set.NewStrings("owned-rev-1", "owned-rev-2"),
	}
	read := []*coresecrets.SecretMetadata{
		{URI: &coresecrets.URI{ID: "read-1"}},
	}
	readRevs := map[string]set.Strings{
		"read-1": set.NewStrings("read-rev-1"),
	}
	modelCfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"secret-backend": "backend-name",
	})
	adminCfg := provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
		},
	}
	model.EXPECT().ControllerUUID().Return(coretesting.ControllerTag.Id()).AnyTimes()
	model.EXPECT().UUID().Return(coretesting.ModelTag.Id()).AnyTimes()
	model.EXPECT().Name().Return("fred").AnyTimes()
	gomock.InOrder(
		model.EXPECT().Config().Return(modelCfg, nil),
		backendState.EXPECT().ListSecretBackends().Return([]*coresecrets.SecretBackend{{
			ID:          "backend-id",
			Name:        "backend-name",
			BackendType: "some-backend",
		}}, nil),
		p.EXPECT().Initialise(gomock.Any()).Return(nil),

		secretsState.EXPECT().ListSecrets(state.SecretsFilter{
			OwnerTags: []names.Tag{appTag},
		}).Return(owned, nil),
		secretsState.EXPECT().ListSecretRevisions(&coresecrets.URI{ID: "owned-1"}).
			Return([]*coresecrets.SecretRevisionMetadata{{
				Revision: 1,
				ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "owned-rev-1"},
			}, {
				Revision: 2,
				ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "owned-rev-2"},
			}}, nil),
		secretsState.EXPECT().ListSecrets(state.SecretsFilter{
			ConsumerTags: []names.Tag{appTag},
		}).Return(read, nil),
		secretsState.EXPECT().ListSecretRevisions(&coresecrets.URI{ID: "read-1"}).
			Return([]*coresecrets.SecretRevisionMetadata{{
				Revision: 1,
				ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "read-rev-1"},
			}}, nil),
		p.EXPECT().RestrictedConfig(&adminCfg, appTag, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil),
	)

	info, err := secrets.BackendConfigInfo(model, appTag, leadershipChecker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &provider.ModelBackendConfigInfo{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		ActiveID:       "backend-id",
		Configs: map[string]provider.BackendConfig{
			"backend-id": {
				BackendType: "some-backend",
			},
		},
	})
}

func (s *secretsSuite) TestBackendConfigInfoFailedInvalidAuthTag(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	badTag := names.NewUserTag("foo")
	model := mocks.NewMockModel(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	p := mocks.NewMockSecretBackendProvider(ctrl)
	backendState := mocks.NewMockSecretBackendsStorage(ctrl)
	secretsState := mocks.NewMockSecretsStore(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return p, nil })
	s.PatchValue(&secrets.GetSecretsState, func(secrets.Model) state.SecretsStore { return secretsState })
	s.PatchValue(&secrets.GetSecretBackendsState, func(secrets.Model) state.SecretBackendsStorage { return backendState })

	cfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"secret-backend": "internal",
	})
	model.EXPECT().ControllerUUID().Return(coretesting.ControllerTag.Id()).AnyTimes()
	model.EXPECT().UUID().Return(coretesting.ModelTag.Id()).AnyTimes()
	model.EXPECT().Name().Return("fred").AnyTimes()
	gomock.InOrder(
		model.EXPECT().Config().Return(cfg, nil),
		backendState.EXPECT().ListSecretBackends().Return([]*coresecrets.SecretBackend{{
			ID:          "some-id",
			Name:        "myvault",
			BackendType: vault.BackendType,
			Config: map[string]interface{}{
				"endpoint": "http://vault",
			},
		}}, nil),
		p.EXPECT().Initialise(gomock.Any()).Return(nil),
	)

	_, err := secrets.BackendConfigInfo(model, badTag, leadershipChecker)
	c.Assert(err, gc.ErrorMatches, `login as "user-foo" not supported`)
}

func (s *secretsSuite) TestBackendForInspect(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)
	p := mocks.NewMockSecretBackendProvider(ctrl)
	backendState := mocks.NewMockSecretBackendsStorage(ctrl)
	secretsState := mocks.NewMockSecretsStore(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return p, nil })
	s.PatchValue(&secrets.GetSecretsState, func(secrets.Model) state.SecretsStore { return secretsState })
	s.PatchValue(&secrets.GetSecretBackendsState, func(secrets.Model) state.SecretBackendsStorage { return backendState })

	vaultCfg := map[string]interface{}{"endpoint": "http://vault"}
	adminCfg := provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "vault",
			Config:      vaultCfg,
		},
	}
	restrictedCfg := adminCfg
	restrictedCfg.Config["restricted"] = "true"
	model.EXPECT().ControllerUUID().Return(coretesting.ControllerTag.Id()).AnyTimes()
	model.EXPECT().UUID().Return(coretesting.ModelTag.Id()).AnyTimes()
	model.EXPECT().Name().Return("fred").AnyTimes()
	gomock.InOrder(
		backendState.EXPECT().GetSecretBackendByID("backend-id").Return(&coresecrets.SecretBackend{
			BackendType: "vault",
			Config:      vaultCfg,
		}, nil),
		p.EXPECT().RestrictedConfig(&adminCfg, nil, nil, nil).Return(&restrictedCfg.BackendConfig, nil),
		p.EXPECT().NewBackend(&restrictedCfg).Return(nil, nil),
	)

	_, err := secrets.BackendForInspect(model, "backend-id")
	c.Assert(err, jc.ErrorIsNil)
}
