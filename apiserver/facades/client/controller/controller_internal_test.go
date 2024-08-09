// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/juju/collections/set"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

var _ = gc.Suite(&controllerSuite{})

type controllerSuite struct{}

func (s *controllerSuite) TestUserListCompatibility(c *gc.C) {
	extProvider1 := "https://api.jujucharms.com/identity"
	extProvider2 := "http://candid.provider/identity"
	specs := []struct {
		descr    string
		src, dst userList
		expErr   string
	}{
		{
			descr: `all src users present in dst`,
			src: userList{
				users: set.NewStrings("foo", "bar"),
			},
			dst: userList{
				users: set.NewStrings("foo", "bar"),
			},
		},
		{
			descr: `local src users present in dst, and an external user has been granted access, and src/dst use the same identity provider url`,
			src: userList{
				users:       set.NewStrings("foo", "bar@external"),
				identityURL: extProvider1,
			},
			dst: userList{
				users:       set.NewStrings("foo"),
				identityURL: extProvider1,
			},
		},
		{
			descr: `some local src users not present in dst`,
			src: userList{
				users: set.NewStrings("foo", "bar"),
			},
			dst: userList{
				users: set.NewStrings("bar"),
			},
			expErr: `cannot initiate migration as the users granted access to the model do not exist
on the destination controller. To resolve this issue you can add the following
users to the destination controller or remove them from the current model:
  - foo`,
		},
		{
			descr: `local src users present in dst, and an external user has been granted access, and src/dst use different identity provider URL`,
			src: userList{
				users:       set.NewStrings("foo", "bar@external"),
				identityURL: extProvider1,
			},
			dst: userList{
				users:       set.NewStrings("foo", "bar@external"),
				identityURL: extProvider2,
			},
			expErr: `cannot initiate migration as external users have been granted access to the model
and the two controllers have different identity provider configurations. To resolve
this issue you can remove the following users from the current model:
  - bar@external`,
		},
		{
			descr: `not all local src users present in dst, and an external user has been granted access, and src/dst use different identity provider URL`,
			src: userList{
				users:       set.NewStrings("foo", "bar@external"),
				identityURL: extProvider1,
			},
			dst: userList{
				users:       set.NewStrings("baz", "bar@external"),
				identityURL: extProvider2,
			},
			expErr: `cannot initiate migration as external users have been granted access to the model
and the two controllers have different identity provider configurations. To resolve
this issue you need to remove the following users from the current model:
  - bar@external

and add the following users to the destination controller or remove them from
the current model:
  - foo`,
		},
	}

	for specIndex, spec := range specs {
		c.Logf("test %d: %s", specIndex, spec.descr)

		err := spec.src.checkCompatibilityWith(spec.dst)
		if spec.expErr == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.Not(gc.Equals), nil)
			c.Assert(err.Error(), gc.Equals, spec.expErr)
		}
	}
}

func (s *controllerSuite) TestTargetToAPIInfoLocalUser(c *gc.C) {
	targetInfo := migration.TargetInfo{
		Addrs:     []string{"6.6.6.6"},
		CACert:    testing.CACert,
		AuthTag:   names.NewUserTag("fred"),
		Password:  "sekret",
		Macaroons: []macaroon.Slice{{}},
	}
	apiInfo := targetToAPIInfo(&targetInfo)
	c.Assert(apiInfo, jc.DeepEquals, &api.Info{
		Addrs:     targetInfo.Addrs,
		CACert:    targetInfo.CACert,
		Tag:       targetInfo.AuthTag,
		Password:  targetInfo.Password,
		Macaroons: targetInfo.Macaroons,
	})
}

func (s *controllerSuite) TestTargetToAPIInfoExternalUser(c *gc.C) {
	targetInfo := migration.TargetInfo{
		Addrs:     []string{"6.6.6.6"},
		CACert:    testing.CACert,
		AuthTag:   names.NewUserTag("fred@external"),
		Password:  "sekret",
		Macaroons: []macaroon.Slice{{}},
	}
	apiInfo := targetToAPIInfo(&targetInfo)
	c.Assert(apiInfo, jc.DeepEquals, &api.Info{
		Addrs:     targetInfo.Addrs,
		CACert:    targetInfo.CACert,
		Password:  targetInfo.Password,
		Macaroons: targetInfo.Macaroons,
	})
}

// controllerMockSuite is a test suite using mocked services instead of real
// ones. Tests should gradually be moved over here as we move functionality
// over to dqlite.
type controllerMockSuite struct {
	// TODO: remove this once we're fully on dqlite
	statetesting.StateSuite

	authorizer   apiservertesting.FakeAuthorizer
	modelService *MockModelService
	agentService *MockAgentService
}

var _ = gc.Suite(&controllerMockSuite{})

func (s *controllerMockSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      s.Owner,
		AdminTag: s.Owner,
	}
}

func (s *controllerMockSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelService = NewMockModelService(ctrl)
	s.agentService = NewMockAgentService(ctrl)
	return ctrl
}

func (s *controllerMockSuite) TestListBlockedModels(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Add blocks to controller model
	_ = s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	_ = s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	// Make new model and add blocks to it
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "test"})
	defer func() { _ = st.Close() }()
	_ = st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	_ = st.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	controllerAPI := ControllerAPI{
		state:        stateShim{s.State},
		authorizer:   s.authorizer,
		modelService: s.modelService,
	}

	controllerModelID := model.UUID(s.State.ModelUUID())
	s.modelService.EXPECT().Model(gomock.Any(), controllerModelID).Return(model.Model{
		Name:      "controller",
		UUID:      controllerModelID,
		OwnerName: s.Owner.Name(),
	}, nil)
	modelID := model.UUID(st.ModelUUID())
	s.modelService.EXPECT().Model(gomock.Any(), modelID).Return(model.Model{
		Name:      "test",
		UUID:      modelID,
		OwnerName: s.Owner.Name(),
	}, nil)

	blockList, err := controllerAPI.ListBlockedModels(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(blockList, jc.DeepEquals, params.ModelBlockInfoList{
		Models: []params.ModelBlockInfo{{
			Name:     "controller",
			UUID:     s.State.ModelUUID(),
			OwnerTag: s.Owner.String(),
			Blocks: []string{
				"BlockDestroy",
				"BlockChange",
			},
		}, {
			Name:     "test",
			UUID:     st.ModelUUID(),
			OwnerTag: s.Owner.String(),
			Blocks: []string{
				"BlockDestroy",
				"BlockChange",
			},
		}},
	})
}

func (s *controllerMockSuite) TestInitiateMigration(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Create two hosted models to migrate.
	st1 := s.Factory.MakeModel(c, nil)
	defer func() { _ = st1.Close() }()
	model1, err := st1.Model()
	c.Assert(err, jc.ErrorIsNil)

	st2 := s.Factory.MakeModel(c, nil)
	defer func() { _ = st2.Close() }()
	model2, err := st2.Model()
	c.Assert(err, jc.ErrorIsNil)

	mac, err := macaroon.New([]byte("secret"), []byte("id"), "location", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	macsJSON, err := json.Marshal([]macaroon.Slice{{mac}})
	c.Assert(err, jc.ErrorIsNil)

	SetPrecheckResult(s, nil)

	// Kick off migrations
	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{
			{
				ModelTag: model1.ModelTag().String(),
				TargetInfo: params.MigrationTargetInfo{
					ControllerTag:   randomControllerTag(),
					ControllerAlias: "", // intentionally left empty; simulates older client
					Addrs:           []string{"1.1.1.1:1111", "2.2.2.2:2222"},
					CACert:          "cert1",
					AuthTag:         names.NewUserTag("admin1").String(),
					Password:        "secret1",
				},
			}, {
				ModelTag: model2.ModelTag().String(),
				TargetInfo: params.MigrationTargetInfo{
					ControllerTag:   randomControllerTag(),
					ControllerAlias: "target-controller",
					Addrs:           []string{"3.3.3.3:3333"},
					CACert:          "cert2",
					AuthTag:         names.NewUserTag("admin2").String(),
					Macaroons:       string(macsJSON),
					Password:        "secret2",
				},
			},
		},
	}

	s.modelService.EXPECT().Model(gomock.Any(), model.UUID(model1.UUID())).Return(model.Model{}, nil)
	s.modelService.EXPECT().Model(gomock.Any(), model.UUID(model2.UUID())).Return(model.Model{}, nil)

	controllerAPI := ControllerAPI{
		statePool:    s.StatePool,
		authorizer:   s.authorizer,
		apiUser:      s.Owner,
		modelService: s.modelService,
		leadership:   noopLeadershipReader{},
	}

	out, err := controllerAPI.InitiateMigration(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.Results, gc.HasLen, 2)

	states := []*state.State{st1, st2}
	for i, spec := range args.Specs {
		c.Log(i)
		st := states[i]
		result := out.Results[i]

		c.Assert(result.Error, gc.IsNil)
		c.Check(result.ModelTag, gc.Equals, spec.ModelTag)
		expectedId := st.ModelUUID() + ":0"
		c.Check(result.MigrationId, gc.Equals, expectedId)

		// Ensure the migration made it into the DB correctly.
		mig, err := st.LatestMigration()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(mig.Id(), gc.Equals, expectedId)
		c.Check(mig.ModelUUID(), gc.Equals, st.ModelUUID())
		c.Check(mig.InitiatedBy(), gc.Equals, s.Owner.Id())

		targetInfo, err := mig.TargetInfo()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(targetInfo.ControllerTag.String(), gc.Equals, spec.TargetInfo.ControllerTag)
		c.Check(targetInfo.ControllerAlias, gc.Equals, spec.TargetInfo.ControllerAlias)
		c.Check(targetInfo.Addrs, jc.SameContents, spec.TargetInfo.Addrs)
		c.Check(targetInfo.CACert, gc.Equals, spec.TargetInfo.CACert)
		c.Check(targetInfo.AuthTag.String(), gc.Equals, spec.TargetInfo.AuthTag)
		c.Check(targetInfo.Password, gc.Equals, spec.TargetInfo.Password)

		if spec.TargetInfo.Macaroons != "" {
			macJSONdb, err := json.Marshal(targetInfo.Macaroons)
			c.Assert(err, jc.ErrorIsNil)
			c.Check(string(macJSONdb), gc.Equals, spec.TargetInfo.Macaroons)
		}
	}
}

func (s *controllerMockSuite) TestInitiateMigrationPartialFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer func() { _ = st.Close() }()
	SetPrecheckResult(s, nil)

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	nonexistentModelID := modeltesting.GenModelUUID(c)
	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{
			{
				ModelTag: m.ModelTag().String(),
				TargetInfo: params.MigrationTargetInfo{
					ControllerTag: randomControllerTag(),
					Addrs:         []string{"1.1.1.1:1111", "2.2.2.2:2222"},
					CACert:        "cert",
					AuthTag:       names.NewUserTag("admin").String(),
					Password:      "secret",
				},
			}, {
				ModelTag: names.NewModelTag(nonexistentModelID.String()).String(), // Doesn't exist.
			},
		},
	}

	s.modelService.EXPECT().Model(gomock.Any(), model.UUID(m.UUID())).Return(model.Model{}, nil)
	s.modelService.EXPECT().Model(gomock.Any(), nonexistentModelID).Return(model.Model{}, modelerrors.NotFound)

	controllerAPI := ControllerAPI{
		statePool:    s.StatePool,
		authorizer:   s.authorizer,
		apiUser:      s.Owner,
		modelService: s.modelService,
		leadership:   noopLeadershipReader{},
	}

	out, err := controllerAPI.InitiateMigration(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.Results, gc.HasLen, 2)

	c.Check(out.Results[0].ModelTag, gc.Equals, m.ModelTag().String())
	c.Check(out.Results[0].Error, gc.IsNil)

	c.Check(out.Results[1].ModelTag, gc.Equals, args.Specs[1].ModelTag)
	c.Check(out.Results[1].Error, gc.ErrorMatches, ".* model not found")
}

func (s *controllerMockSuite) TestInitiateMigrationInvalidMacaroons(c *gc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{
			{
				ModelTag: m.ModelTag().String(),
				TargetInfo: params.MigrationTargetInfo{
					ControllerTag: randomControllerTag(),
					Addrs:         []string{"1.1.1.1:1111", "2.2.2.2:2222"},
					CACert:        "cert",
					AuthTag:       names.NewUserTag("admin").String(),
					Macaroons:     "BLAH",
				},
			},
		},
	}

	s.modelService.EXPECT().Model(gomock.Any(), model.UUID(m.UUID())).Return(model.Model{}, nil)

	controllerAPI := ControllerAPI{
		statePool:    s.StatePool,
		authorizer:   s.authorizer,
		modelService: s.modelService,
	}

	out, err := controllerAPI.InitiateMigration(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.Results, gc.HasLen, 1)
	result := out.Results[0]
	c.Check(result.ModelTag, gc.Equals, args.Specs[0].ModelTag)
	c.Check(result.Error, gc.ErrorMatches, "invalid macaroons: .+")
}

func (s *controllerMockSuite) TestInitiateMigrationSpecError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Create a hosted model to migrate.
	st := s.Factory.MakeModel(c, nil)
	defer func() { _ = st.Close() }()
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	controllerAPI := ControllerAPI{
		statePool:    s.StatePool,
		authorizer:   s.authorizer,
		modelService: s.modelService,
	}

	s.modelService.EXPECT().Model(gomock.Any(), model.UUID(m.UUID()))

	// Kick off the migration with missing details.
	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{{
			ModelTag: m.ModelTag().String(),
			// TargetInfo missing
		}},
	}
	out, err := controllerAPI.InitiateMigration(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.Results, gc.HasLen, 1)
	result := out.Results[0]
	c.Check(result.ModelTag, gc.Equals, args.Specs[0].ModelTag)
	c.Check(result.MigrationId, gc.Equals, "")
	c.Check(result.Error, gc.ErrorMatches, "controller tag: .+ is not a valid tag")
}

func (s *controllerMockSuite) TestInitiateMigrationPrecheckFail(c *gc.C) {
	defer s.setupMocks(c).Finish()

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	SetPrecheckResult(s, errors.New("boom"))
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	controllerAPI := ControllerAPI{
		statePool:    s.StatePool,
		authorizer:   s.authorizer,
		modelService: s.modelService,
		agentService: s.agentService,
		leadership:   noopLeadershipReader{},
	}

	modelID := model.UUID(m.UUID())
	s.modelService.EXPECT().Model(gomock.Any(), modelID)
	// TODO: define return values on agent service

	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{{
			ModelTag: m.ModelTag().String(),
			TargetInfo: params.MigrationTargetInfo{
				ControllerTag: randomControllerTag(),
				Addrs:         []string{"1.1.1.1:1111"},
				CACert:        "cert1",
				AuthTag:       names.NewUserTag("admin1").String(),
				Password:      "secret1",
			},
		}},
	}
	out, err := controllerAPI.InitiateMigration(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.Results, gc.HasLen, 1)
	c.Check(out.Results[0].Error, gc.ErrorMatches, "boom")

	active, err := st.IsMigrationActive()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(active, jc.IsFalse)
}

func (s *controllerMockSuite) TestHostedModelConfigs(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// ControllerModel is already set up in state
	controllerModelUUID := s.State.ControllerModelUUID()
	s.modelService.EXPECT().ControllerModel(gomock.Any()).Return(model.Model{
		UUID: model.UUID(controllerModelUUID),
	}, nil)

	// Set up another model in state
	st := s.Factory.MakeModel(c, &factory.ModelParams{Name: "first"})
	defer st.Close()
	hostedModelUUID := model.UUID(st.ModelUUID())
	s.modelService.EXPECT().Model(gomock.Any(), hostedModelUUID).Return(model.Model{
		Name:      "first",
		UUID:      hostedModelUUID,
		OwnerName: "username-3",
	}, nil)

	modelConfig, err := config.New(config.UseDefaults, map[string]any{
		"name": "first",
		"uuid": hostedModelUUID.String(),
		"type": "ec2",
	})
	c.Assert(err, jc.ErrorIsNil)
	modelConfigService := NewMockModelConfigService(ctrl)
	modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(modelConfig, nil)

	modelConfigServiceGetter := func(modelID model.UUID) ModelConfigService {
		switch modelID {
		case hostedModelUUID:
			return modelConfigService
		default:
			c.Fatalf("modelConfigServiceGetter called for model ID %q", modelID)
		}
		return nil
	}

	cloudSpec := &params.CloudSpec{
		Name:             "first",
		Type:             "ec2",
		Region:           "region",
		Endpoint:         "endpoint",
		IdentityEndpoint: "identity-endpoint",
		StorageEndpoint:  "storage-endpoint",
		Credential: &params.CloudCredential{
			AuthType: "auth-type",
			Attributes: map[string]string{
				"username": "foo",
				"password": "bar",
				"Token":    "token",
			},
		},
		CACertificates: []string{testing.CACert},
		SkipTLSVerify:  true,
	}
	cloudSpecer := NewMockCloudSpecer(ctrl)
	cloudSpecer.EXPECT().GetCloudSpec(gomock.Any(), names.NewModelTag(hostedModelUUID.String())).
		Return(params.CloudSpecResult{Result: cloudSpec})

	controllerAPI := ControllerAPI{
		CloudSpecer:              cloudSpecer,
		state:                    &stateShim{s.State},
		authorizer:               s.authorizer,
		modelService:             s.modelService,
		modelConfigServiceGetter: modelConfigServiceGetter,
	}

	results, err := controllerAPI.HostedModelConfigs(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, params.HostedModelConfigsResults{
		Models: []params.HostedModelConfig{
			{
				Name:      "first",
				OwnerTag:  "user-username-3",
				Config:    modelConfig.AllAttrs(),
				CloudSpec: cloudSpec,
				Error:     nil,
			},
		},
	})
}

func randomControllerTag() string {
	uuid := uuid.MustNewUUID().String()
	return names.NewControllerTag(uuid).String()
}

type noopLeadershipReader struct {
	leadership.Reader
}

func (noopLeadershipReader) Leaders() (map[string]string, error) {
	return make(map[string]string), nil
}
