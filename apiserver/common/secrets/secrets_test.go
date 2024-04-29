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

	"github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/apiserver/common/secrets/mocks"
	"github.com/juju/juju/core/leadership"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	"github.com/juju/juju/internal/secrets/provider/vault"
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

func (s *secretsSuite) TestBackendConfigInfoLeaderUnit(c *gc.C) {
	s.assertBackendConfigInfoLeaderUnit(c, []string{"backend-id"})
}

func (s *secretsSuite) TestBackendConfigInfoDefaultAdmin(c *gc.C) {
	s.assertBackendConfigInfoLeaderUnit(c, nil)
}

func (s *secretsSuite) assertBackendConfigInfoLeaderUnit(c *gc.C, wanted []string) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	accessor := coresecrets.Accessor{
		Kind: coresecrets.UnitAccessor,
		ID:   "gitlab/0",
	}
	model := mocks.NewMockModel(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	token := mocks.NewMockToken(ctrl)
	p := mocks.NewMockSecretBackendProvider(ctrl)
	secretService := mocks.NewMockSecretService(ctrl)
	secretBackendService := mocks.NewMockSecretBackendService(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return p, nil })

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
	secretBackendService.EXPECT().GetSecretBackendConfigForAdmin(gomock.Any(), coremodel.UUID(coretesting.ModelTag.Id())).Return(
		&provider.ModelBackendConfigInfo{
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
				"backend-id2": {
					ControllerUUID: coretesting.ControllerTag.Id(),
					ModelUUID:      coretesting.ModelTag.Id(),
					ModelName:      "fred",
					BackendConfig: provider.BackendConfig{
						BackendType: "some-backend2",
					},
				},
			},
		}, nil)
	p.EXPECT().Initialise(gomock.Any()).Return(nil)
	leadershipChecker.EXPECT().LeadershipCheck("gitlab", "gitlab/0").Return(token)
	token.EXPECT().Check().Return(nil)

	secretService.EXPECT().ListGrantedSecretsForBackend(gomock.Any(), "backend-id", coresecrets.RoleManage, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   accessor.ID,
	}, secretservice.SecretAccessor{
		Kind: secretservice.ApplicationAccessor,
		ID:   "gitlab",
	}).Return(owned, nil)
	secretService.EXPECT().ListGrantedSecretsForBackend(gomock.Any(), "backend-id", coresecrets.RoleView, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   accessor.ID,
	}, secretservice.SecretAccessor{
		Kind: secretservice.ApplicationAccessor,
		ID:   "gitlab",
	}).Return(read, nil)
	p.EXPECT().RestrictedConfig(context.Background(), &adminCfg, true, false, accessor, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil)

	unitTag := names.NewUnitTag("gitlab/0")
	info, err := secrets.BackendConfigInfo(context.Background(), model, true, secretService, secretBackendService, wanted, false, unitTag, leadershipChecker)
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

	accessor := coresecrets.Accessor{
		Kind: coresecrets.UnitAccessor,
		ID:   "gitlab/0",
	}
	model := mocks.NewMockModel(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	token := mocks.NewMockToken(ctrl)
	p := mocks.NewMockSecretBackendProvider(ctrl)
	secretService := mocks.NewMockSecretService(ctrl)
	secretBackendService := mocks.NewMockSecretBackendService(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return p, nil })

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
	secretBackendService.EXPECT().GetSecretBackendConfigForAdmin(gomock.Any(), coremodel.UUID(coretesting.ModelTag.Id())).Return(
		&provider.ModelBackendConfigInfo{
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
		}, nil)
	p.EXPECT().Initialise(gomock.Any()).Return(nil)
	leadershipChecker.EXPECT().LeadershipCheck("gitlab", "gitlab/0").Return(token)
	token.EXPECT().Check().Return(leadership.NewNotLeaderError("", ""))

	secretService.EXPECT().ListGrantedSecretsForBackend(gomock.Any(), "backend-id", coresecrets.RoleView, secretservice.SecretAccessor{
		Kind: secretservice.ApplicationAccessor,
		ID:   "gitlab",
	}).Return(appOwned, nil)
	secretService.EXPECT().ListGrantedSecretsForBackend(gomock.Any(), "backend-id", coresecrets.RoleManage, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   "gitlab/0",
	}).Return(unitOwned, nil)
	secretService.EXPECT().ListGrantedSecretsForBackend(gomock.Any(), "backend-id", coresecrets.RoleView, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   "gitlab/0",
	}).Return(read, nil)

	p.EXPECT().RestrictedConfig(context.Background(), &adminCfg, true, false, accessor, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil)

	unitTag := names.NewUnitTag("gitlab/0")
	info, err := secrets.BackendConfigInfo(context.Background(), model, true, secretService, secretBackendService, []string{"backend-id"}, false, unitTag, leadershipChecker)
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

	accessor := coresecrets.Accessor{
		Kind: coresecrets.UnitAccessor,
		ID:   "gitlab/0",
	}
	model := mocks.NewMockModel(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	token := mocks.NewMockToken(ctrl)
	p := mocks.NewMockSecretBackendProvider(ctrl)
	secretService := mocks.NewMockSecretService(ctrl)
	secretBackendService := mocks.NewMockSecretBackendService(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return p, nil })

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
	secretBackendService.EXPECT().GetSecretBackendConfigForAdmin(gomock.Any(), coremodel.UUID(coretesting.ModelTag.Id())).Return(
		&provider.ModelBackendConfigInfo{
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
		}, nil)
	p.EXPECT().Initialise(gomock.Any()).Return(nil)
	leadershipChecker.EXPECT().LeadershipCheck("gitlab", "gitlab/0").Return(token)
	token.EXPECT().Check().Return(leadership.NewNotLeaderError("", ""))

	secretService.EXPECT().ListGrantedSecretsForBackend(gomock.Any(), "backend-id", coresecrets.RoleManage, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   accessor.ID,
	}).Return(unitOwned, nil)
	secretService.EXPECT().ListGrantedSecretsForBackend(gomock.Any(), "backend-id", coresecrets.RoleView, secretservice.SecretAccessor{
		Kind: secretservice.ApplicationAccessor,
		ID:   "gitlab",
	}).Return(appOwned, nil)
	secretService.EXPECT().ListGrantedSecretsForBackend(gomock.Any(), "backend-id", coresecrets.RoleView, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   accessor.ID,
	}, secretservice.SecretAccessor{
		Kind: secretservice.ApplicationAccessor,
		ID:   "gitlab",
	}).Return(read, nil)

	p.EXPECT().RestrictedConfig(context.Background(), &adminCfg, true, true, accessor, ownedRevs, readRevs).Return(&adminCfg.BackendConfig, nil)

	unitTag := names.NewUnitTag("gitlab/0")
	info, err := secrets.DrainBackendConfigInfo(context.Background(), "backend-id", model, secretService, secretBackendService, unitTag, leadershipChecker)
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
	secretService := mocks.NewMockSecretService(ctrl)
	secretBackendService := mocks.NewMockSecretBackendService(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return p, nil })

	model.EXPECT().ControllerUUID().Return(coretesting.ControllerTag.Id()).AnyTimes()
	model.EXPECT().UUID().Return(coretesting.ModelTag.Id()).AnyTimes()
	model.EXPECT().Name().Return("fred").AnyTimes()
	secretBackendService.EXPECT().GetSecretBackendConfigForAdmin(gomock.Any(), coremodel.UUID(coretesting.ModelTag.Id())).Return(
		&provider.ModelBackendConfigInfo{
			ActiveID: "backend-id",
			Configs: map[string]provider.ModelBackendConfig{
				"some-id": {
					ControllerUUID: coretesting.ControllerTag.Id(),
					ModelUUID:      coretesting.ModelTag.Id(),
					ModelName:      "fred",
					BackendConfig: provider.BackendConfig{
						BackendType: vault.BackendType,
						Config: map[string]interface{}{
							"endpoint": "http://vault",
						}},
				},
			},
		}, nil)
	p.EXPECT().Initialise(gomock.Any()).Return(nil)

	_, err := secrets.BackendConfigInfo(context.Background(), model, true, secretService, secretBackendService, []string{"some-id"}, false, badTag, leadershipChecker)
	c.Assert(err, gc.ErrorMatches, `login as "user-foo" not supported`)
}
