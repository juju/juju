// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	commonmocks "github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/apiserver/common/secrets/mocks"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/leadership"
	coresecrets "github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/secrets/provider/vault"
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
				"endpoint":            "http://nowhere",
				"ca-certs":            []string{"cert-data"},
				"credential":          `{"auth-type":"access-key","Attributes":{"foo":"bar"}}`,
				"is-controller-cloud": true,
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
	cloudService := commonmocks.NewMockCloudService(ctrl)
	credentialService := commonmocks.NewMockCredentialService(ctrl)

	s.PatchValue(&secrets.GetSecretBackendsState, func(secrets.Model) state.SecretBackendsStorage { return backendState })

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
		model.EXPECT().CloudName().Return("test")
		cloudService.EXPECT().Cloud(gomock.Any(), "test").Return(&cld, nil)
		tag := names.NewCloudCredentialTag("test/fred/default")
		model.EXPECT().CloudCredentialTag().Return(tag, true)
		cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{"foo": "bar"})
		credentialService.EXPECT().CloudCredential(gomock.Any(), credential.KeyFromTag(tag)).Return(cred, nil)
	}

	backendState.EXPECT().ListSecretBackends().Return([]*coresecrets.SecretBackend{{
		ID:          vaultBackendID,
		Name:        "myvault",
		BackendType: vault.BackendType,
		Config: map[string]interface{}{
			"endpoint": "http://vault",
		},
	}}, nil)

	info, err := secrets.AdminBackendConfigInfo(context.Background(), model, cloudService, credentialService)
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
	secretService := mocks.NewMockSecretService(ctrl)
	cloudService := commonmocks.NewMockCloudService(ctrl)
	credentialService := commonmocks.NewMockCredentialService(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return p, nil })
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
	model.EXPECT().Config().Return(modelCfg, nil)
	model.EXPECT().Type().Return(state.ModelTypeIAAS)
	backendState.EXPECT().ListSecretBackends().Return([]*coresecrets.SecretBackend{{
		ID:          "backend-id",
		Name:        "backend-name",
		BackendType: "some-backend",
	}, {
		ID:          "backend-id2",
		Name:        "backend-name2",
		BackendType: "some-backend2",
	}}, nil)
	p.EXPECT().Initialise(gomock.Any()).Return(nil)
	leadershipChecker.EXPECT().LeadershipCheck("gitlab", "gitlab/0").Return(token)
	token.EXPECT().Check().Return(nil)

	secretService.EXPECT().ListCharmSecrets(gomock.Any(), []secretservice.CharmSecretOwner{{
		Kind: secretservice.UnitOwner,
		ID:   unitTag.Id(),
	}, {
		Kind: secretservice.ApplicationOwner,
		ID:   "gitlab",
	}}).Return(owned, [][]*coresecrets.SecretRevisionMetadata{{
		{
			Revision: 1,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "owned-rev-1"},
		}, {
			Revision: 2,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "owned-rev-2"},
		},
	}}, nil)
	secretService.EXPECT().ListConsumedSecrets(gomock.Any(), secretservice.SecretConsumer{
		ApplicationName: ptr("gitlab"),
		UnitName:        ptr(unitTag.Id()),
	}).Return(read, [][]*coresecrets.SecretRevisionMetadata{{
		{
			Revision: 1,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "read-rev-1"},
		},
	}}, nil)
	p.EXPECT().RestrictedConfig(context.Background(), &adminCfg, true, false, unitTag, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil)

	info, err := secrets.BackendConfigInfo(context.Background(), model, true, secretService, cloudService, credentialService, wanted, false, unitTag, leadershipChecker)
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
	secretService := mocks.NewMockSecretService(ctrl)
	cloudService := commonmocks.NewMockCloudService(ctrl)
	credentialService := commonmocks.NewMockCredentialService(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return p, nil })
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
	model.EXPECT().Config().Return(modelCfg, nil)
	model.EXPECT().Type().Return(state.ModelTypeIAAS)
	backendState.EXPECT().ListSecretBackends().Return([]*coresecrets.SecretBackend{{
		ID:          "backend-id",
		Name:        "backend-name",
		BackendType: "some-backend",
	}}, nil)
	p.EXPECT().Initialise(gomock.Any()).Return(nil)
	leadershipChecker.EXPECT().LeadershipCheck("gitlab", "gitlab/0").Return(token)
	token.EXPECT().Check().Return(leadership.NewNotLeaderError("", ""))

	secretService.EXPECT().ListCharmSecrets(gomock.Any(), []secretservice.CharmSecretOwner{{
		Kind: secretservice.UnitOwner,
		ID:   unitTag.Id(),
	}}).Return(unitOwned, [][]*coresecrets.SecretRevisionMetadata{{
		{
			Revision: 1,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "owned-rev-1"},
		}, {
			Revision: 2,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "owned-rev-2"},
		},
	}}, nil)
	secretService.EXPECT().ListCharmSecrets(gomock.Any(), []secretservice.CharmSecretOwner{{
		Kind: secretservice.ApplicationOwner,
		ID:   "gitlab",
	}}).Return(appOwned, [][]*coresecrets.SecretRevisionMetadata{{
		{
			Revision: 1,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "app-owned-rev-1"},
		}, {
			Revision: 2,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "app-owned-rev-2"},
		}, {
			Revision: 3,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "app-owned-rev-3"},
		},
	}}, nil)
	secretService.EXPECT().ListConsumedSecrets(gomock.Any(), secretservice.SecretConsumer{
		UnitName:        ptr(unitTag.Id()),
		ApplicationName: ptr("gitlab"),
	}).Return(read, [][]*coresecrets.SecretRevisionMetadata{{
		{
			Revision: 1,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "read-rev-1"},
		},
	}}, nil)
	p.EXPECT().RestrictedConfig(context.Background(), &adminCfg, true, false, unitTag, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil)

	info, err := secrets.BackendConfigInfo(context.Background(), model, true, secretService, cloudService, credentialService, []string{"backend-id"}, false, unitTag, leadershipChecker)
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
	secretService := mocks.NewMockSecretService(ctrl)
	cloudService := commonmocks.NewMockCloudService(ctrl)
	credentialService := commonmocks.NewMockCredentialService(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return p, nil })
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

	model.EXPECT().Config().Return(modelCfg, nil)
	model.EXPECT().Type().Return(state.ModelTypeIAAS)
	backendState.EXPECT().ListSecretBackends().Return([]*coresecrets.SecretBackend{{
		ID:          "backend-id",
		Name:        "backend-name",
		BackendType: "some-backend",
	}}, nil)
	p.EXPECT().Initialise(gomock.Any()).Return(nil)
	leadershipChecker.EXPECT().LeadershipCheck("gitlab", "gitlab/0").Return(token)
	token.EXPECT().Check().Return(leadership.NewNotLeaderError("", ""))

	secretService.EXPECT().ListCharmSecrets(gomock.Any(), []secretservice.CharmSecretOwner{{
		Kind: secretservice.UnitOwner,
		ID:   unitTag.Id(),
	}}).Return(unitOwned, [][]*coresecrets.SecretRevisionMetadata{{
		{
			Revision: 1,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "owned-rev-1"},
		}, {
			Revision: 2,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "owned-rev-2"},
		},
	}}, nil)
	secretService.EXPECT().ListCharmSecrets(gomock.Any(), []secretservice.CharmSecretOwner{{
		Kind: secretservice.ApplicationOwner,
		ID:   "gitlab",
	}}).Return(appOwned, [][]*coresecrets.SecretRevisionMetadata{{
		{
			Revision: 1,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "app-owned-rev-1"},
		}, {
			Revision: 2,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "app-owned-rev-2"},
		}, {
			Revision: 3,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "app-owned-rev-3"},
		},
	}}, nil)
	secretService.EXPECT().ListConsumedSecrets(gomock.Any(), secretservice.SecretConsumer{
		UnitName:        ptr(unitTag.Id()),
		ApplicationName: ptr("gitlab"),
	}).Return(read, [][]*coresecrets.SecretRevisionMetadata{{
		{
			Revision: 1,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "read-rev-1"},
		},
	}}, nil)

	p.EXPECT().RestrictedConfig(context.Background(), &adminCfg, true, true, unitTag, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil)

	info, err := secrets.DrainBackendConfigInfo(context.Background(), "backend-id", model, secretService, cloudService, credentialService, unitTag, leadershipChecker)
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
	secretService := mocks.NewMockSecretService(ctrl)
	cloudService := commonmocks.NewMockCloudService(ctrl)
	credentialService := commonmocks.NewMockCredentialService(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return p, nil })
	s.PatchValue(&secrets.GetSecretBackendsState, func(secrets.Model) state.SecretBackendsStorage { return backendState })

	cfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"secret-backend": "internal",
	})
	model.EXPECT().ControllerUUID().Return(coretesting.ControllerTag.Id()).AnyTimes()
	model.EXPECT().UUID().Return(coretesting.ModelTag.Id()).AnyTimes()
	model.EXPECT().Name().Return("fred").AnyTimes()
	model.EXPECT().Config().Return(cfg, nil)
	model.EXPECT().Type().Return(state.ModelTypeIAAS)
	backendState.EXPECT().ListSecretBackends().Return([]*coresecrets.SecretBackend{{
		ID:          "some-id",
		Name:        "myvault",
		BackendType: vault.BackendType,
		Config: map[string]interface{}{
			"endpoint": "http://vault",
		},
	}}, nil)
	p.EXPECT().Initialise(gomock.Any()).Return(nil)

	_, err := secrets.BackendConfigInfo(context.Background(), model, true, secretService, cloudService, credentialService, []string{"some-id"}, false, badTag, leadershipChecker)
	c.Assert(err, gc.ErrorMatches, `login as "user-foo" not supported`)
}
