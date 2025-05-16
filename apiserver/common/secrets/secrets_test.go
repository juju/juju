// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"fmt"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/apiserver/common/secrets/mocks"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/leadership"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
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

var (
	jujuBackendID  = coretesting.ControllerTag.Id()
	k8sBackendID   = coretesting.ModelTag.Id()
	vaultBackendID = "vault-backend-id"

	jujuBackendConfig = provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: juju.BackendType,
		},
	}
	k8sBackendConfig = provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: kubernetes.BackendType,
			Config: provider.ConfigAttrs{
				"endpoint":                 "http://nowhere",
				"ca-certs":                 []string{"cert-data"},
				"namespace":                "fred",
				"token":                    "bar",
				"prefer-incluster-address": true,
			},
		},
	}
	vaultBackendConfig = provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: vault.BackendType,
			Config: provider.ConfigAttrs{
				"endpoint": "http://vault",
			},
		},
	}
)

func (s *secretsSuite) TestMarshallLegacyBackendConfig(c *gc.C) {
	cfg := params.SecretBackendConfig{
		BackendType: kubernetes.BackendType,
		Params: map[string]interface{}{
			"endpoint":                 "http://nowhere",
			"ca-certs":                 []string{"cert-data"},
			"namespace":                "fred",
			"token":                    "bar",
			"prefer-incluster-address": true,
		},
	}
	err := secrets.MarshallLegacyBackendConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, params.SecretBackendConfig{
		BackendType: kubernetes.BackendType,
		Params: map[string]interface{}{
			"endpoint":            "http://nowhere",
			"ca-certs":            []string{"cert-data"},
			"credential":          `{"auth-type":"oauth2","Attributes":{"Token":"bar"}}`,
			"is-controller-cloud": false,
		},
	})
}

func (s *secretsSuite) TestAdminBackendConfigInfoDefaultIAAS(c *gc.C) {
	s.assertAdminBackendConfigInfoDefault(c, state.ModelTypeIAAS, "auto",
		&provider.ModelBackendConfigInfo{
			ActiveID: jujuBackendID,
			Configs: map[string]provider.ModelBackendConfig{
				jujuBackendID:  jujuBackendConfig,
				vaultBackendID: vaultBackendConfig,
			},
		},
	)
}

func (s *secretsSuite) TestAdminBackendConfigInfoDefaultCAAS(c *gc.C) {
	s.assertAdminBackendConfigInfoDefault(c, state.ModelTypeCAAS, "auto",
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

func (s *secretsSuite) TestAdminBackendConfigInfoInternalIAAS(c *gc.C) {
	s.assertAdminBackendConfigInfoDefault(c, state.ModelTypeIAAS, "internal",
		&provider.ModelBackendConfigInfo{
			ActiveID: jujuBackendID,
			Configs: map[string]provider.ModelBackendConfig{
				jujuBackendID:  jujuBackendConfig,
				vaultBackendID: vaultBackendConfig,
			},
		},
	)
}

func (s *secretsSuite) TestAdminBackendConfigInfoInternalCAAS(c *gc.C) {
	s.assertAdminBackendConfigInfoDefault(c, state.ModelTypeCAAS, "internal",
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

func (s *secretsSuite) TestAdminBackendConfigInfoExternalIAAS(c *gc.C) {
	s.assertAdminBackendConfigInfoDefault(c, state.ModelTypeIAAS, "myvault",
		&provider.ModelBackendConfigInfo{
			ActiveID: vaultBackendID,
			Configs: map[string]provider.ModelBackendConfig{
				jujuBackendID:  jujuBackendConfig,
				vaultBackendID: vaultBackendConfig,
			},
		},
	)
}

func (s *secretsSuite) TestAdminBackendConfigInfoExternalCAAS(c *gc.C) {
	s.assertAdminBackendConfigInfoDefault(c, state.ModelTypeCAAS, "myvault",
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

func (s *secretsSuite) assertAdminBackendConfigInfoDefault(
	c *gc.C, modelType state.ModelType, backendName string, expected *provider.ModelBackendConfigInfo,
) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)
	backendState := mocks.NewMockSecretBackendsStorage(ctrl)
	secretsState := mocks.NewMockSecretsStore(ctrl)

	s.PatchValue(&secrets.GetSecretBackendsState, func(secrets.Model) state.SecretBackendsStorage { return backendState })
	s.PatchValue(&secrets.GetSecretsState, func(secrets.Model) state.SecretsStore { return secretsState })

	cfg := coretesting.CustomModelConfig(c, coretesting.Attrs{"secret-backend": backendName})
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
		cred.EXPECT().AuthType().Return("oauth2")
		cred.EXPECT().Attributes().Return(map[string]string{"Token": "bar"})
		model.EXPECT().Cloud().Return(cld, nil)
		model.EXPECT().CloudCredential().Return(cred, nil)
	}

	backendState.EXPECT().ListSecretBackends().Return([]*coresecrets.SecretBackend{{
		ID:          vaultBackendID,
		Name:        "myvault",
		BackendType: vault.BackendType,
		Config: map[string]interface{}{
			"endpoint": "http://vault",
		},
	}}, nil)

	info, err := secrets.AdminBackendConfigInfo(model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)
}

func (s *secretsSuite) TestBackendConfigInfoLeaderUnit(c *gc.C) {
	s.assertBackendConfigInfoLeaderUnit(c, []string{"backend-id"})
}

func (s *secretsSuite) TestBackendConfigInfoDefaultAdmin(c *gc.C) {
	s.assertBackendConfigInfoLeaderUnit(c, nil)
}

func (s *secretsSuite) assertBackendConfigInfoLeaderUnit(c *gc.C, wanted []string) {
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
		model.EXPECT().Type().Return(state.ModelTypeIAAS),
		backendState.EXPECT().ListSecretBackends().Return([]*coresecrets.SecretBackend{{
			ID:          "backend-id",
			Name:        "backend-name",
			BackendType: "some-backend",
		}, {
			ID:          "backend-id2",
			Name:        "backend-name2",
			BackendType: "some-backend2",
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
	)
	p.EXPECT().RestrictedConfig(&adminCfg, true, false, unitTag, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil)

	info, err := secrets.BackendConfigInfo(model, true, wanted, false, unitTag, leadershipChecker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
				},
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
		model.EXPECT().Type().Return(state.ModelTypeIAAS),
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
	)
	p.EXPECT().RestrictedConfig(&adminCfg, true, false, unitTag, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil)

	info, err := secrets.BackendConfigInfo(model, true, []string{"backend-id"}, false, unitTag, leadershipChecker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
				},
			},
		},
	})
}

func (s *secretsSuite) TestDrainBackendConfigInfo(c *gc.C) {
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
		model.EXPECT().Type().Return(state.ModelTypeIAAS),
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
	)
	p.EXPECT().RestrictedConfig(&adminCfg, true, true, unitTag, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil)

	info, err := secrets.DrainBackendConfigInfo("backend-id", model, unitTag, leadershipChecker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
				},
			},
		},
	})
}

func (s *secretsSuite) TestSecretCleanupBackendConfigInfo(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)
	backendState := mocks.NewMockSecretBackendsStorage(ctrl)

	s.PatchValue(&secrets.GetSecretBackendsState, func(secrets.Model) state.SecretBackendsStorage { return backendState })

	modelCfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"secret-backend": "backend-name",
	})
	model.EXPECT().ControllerUUID().Return(coretesting.ControllerTag.Id()).AnyTimes()
	model.EXPECT().UUID().Return(coretesting.ModelTag.Id()).AnyTimes()
	model.EXPECT().Name().Return("fred").AnyTimes()
	gomock.InOrder(
		model.EXPECT().Config().Return(modelCfg, nil),
		model.EXPECT().Type().Return(state.ModelTypeIAAS),
		backendState.EXPECT().ListSecretBackends().Return([]*coresecrets.SecretBackend{{
			ID:          "backend-id",
			Name:        "backend-name",
			BackendType: "some-backend",
		}}, nil),
	)

	info, err := secrets.SecretCleanupBackendConfigInfo(model, "backend-id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
				},
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
		model.EXPECT().Type().Return(state.ModelTypeIAAS),
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
	)
	p.EXPECT().RestrictedConfig(&adminCfg, true, false, appTag, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil)

	info, err := secrets.BackendConfigInfo(model, true, []string{"backend-id"}, false, appTag, leadershipChecker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
				},
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
		model.EXPECT().Type().Return(state.ModelTypeIAAS),
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

	_, err := secrets.BackendConfigInfo(model, true, []string{"some-id"}, false, badTag, leadershipChecker)
	c.Assert(err, gc.ErrorMatches, `login as "user-foo" not supported`)
}

func (s *secretsSuite) TestGetSecretMetadata(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	leadershipChecker := mocks.NewMockChecker(ctrl)
	token := mocks.NewMockToken(ctrl)
	secretsMetaState := mocks.NewMockSecretsMetaState(ctrl)

	leadershipChecker.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(token)
	token.EXPECT().Check().Return(nil)

	now := time.Now()
	uri := coresecrets.NewURI()
	authTag := names.NewUnitTag("mariadb/0")
	secretsMetaState.EXPECT().ListSecrets(
		state.SecretsFilter{
			OwnerTags: []names.Tag{names.NewUnitTag("mariadb/0"), names.NewApplicationTag("mariadb")},
		}).Return([]*coresecrets.SecretMetadata{{
		URI:                    uri,
		OwnerTag:               "application-mariadb",
		Description:            "description",
		Label:                  "label",
		RotatePolicy:           coresecrets.RotateHourly,
		LatestRevision:         666,
		LatestRevisionChecksum: "checksum",
		LatestExpireTime:       &now,
		NextRotateTime:         &now,
	}}, nil)
	secretsMetaState.EXPECT().SecretGrants(uri, coresecrets.RoleView).Return([]coresecrets.AccessInfo{
		{
			Target: "application-gitlab",
			Scope:  "relation-key",
			Role:   coresecrets.RoleView,
		},
	}, nil)
	secretsMetaState.EXPECT().ListSecretRevisions(uri).Return([]*coresecrets.SecretRevisionMetadata{{
		Revision: 666,
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		},
	}, {
		Revision: 667,
	}}, nil)

	results, err := secrets.GetSecretMetadata(authTag, secretsMetaState, leadershipChecker, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ListSecretResults{
		Results: []params.ListSecretResult{{
			URI:                    uri.String(),
			OwnerTag:               "application-mariadb",
			Description:            "description",
			Label:                  "label",
			RotatePolicy:           coresecrets.RotateHourly.String(),
			LatestRevision:         666,
			LatestRevisionChecksum: "checksum",
			LatestExpireTime:       &now,
			NextRotateTime:         &now,
			Revisions: []params.SecretRevision{{
				Revision: 666,
				ValueRef: &params.SecretValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				},
			}, {
				Revision: 667,
			}},
			Access: []params.AccessInfo{
				{TargetTag: "application-gitlab", ScopeTag: "relation-key", Role: "view"},
			},
		}},
	})
}

func (s *secretsSuite) TestRemoveSecretsForSecretOwnersWithRevisions(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	expectURI := *uri
	removeState := mocks.NewMockSecretsRemoveState(ctrl)
	mockprovider := mocks.NewMockSecretBackendProvider(ctrl)
	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return mockprovider, nil })

	removeState.EXPECT().GetSecret(&expectURI).Return(&coresecrets.SecretMetadata{}, nil)
	removeState.EXPECT().DeleteSecret(&expectURI, []int{666}).Return([]coresecrets.ValueRef{{
		BackendID:  "backend-id",
		RevisionID: "rev-666",
	}}, nil)

	results, err := secrets.RemoveSecretsForAgent(
		removeState,
		params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{{
				URI:       expectURI.String(),
				Revisions: []int{666},
			}},
		},
		coretesting.ModelTag.Id(),
		func(*coresecrets.URI) error { return nil },
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *secretsSuite) TestRemoveSecretsForSecretOwners(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	expectURI := *uri
	removeState := mocks.NewMockSecretsRemoveState(ctrl)
	mockprovider := mocks.NewMockSecretBackendProvider(ctrl)
	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return mockprovider, nil })

	removeState.EXPECT().GetSecret(&expectURI).Return(&coresecrets.SecretMetadata{}, nil)
	removeState.EXPECT().DeleteSecret(&expectURI, []int{}).Return([]coresecrets.ValueRef{{
		BackendID:  "backend-id",
		RevisionID: "rev-666",
	}}, nil)

	results, err := secrets.RemoveSecretsForAgent(
		removeState,
		params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{{
				URI: expectURI.String(),
			}},
		},
		coretesting.ModelTag.Id(),
		func(*coresecrets.URI) error { return nil },
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func ptr[T any](v T) *T {
	return &v
}

func (s *secretsSuite) TestRemoveSecretsByLabel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	expectURI := *uri
	removeState := mocks.NewMockSecretsRemoveState(ctrl)
	mockprovider := mocks.NewMockSecretBackendProvider(ctrl)
	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return mockprovider, nil })

	removeState.EXPECT().ListSecrets(state.SecretsFilter{
		Label:     ptr("my-secret"),
		OwnerTags: []names.Tag{coretesting.ModelTag},
	}).Return([]*coresecrets.SecretMetadata{{
		URI: uri,
	}}, nil)
	removeState.EXPECT().GetSecret(&expectURI).Return(&coresecrets.SecretMetadata{}, nil)
	removeState.EXPECT().DeleteSecret(&expectURI, []int{}).Return([]coresecrets.ValueRef{{
		BackendID:  "backend-id",
		RevisionID: "rev-666",
	}}, nil)

	results, err := secrets.RemoveSecretsForAgent(
		removeState,
		params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{{
				Label: "my-secret",
			}},
		},
		coretesting.ModelTag.Id(),
		func(*coresecrets.URI) error { return nil },
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *secretsSuite) TestRemoveSecretsForModelAdminFromJujuBackend(c *gc.C) {
	// Test that we correctly delete secrets held in the Juju backend rather than an external provider.
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Data needed for mocks
	uri := coresecrets.NewURI()
	// Secret that lives only in the Juju database, not a backend
	revisionMetadata := &coresecrets.SecretRevisionMetadata{
		Revision: 5,
	}

	removeState := mocks.NewMockSecretsRemoveState(ctrl)
	// removeState.GetSecret always confirms a secrets exist
	removeState.EXPECT().GetSecret(gomock.Any()).AnyTimes().Return(&coresecrets.SecretMetadata{}, nil)
	removeState.EXPECT().GetSecretRevision(uri, 5).Return(revisionMetadata, nil)
	removeState.EXPECT().DeleteSecret(uri, []int{5}).Return([]coresecrets.ValueRef{}, nil)

	adminConfigGetter := func() (*provider.ModelBackendConfigInfo, error) {
		return &provider.ModelBackendConfigInfo{}, nil
	}

	results, err := secrets.RemoveUserSecrets(
		removeState, adminConfigGetter,
		params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{{
				URI:       (*uri).String(),
				Revisions: []int{5},
			}},
		},
		coretesting.ModelTag.Id(),
		func(*coresecrets.URI) error { return nil },
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})

}

func (s *secretsSuite) TestRemoveSecretsForModelAdminWithRevisions(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	expectURI := *uri
	removeState := mocks.NewMockSecretsRemoveState(ctrl)
	mockprovider := mocks.NewMockSecretBackendProvider(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)
	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return mockprovider, nil })

	removeState.EXPECT().GetSecret(&expectURI).Return(&coresecrets.SecretMetadata{}, nil)
	removeState.EXPECT().GetSecretRevision(&expectURI, 666).Return(&coresecrets.SecretRevisionMetadata{
		Revision: 666,
		ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "rev-666"},
	}, nil)
	removeState.EXPECT().DeleteSecret(&expectURI, []int{666}).Return([]coresecrets.ValueRef{{
		BackendID:  "backend-id",
		RevisionID: "rev-666",
	}}, nil)

	cfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
			Config:      map[string]interface{}{"foo": "admin"},
		},
	}
	mockprovider.EXPECT().NewBackend(cfg).Return(backend, nil)
	backend.EXPECT().DeleteContent(gomock.Any(), "rev-666").Return(nil)
	mockprovider.EXPECT().CleanupSecrets(
		cfg, coretesting.ModelTag,
		provider.SecretRevisions{uri.ID: set.NewStrings("rev-666")},
	).Return(nil)

	adminConfigGetter := func() (*provider.ModelBackendConfigInfo, error) {
		return &provider.ModelBackendConfigInfo{
			ActiveID: "backend-id",
			Configs: map[string]provider.ModelBackendConfig{
				"backend-id": *cfg,
			},
		}, nil
	}

	results, err := secrets.RemoveUserSecrets(
		removeState, adminConfigGetter,
		params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{{
				URI:       expectURI.String(),
				Revisions: []int{666},
			}},
		},
		coretesting.ModelTag.Id(),
		func(*coresecrets.URI) error { return nil },
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *secretsSuite) TestRemoveSecretsForModelAdmin(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	expectURI := *uri
	removeState := mocks.NewMockSecretsRemoveState(ctrl)
	mockprovider := mocks.NewMockSecretBackendProvider(ctrl)
	backend := mocks.NewMockSecretsBackend(ctrl)
	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return mockprovider, nil })

	removeState.EXPECT().GetSecret(&expectURI).Return(&coresecrets.SecretMetadata{}, nil)
	removeState.EXPECT().ListSecretRevisions(&expectURI).Return(
		[]*coresecrets.SecretRevisionMetadata{
			{
				Revision: 666,
				ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "rev-666"},
			},
		},
		nil,
	)
	removeState.EXPECT().DeleteSecret(&expectURI, []int{}).Return([]coresecrets.ValueRef{{
		BackendID:  "backend-id",
		RevisionID: "rev-666",
	}}, nil)

	cfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
			Config:      map[string]interface{}{"foo": "admin"},
		},
	}
	mockprovider.EXPECT().NewBackend(cfg).Return(backend, nil)
	backend.EXPECT().DeleteContent(gomock.Any(), "rev-666").Return(nil)
	mockprovider.EXPECT().CleanupSecrets(
		cfg, coretesting.ModelTag,
		provider.SecretRevisions{uri.ID: set.NewStrings("rev-666")},
	).Return(nil)

	adminConfigGetter := func() (*provider.ModelBackendConfigInfo, error) {
		return &provider.ModelBackendConfigInfo{
			ActiveID: "backend-id",
			Configs: map[string]provider.ModelBackendConfig{
				"backend-id": *cfg,
			},
		}, nil
	}

	results, err := secrets.RemoveUserSecrets(
		removeState, adminConfigGetter,
		params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{{
				URI: expectURI.String(),
			}},
		},
		coretesting.ModelTag.Id(),
		func(*coresecrets.URI) error { return nil },
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *secretsSuite) TestRemoveSecretsForModelAdminDuringBackendMigration(c *gc.C) {
	// Tests when:
	// * we delete a secret that has two revisions
	// * the first revision is deleted successfully
	// * the second revision is NotFound on the first deletion attempt (simulating a revision that was drained to
	//   another backend while we were delating the other revision), so RemoveUserSecrets must attempt another deletion
	// * the second revision is successfully deleted on the second attempt

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Data needed for mocks
	uri := coresecrets.NewURI()
	revisionMetadataBeforeMigration := []*coresecrets.SecretRevisionMetadata{
		{
			Revision: 5,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id-before-migration", RevisionID: "rev-5"},
		},
		{
			Revision: 6,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id-before-migration", RevisionID: "rev-6"},
		},
	}
	revisionMetadataAfterMigration := []*coresecrets.SecretRevisionMetadata{
		{
			Revision: 5,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id-after-migration", RevisionID: "rev-5"},
		},
		{
			Revision: 6,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id-after-migration", RevisionID: "rev-6"},
		},
	}

	backendBeforeMigrationCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
			Config:      map[string]interface{}{"before": "migration"},
		},
	}
	backendAfterMigrationCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
			Config:      map[string]interface{}{"after": "migration"},
		},
	}

	// backendBeforeMigration simulates a backend that has rev-5 but not rev-6, as if rev-6 was drained to another
	// backend
	backendBeforeMigration := mocks.NewMockSecretsBackend(ctrl)
	backendBeforeMigration.EXPECT().DeleteContent(gomock.Any(), "rev-5").Return(nil)
	backendBeforeMigration.EXPECT().DeleteContent(gomock.Any(), "rev-6").Return(errors.NotFound)

	// backendAfterMigration simulates a backend that has rev-6 after drain
	backendAfterMigration := mocks.NewMockSecretsBackend(ctrl)
	backendAfterMigration.EXPECT().DeleteContent(gomock.Any(), "rev-6").Return(nil)

	// Provider which has both backends
	mockprovider := mocks.NewMockSecretBackendProvider(ctrl)
	// Mock the package GetProvider function to return the mockprovider
	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return mockprovider, nil })
	// Uses AnyTimes as the number of calls to this is an implementation detail of the function under test
	mockprovider.EXPECT().NewBackend(backendBeforeMigrationCfg).Return(backendBeforeMigration, nil)
	mockprovider.EXPECT().NewBackend(backendAfterMigrationCfg).Return(backendAfterMigration, nil)
	// Expect that we clean up secret revisions for both backends.
	mockprovider.EXPECT().CleanupSecrets(
		backendBeforeMigrationCfg, coretesting.ModelTag,
		provider.SecretRevisions{uri.ID: set.NewStrings("rev-5")},
	).Return(nil)
	mockprovider.EXPECT().CleanupSecrets(
		backendAfterMigrationCfg, coretesting.ModelTag,
		provider.SecretRevisions{uri.ID: set.NewStrings("rev-6")},
	).Return(nil)

	removeState := mocks.NewMockSecretsRemoveState(ctrl)
	// removeState.GetSecret always confirms a secrets exist
	removeState.EXPECT().GetSecret(gomock.Any()).AnyTimes().Return(&coresecrets.SecretMetadata{}, nil)
	// At first pass during deletion we ListSecretRevisions, so return the initial revisions pointing to the first
	// backend.
	removeState.EXPECT().ListSecretRevisions(uri).Return(revisionMetadataBeforeMigration, nil)
	// After DeleteContent fails to delete rev-6 with NotFound, RemoveUserSecrets calls GetSecretRevision to see if the
	// revision has moved. Simulate that here by returning new revision metadata.
	removeState.EXPECT().GetSecretRevision(uri, 6).Return(revisionMetadataAfterMigration[1], nil)
	// Expect that we DeleteSecret exactly once for the entire secret
	removeState.EXPECT().DeleteSecret(uri, []int{}).Return([]coresecrets.ValueRef{}, nil)

	adminConfigGetter := func() (*provider.ModelBackendConfigInfo, error) {
		return &provider.ModelBackendConfigInfo{
			ActiveID: "backend-id-before-migration",
			Configs: map[string]provider.ModelBackendConfig{
				"backend-id-before-migration": *backendBeforeMigrationCfg,
				"backend-id-after-migration":  *backendAfterMigrationCfg,
			},
		}, nil
	}

	results, err := secrets.RemoveUserSecrets(
		removeState, adminConfigGetter,
		params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{{
				URI: (*uri).String(),
			}},
		},
		coretesting.ModelTag.Id(),
		func(*coresecrets.URI) error { return nil },
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})

}

func (s *secretsSuite) TestRemoveSecretNotFoundForModelAdmin(c *gc.C) {
	// Tests when we delete a secret with two revisions, neither of which are in the backend.
	// Asserts that the revisions are successfully removed from the juju db and that RemoveUserSecrets returns without
	// error.

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Data needed for mocks
	uri := coresecrets.NewURI()

	// Secret with two revisions with arbitrary revision numbers/IDs
	var revisionMetadata []*coresecrets.SecretRevisionMetadata
	revisionIDs := set.NewStrings()
	revisions := []int{5, 6}
	for _, rev := range revisions {
		revisionID := fmt.Sprintf("rev-%d", rev)
		revisionMetadata = append(revisionMetadata, &coresecrets.SecretRevisionMetadata{
			Revision: rev,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: revisionID},
		})
		revisionIDs.Add(revisionID)
	}

	backendCfg := &provider.ModelBackendConfig{
		ModelUUID: coretesting.ModelTag.Id(),
	}
	backend := mocks.NewMockSecretsBackend(ctrl)
	// backend.DeleteContent returns NotFound to any revisions to simulate a backend that has no secret revision data.
	// backend.DeleteContent should be called exactly once for each secret revision, and return NotFound for each to
	// simulate that the secret revision doesn't exist in this backend.
	for _, rev := range revisionMetadata {
		backend.EXPECT().DeleteContent(gomock.Any(), rev.ValueRef.RevisionID).Return(errors.NotFound)
	}

	// Provider which has our single backend that is missing the secret revisions we're trying to delete
	mockprovider := mocks.NewMockSecretBackendProvider(ctrl)
	// Mock the package GetProvider function to return the mockprovider
	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return mockprovider, nil })
	mockprovider.EXPECT().NewBackend(backendCfg).Return(backend, nil)

	removeState := mocks.NewMockSecretsRemoveState(ctrl)
	// removeState.GetSecret always confirms a secret exists
	removeState.EXPECT().GetSecret(gomock.Any()).AnyTimes().Return(&coresecrets.SecretMetadata{}, nil)
	// ListSecretRevisions reflects that we have the given revisions for our secret
	removeState.EXPECT().ListSecretRevisions(uri).Return(revisionMetadata, nil)
	// GetSecretRevision returns the original revision metadata to simulate that the database still thinks our secret is
	// in this backend.
	for _, rev := range revisionMetadata {
		removeState.EXPECT().GetSecretRevision(uri, rev.Revision).Return(rev, nil)
	}
	// Expect that we DeleteSecret from the juju db exactly once for the entire secret
	removeState.EXPECT().DeleteSecret(uri, []int{}).Return([]coresecrets.ValueRef{}, nil)

	adminConfigGetter := func() (*provider.ModelBackendConfigInfo, error) {
		return &provider.ModelBackendConfigInfo{
			ActiveID: "backend",
			Configs: map[string]provider.ModelBackendConfig{
				"backend-id": *backendCfg,
			},
		}, nil
	}

	results, err := secrets.RemoveUserSecrets(
		removeState, adminConfigGetter,
		params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{{
				URI: (*uri).String(),
			}},
		},
		coretesting.ModelTag.Id(),
		func(*coresecrets.URI) error { return nil },
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})

}
