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
	"github.com/juju/juju/core/leadership"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type secretsSuite struct {
	testing.IsolationSuite
	coretesting.JujuOSEnvSuite
}

var _ = gc.Suite(&secretsSuite{})

func (s *secretsSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags(feature.DeveloperMode)
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
}

func (s *secretsSuite) TestProviderInfoForModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)

	cfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"secret-store": "vault",
	})
	gomock.InOrder(
		model.EXPECT().Config().Return(cfg, nil),
	)
	p, _, err := secrets.ProviderInfoForModel(model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p.Type(), gc.Equals, "vault")
}

func (s *secretsSuite) TestProviderInfoForModelJujuDefault(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)

	cfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"secret-store": "",
	})
	gomock.InOrder(
		model.EXPECT().Config().Return(cfg, nil),
		model.EXPECT().Type().Return(state.ModelTypeIAAS),

		model.EXPECT().Config().Return(cfg, nil),
		model.EXPECT().Type().Return(state.ModelTypeCAAS),
	)
	p, _, err := secrets.ProviderInfoForModel(model)
	c.Check(err, jc.ErrorIsNil)
	c.Check(p.Type(), gc.Equals, "juju")

	p, _, err = secrets.ProviderInfoForModel(model)
	c.Check(err, jc.ErrorIsNil)
	c.Check(p.Type(), gc.Equals, "kubernetes")
}

func (s *secretsSuite) TestProviderInfoForModelK8sDefault(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)

	s.SetFeatureFlags(feature.DeveloperMode)
	gomock.InOrder(
		model.EXPECT().Config().Return(coretesting.ModelConfig(c), nil),
		model.EXPECT().Type().Return(state.ModelTypeCAAS),
	)
	p, _, err := secrets.ProviderInfoForModel(model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p.Type(), gc.Equals, "kubernetes")
}

func (s *secretsSuite) TestStoreConfigLeaderUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("gitlab/0")
	model := mocks.NewMockModel(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	token := mocks.NewMockToken(ctrl)
	p := mocks.NewMockSecretStoreProvider(ctrl)
	backend := mocks.NewMockSecretsStore(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretStoreProvider, error) { return p, nil })
	s.PatchValue(&secrets.GetStateBackEnd, func(secrets.Model) state.SecretsStore { return backend })

	owned := []*coresecrets.SecretMetadata{
		{URI: &coresecrets.URI{ID: "owned-1"}, LatestRevision: 1},
	}
	read := []*coresecrets.SecretMetadata{
		{URI: &coresecrets.URI{ID: "read-1"}, LatestRevision: 2},
	}
	gomock.InOrder(
		model.EXPECT().Config().Return(coretesting.ModelConfig(c), nil),
		model.EXPECT().Type().Return(state.ModelTypeIAAS),

		p.EXPECT().Initialise(gomock.Any()).Return(nil),
		leadershipChecker.EXPECT().LeadershipCheck("gitlab", "gitlab/0").Return(token),
		token.EXPECT().Check().Return(nil),

		backend.EXPECT().ListSecrets(state.SecretsFilter{
			OwnerTags: []names.Tag{
				unitTag, names.NewApplicationTag("gitlab"),
			},
		}).Return(owned, nil),
		backend.EXPECT().ListSecrets(state.SecretsFilter{
			ConsumerTags: []names.Tag{unitTag, names.NewApplicationTag("gitlab")},
		}).Return(read, nil),
		p.EXPECT().StoreConfig(gomock.Any(), unitTag,
			provider.SecretRevisions{"owned-1": set.NewInts(1)},
			provider.SecretRevisions{"read-1": set.NewInts(2)},
		).Return(nil, nil),
	)

	_, err := secrets.StoreConfig(model, unitTag, leadershipChecker)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *secretsSuite) TestStoreConfigNonLeaderUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("gitlab/0")
	model := mocks.NewMockModel(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	token := mocks.NewMockToken(ctrl)
	p := mocks.NewMockSecretStoreProvider(ctrl)
	backend := mocks.NewMockSecretsStore(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretStoreProvider, error) { return p, nil })
	s.PatchValue(&secrets.GetStateBackEnd, func(secrets.Model) state.SecretsStore { return backend })

	unitOwned := []*coresecrets.SecretMetadata{
		{URI: &coresecrets.URI{ID: "owned-1"}, LatestRevision: 1},
	}
	appOwned := []*coresecrets.SecretMetadata{
		{URI: &coresecrets.URI{ID: "app-owned-1"}, LatestRevision: 1},
	}
	read := []*coresecrets.SecretMetadata{
		{URI: &coresecrets.URI{ID: "read-1"}, LatestRevision: 2},
	}
	gomock.InOrder(
		model.EXPECT().Config().Return(coretesting.ModelConfig(c), nil),
		model.EXPECT().Type().Return(state.ModelTypeIAAS),

		p.EXPECT().Initialise(gomock.Any()).Return(nil),
		leadershipChecker.EXPECT().LeadershipCheck("gitlab", "gitlab/0").Return(token),
		token.EXPECT().Check().Return(leadership.NewNotLeaderError("", "")),

		backend.EXPECT().ListSecrets(state.SecretsFilter{
			OwnerTags: []names.Tag{unitTag},
		}).Return(unitOwned, nil),
		backend.EXPECT().ListSecrets(state.SecretsFilter{
			ConsumerTags: []names.Tag{unitTag, names.NewApplicationTag("gitlab")},
		}).Return(read, nil),
		backend.EXPECT().ListSecrets(state.SecretsFilter{
			OwnerTags: []names.Tag{names.NewApplicationTag("gitlab")},
		}).Return(appOwned, nil),
		p.EXPECT().StoreConfig(gomock.Any(), unitTag,
			provider.SecretRevisions{"owned-1": set.NewInts(1)},
			provider.SecretRevisions{"read-1": set.NewInts(2), "app-owned-1": set.NewInts(1)},
		).Return(nil, nil),
	)

	_, err := secrets.StoreConfig(model, unitTag, leadershipChecker)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *secretsSuite) TestStoreConfigAppTagLogin(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appTag := names.NewApplicationTag("gitlab")
	model := mocks.NewMockModel(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	p := mocks.NewMockSecretStoreProvider(ctrl)
	backend := mocks.NewMockSecretsStore(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretStoreProvider, error) { return p, nil })
	s.PatchValue(&secrets.GetStateBackEnd, func(secrets.Model) state.SecretsStore { return backend })

	owned := []*coresecrets.SecretMetadata{
		{URI: &coresecrets.URI{ID: "owned-1"}, LatestRevision: 1},
	}
	read := []*coresecrets.SecretMetadata{
		{URI: &coresecrets.URI{ID: "read-1"}, LatestRevision: 2},
	}
	gomock.InOrder(
		model.EXPECT().Config().Return(coretesting.ModelConfig(c), nil),
		model.EXPECT().Type().Return(state.ModelTypeIAAS),

		p.EXPECT().Initialise(gomock.Any()).Return(nil),

		backend.EXPECT().ListSecrets(state.SecretsFilter{
			OwnerTags: []names.Tag{appTag},
		}).Return(owned, nil),
		backend.EXPECT().ListSecrets(state.SecretsFilter{
			ConsumerTags: []names.Tag{appTag},
		}).Return(read, nil),
		p.EXPECT().StoreConfig(gomock.Any(), appTag,
			provider.SecretRevisions{"owned-1": set.NewInts(1)},
			provider.SecretRevisions{"read-1": set.NewInts(2)},
		).Return(nil, nil),
	)

	_, err := secrets.StoreConfig(model, appTag, leadershipChecker)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *secretsSuite) TestStoreConfigFailedInvalidAuthTag(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	badTag := names.NewUserTag("foo")
	model := mocks.NewMockModel(ctrl)
	leadershipChecker := mocks.NewMockChecker(ctrl)
	p := mocks.NewMockSecretStoreProvider(ctrl)
	backend := mocks.NewMockSecretsStore(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretStoreProvider, error) { return p, nil })
	s.PatchValue(&secrets.GetStateBackEnd, func(secrets.Model) state.SecretsStore { return backend })

	gomock.InOrder(
		model.EXPECT().Config().Return(coretesting.ModelConfig(c), nil),
		model.EXPECT().Type().Return(state.ModelTypeIAAS),

		p.EXPECT().Initialise(gomock.Any()).Return(nil),
	)

	_, err := secrets.StoreConfig(model, badTag, leadershipChecker)
	c.Assert(err, gc.ErrorMatches, `login as "user-foo" not supported`)
}

func (s *secretsSuite) TestStoreForInspect(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)
	p := mocks.NewMockSecretStoreProvider(ctrl)

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretStoreProvider, error) { return p, nil })

	storeCfg := &provider.StoreConfig{StoreType: "juju"}
	gomock.InOrder(
		model.EXPECT().Config().Return(coretesting.ModelConfig(c), nil),
		model.EXPECT().Type().Return(state.ModelTypeIAAS),

		p.EXPECT().Initialise(gomock.Any()).Return(nil),
		p.EXPECT().StoreConfig(gomock.Any(), nil, nil, nil).Return(storeCfg, nil),
		p.EXPECT().NewStore(storeCfg).Return(nil, nil),
	)

	_, err := secrets.StoreForInspect(model)
	c.Assert(err, jc.ErrorIsNil)
}
