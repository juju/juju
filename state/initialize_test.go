// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
)

type InitializeSuite struct {
	gitjujutesting.MgoSuite
	testing.BaseSuite
	State *state.State
}

var _ = gc.Suite(&InitializeSuite{})

func (s *InitializeSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *InitializeSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *InitializeSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
}

func (s *InitializeSuite) openState(c *gc.C, modelTag names.ModelTag) {
	st, err := state.Open(
		modelTag,
		statetesting.NewMongoInfo(),
		mongotest.DialOpts(),
		state.Policy(nil),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.State = st
}

func (s *InitializeSuite) TearDownTest(c *gc.C) {
	if s.State != nil {
		err := s.State.Close()
		c.Check(err, jc.ErrorIsNil)
	}
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *InitializeSuite) TestInitialize(c *gc.C) {
	cfg := testing.ModelConfig(c)
	uuid := cfg.UUID()
	initial := cfg.AllAttrs()
	owner := names.NewLocalUserTag("initialize-admin")

	userpassCredential := cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{
			"username": "alice",
			"password": "hunter2",
		},
	)
	userpassCredential.Label = "some-credential"
	emptyCredential := cloud.NewEmptyCredential()
	emptyCredential.Label = "empty-credential"
	cloudCredentialsIn := map[string]cloud.Credential{
		userpassCredential.Label: userpassCredential,
		emptyCredential.Label:    emptyCredential,
	}
	controllerCfg := testing.FakeControllerConfig()
	controllerCfg["controller-uuid"] = uuid

	st, err := state.Initialize(state.InitializeParams{
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Owner:           owner,
			Config:          cfg,
			CloudName:       "dummy",
			CloudRegion:     "some-region",
			CloudCredential: "some-credential",
		},
		CloudName: "dummy",
		Cloud: cloud.Cloud{
			Type: "dummy",
			AuthTypes: []cloud.AuthType{
				cloud.EmptyAuthType, cloud.UserPassAuthType,
			},
			Regions: []cloud.Region{{Name: "some-region"}},
		},
		CloudCredentials: cloudCredentialsIn,
		MongoInfo:        statetesting.NewMongoInfo(),
		MongoDialOpts:    mongotest.DialOpts(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.NotNil)
	modelTag := st.ModelTag()
	c.Assert(modelTag.Id(), gc.Equals, uuid)
	err = st.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.openState(c, modelTag)

	cfg, err = s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs(), jc.DeepEquals, initial)
	// Check that the model has been created.
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Tag(), gc.Equals, modelTag)
	c.Assert(model.CloudRegion(), gc.Equals, "some-region")
	// Check that the owner has been created.
	c.Assert(model.Owner(), gc.Equals, owner)
	// Check that the owner can be retrieved by the tag.
	entity, err := s.State.FindEntity(model.Owner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Tag(), gc.Equals, owner)
	// Check that the owner has an ModelUser created for the bootstrapped model.
	modelUser, err := s.State.ModelUser(model.Owner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserTag(), gc.Equals, owner)
	c.Assert(modelUser.ModelTag(), gc.Equals, model.Tag())

	// Check that the model can be found through the tag.
	entity, err = s.State.FindEntity(modelTag)
	c.Assert(err, jc.ErrorIsNil)
	cons, err := s.State.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)

	addrs, err := s.State.APIHostPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	info, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &state.ControllerInfo{ModelTag: modelTag, CloudName: "dummy"})

	// Check that the model's credential name is as
	// expected, and the owner's cloud credentials
	// are initialised.
	c.Assert(model.CloudCredential(), gc.Equals, "some-credential")
	cloudCredentials, err := s.State.CloudCredentials(model.Owner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudCredentials, jc.DeepEquals, cloudCredentialsIn)
}

func (s *InitializeSuite) TestInitializeWithInvalidCredentialType(c *gc.C) {
	owner := names.NewLocalUserTag("initialize-admin")
	modelCfg := testing.ModelConfig(c)
	controllerCfg := testing.FakeControllerConfig()
	controllerCfg["controller-uuid"] = modelCfg.UUID()
	_, err := state.Initialize(state.InitializeParams{
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			CloudName: "dummy",
			Owner:     owner,
			Config:    modelCfg,
		},
		CloudName: "dummy",
		Cloud: cloud.Cloud{
			Type: "dummy",
			AuthTypes: []cloud.AuthType{
				cloud.AccessKeyAuthType, cloud.OAuth1AuthType,
			},
		},
		CloudCredentials: map[string]cloud.Credential{
			"borken": cloud.NewCredential(cloud.UserPassAuthType, nil),
		},
		MongoInfo:     statetesting.NewMongoInfo(),
		MongoDialOpts: mongotest.DialOpts(),
	})
	c.Assert(err, gc.ErrorMatches,
		`validating initialization args: validating cloud credentials: credential "borken" with auth-type "userpass" is not supported \(expected one of \["access-key" "oauth1"\]\)`,
	)
}

func (s *InitializeSuite) TestInitializeWithControllerinheritedconfig(c *gc.C) {
	cfg := testing.ModelConfig(c)
	uuid := cfg.UUID()
	initial := cfg.AllAttrs()
	controllerInheritedConfigIn := map[string]interface{}{
		"default-series": initial["default-series"],
	}
	owner := names.NewLocalUserTag("initialize-admin")
	controllerCfg := testing.FakeControllerConfig()
	controllerCfg["controller-uuid"] = uuid

	st, err := state.Initialize(state.InitializeParams{
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			CloudName: "dummy",
			Owner:     owner,
			Config:    cfg,
		},
		CloudName: "dummy",
		Cloud: cloud.Cloud{
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		ControllerInheritedConfig: controllerInheritedConfigIn,
		MongoInfo:                 statetesting.NewMongoInfo(),
		MongoDialOpts:             mongotest.DialOpts(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.NotNil)
	modelTag := st.ModelTag()
	c.Assert(modelTag.Id(), gc.Equals, uuid)
	err = st.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.openState(c, modelTag)

	ControllerInheritedConfig, err := state.ReadSettings(s.State, state.GlobalSettingsC, state.ControllerInheritedSettingsGlobalKey)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ControllerInheritedConfig.Map(), jc.DeepEquals, controllerInheritedConfigIn)

	cfg, err = s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs(), jc.DeepEquals, initial)
}

func (s *InitializeSuite) TestDoubleInitializeConfig(c *gc.C) {
	cfg := testing.ModelConfig(c)
	owner := names.NewLocalUserTag("initialize-admin")

	mgoInfo := statetesting.NewMongoInfo()
	dialOpts := mongotest.DialOpts()
	controllerCfg := testing.FakeControllerConfig()
	controllerCfg["controller-uuid"] = cfg.UUID()

	args := state.InitializeParams{
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			CloudName: "dummy",
			Owner:     owner,
			Config:    cfg,
		},
		CloudName: "dummy",
		Cloud: cloud.Cloud{
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		MongoInfo:     mgoInfo,
		MongoDialOpts: dialOpts,
	}
	st, err := state.Initialize(args)
	c.Assert(err, jc.ErrorIsNil)
	err = st.Close()
	c.Check(err, jc.ErrorIsNil)

	st, err = state.Initialize(args)
	c.Check(err, gc.ErrorMatches, "already initialized")
	if !c.Check(st, gc.IsNil) {
		err = st.Close()
		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *InitializeSuite) TestModelConfigWithAdminSecret(c *gc.C) {
	// admin-secret blocks Initialize.
	good := testing.ModelConfig(c)
	badUpdateAttrs := map[string]interface{}{"admin-secret": "foo"}
	bad, err := good.Apply(badUpdateAttrs)
	owner := names.NewLocalUserTag("initialize-admin")
	controllerCfg := testing.FakeControllerConfig()
	controllerCfg["controller-uuid"] = good.UUID()

	args := state.InitializeParams{
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			CloudName: "dummy",
			Owner:     owner,
			Config:    bad,
		},
		CloudName: "dummy",
		Cloud: cloud.Cloud{
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		MongoInfo:     statetesting.NewMongoInfo(),
		MongoDialOpts: mongotest.DialOpts(),
	}
	_, err = state.Initialize(args)
	c.Assert(err, gc.ErrorMatches, "admin-secret should never be written to the state")

	// admin-secret blocks UpdateModelConfig.
	args.ControllerModelArgs.Config = good
	st, err := state.Initialize(args)
	c.Assert(err, jc.ErrorIsNil)
	st.Close()

	s.openState(c, st.ModelTag())
	err = s.State.UpdateModelConfig(badUpdateAttrs, nil, nil)
	c.Assert(err, gc.ErrorMatches, "admin-secret should never be written to the state")

	// ModelConfig remains inviolate.
	cfg, err := s.State.ModelConfig()
	goodAttrs := good.AllAttrs()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs(), jc.DeepEquals, goodAttrs)
}

func (s *InitializeSuite) TestModelConfigWithoutAgentVersion(c *gc.C) {
	// admin-secret blocks Initialize.
	good := testing.ModelConfig(c)
	attrs := good.AllAttrs()
	delete(attrs, "agent-version")
	bad, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	owner := names.NewLocalUserTag("initialize-admin")
	controllerCfg := testing.FakeControllerConfig()
	controllerCfg["controller-uuid"] = good.UUID()

	args := state.InitializeParams{
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			CloudName: "dummy",
			Owner:     owner,
			Config:    bad,
		},
		CloudName: "dummy",
		Cloud: cloud.Cloud{
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		MongoInfo:     statetesting.NewMongoInfo(),
		MongoDialOpts: mongotest.DialOpts(),
	}
	_, err = state.Initialize(args)
	c.Assert(err, gc.ErrorMatches, "agent-version must always be set in state")

	args.ControllerModelArgs.Config = good
	st, err := state.Initialize(args)
	c.Assert(err, jc.ErrorIsNil)
	st.Close()

	s.openState(c, st.ModelTag())
	err = s.State.UpdateModelConfig(map[string]interface{}{}, []string{"agent-version"}, nil)
	c.Assert(err, gc.ErrorMatches, "agent-version must always be set in state")

	// ModelConfig remains inviolate.
	cfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	goodAttrs := good.AllAttrs()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs(), jc.DeepEquals, goodAttrs)
}

func (s *InitializeSuite) TestCloudConfigWithForbiddenValues(c *gc.C) {
	badAttrNames := []string{
		config.AdminSecretKey,
		config.AgentVersionKey,
	}
	for _, attr := range controller.ControllerOnlyConfigAttributes {
		badAttrNames = append(badAttrNames, attr)
	}

	modelCfg := testing.ModelConfig(c)
	controllerCfg := testing.FakeControllerConfig()
	controllerCfg["controller-uuid"] = modelCfg.UUID()
	args := state.InitializeParams{
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			CloudName: "dummy",
			Owner:     names.NewLocalUserTag("initialize-admin"),
			Config:    modelCfg,
		},
		CloudName: "dummy",
		Cloud: cloud.Cloud{
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		MongoInfo:     statetesting.NewMongoInfo(),
		MongoDialOpts: mongotest.DialOpts(),
	}

	for _, badAttrName := range badAttrNames {
		badAttrs := map[string]interface{}{badAttrName: "foo"}
		args.ControllerInheritedConfig = badAttrs
		_, err := state.Initialize(args)
		c.Assert(err, gc.ErrorMatches, "local cloud config cannot contain .*")
	}
}
