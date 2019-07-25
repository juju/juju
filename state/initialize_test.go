// Copyright Canonical Ltd. 2013
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/clock"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type InitializeSuite struct {
	gitjujutesting.MgoSuite
	testing.BaseSuite
	Pool  *state.StatePool
	State *state.State
	Model *state.Model
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
	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      testing.ControllerTag,
		ControllerModelTag: modelTag,
		MongoSession:       s.Session,
	})
	c.Assert(err, jc.ErrorIsNil)
	st := pool.SystemState()
	s.Pool = pool
	s.State = st

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.Model = m
}

func (s *InitializeSuite) TearDownTest(c *gc.C) {
	if s.Pool != nil {
		err := s.Pool.Close()
		c.Check(err, jc.ErrorIsNil)
	}
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *InitializeSuite) TestInitialize(c *gc.C) {
	cfg := testing.ModelConfig(c)
	uuid := cfg.UUID()
	owner := names.NewLocalUserTag("initialize-admin")

	userPassCredentialTag := names.NewCloudCredentialTag(
		"dummy/" + owner.Id() + "/some-credential",
	)
	emptyCredentialTag := names.NewCloudCredentialTag(
		"dummy/" + owner.Id() + "/empty-credential",
	)
	userpassCredential := cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{
			"username": "alice",
			"password": "hunter2",
		},
	)
	userpassCredential.Label = userPassCredentialTag.Name()
	expectedUserpassCredential := statetesting.CloudCredential(
		cloud.UserPassAuthType,
		map[string]string{
			"username": "alice",
			"password": "hunter2",
		},
	)
	expectedUserpassCredential.DocID = "dummy#initialize-admin#some-credential"
	expectedUserpassCredential.Owner = "initialize-admin"
	expectedUserpassCredential.Cloud = "dummy"
	expectedUserpassCredential.Name = "some-credential"

	emptyCredential := cloud.NewEmptyCredential()
	emptyCredential.Label = emptyCredentialTag.Name()
	expectedEmptyCredential := statetesting.NewEmptyCredential()
	expectedEmptyCredential.DocID = "dummy#initialize-admin#empty-credential"
	expectedEmptyCredential.Owner = "initialize-admin"
	expectedEmptyCredential.Cloud = "dummy"
	expectedEmptyCredential.Name = "empty-credential"

	cloudCredentialsIn := map[names.CloudCredentialTag]cloud.Credential{
		userPassCredentialTag: userpassCredential,
		emptyCredentialTag:    emptyCredential,
	}
	controllerCfg := testing.FakeControllerConfig()

	ctlr, err := state.Initialize(state.InitializeParams{
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			Owner:                   owner,
			Config:                  cfg,
			CloudName:               "dummy",
			CloudRegion:             "dummy-region",
			CloudCredential:         userPassCredentialTag,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name: "dummy",
			Type: "dummy",
			AuthTypes: []cloud.AuthType{
				cloud.EmptyAuthType, cloud.UserPassAuthType,
			},
			Regions: []cloud.Region{{Name: "dummy-region"}},
		},
		CloudCredentials: cloudCredentialsIn,
		MongoSession:     s.Session,
		AdminPassword:    "dummy-secret",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctlr, gc.NotNil)
	st := ctlr.SystemState()
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	modelTag := m.ModelTag()
	c.Assert(modelTag.Id(), gc.Equals, uuid)

	err = ctlr.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.openState(c, modelTag)

	cfg, err = s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expected := cfg.AllAttrs()
	for k, v := range config.ConfigDefaults() {
		if _, ok := expected[k]; !ok {
			expected[k] = v
		}
	}
	c.Assert(cfg.AllAttrs(), jc.DeepEquals, expected)
	// Check that the model has been created.
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Tag(), gc.Equals, modelTag)
	c.Assert(model.CloudRegion(), gc.Equals, "dummy-region")
	// Check that the owner has been created.
	c.Assert(model.Owner(), gc.Equals, owner)
	// Check that the owner can be retrieved by the tag.
	entity, err := s.State.FindEntity(model.Owner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Tag(), gc.Equals, owner)
	// Check that the owner has an ModelUser created for the bootstrapped model.
	modelUser, err := s.State.UserAccess(model.Owner(), model.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserTag, gc.Equals, owner)
	c.Assert(modelUser.Object, gc.Equals, model.Tag())

	// Check that the model can be found through the tag.
	entity, err = s.State.FindEntity(modelTag)
	c.Assert(err, jc.ErrorIsNil)
	cons, err := s.State.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)

	addrs, err := s.State.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	info, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &state.ControllerInfo{ModelTag: modelTag, CloudName: "dummy"})

	// Check that the model's cloud and credential names are as
	// expected, and the owner's cloud credentials are initialised.
	c.Assert(model.Cloud(), gc.Equals, "dummy")
	credentialTag, ok := model.CloudCredential()
	c.Assert(ok, jc.IsTrue)
	c.Assert(credentialTag, gc.Equals, userPassCredentialTag)
	cloudCredentials, err := s.State.CloudCredentials(model.Owner(), "dummy")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudCredentials, jc.DeepEquals, map[string]state.Credential{
		"dummy/initialize-admin/some-credential":  expectedUserpassCredential,
		"dummy/initialize-admin/empty-credential": expectedEmptyCredential,
	})

	// Check that the cloud owner has admin access.
	access, err := s.State.GetCloudAccess("dummy", owner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.AdminAccess)

	// Check that the cloud's model count is initially 1.
	cl, err := s.State.Cloud("dummy")
	c.Assert(err, jc.ErrorIsNil)

	refCount, err := state.CloudModelRefCount(s.State, cl.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(refCount, gc.Equals, 1)

	// Check that the default space is created.
	_, err = s.State.Space("")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InitializeSuite) TestInitializeWithInvalidCredentialType(c *gc.C) {
	owner := names.NewLocalUserTag("initialize-admin")
	modelCfg := testing.ModelConfig(c)
	controllerCfg := testing.FakeControllerConfig()
	credentialTag := names.NewCloudCredentialTag("dummy/" + owner.Id() + "/borken")
	_, err := state.Initialize(state.InitializeParams{
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  modelCfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name: "dummy",
			Type: "dummy",
			AuthTypes: []cloud.AuthType{
				cloud.AccessKeyAuthType, cloud.OAuth1AuthType,
			},
		},
		CloudCredentials: map[names.CloudCredentialTag]cloud.Credential{
			credentialTag: cloud.NewCredential(cloud.UserPassAuthType, nil),
		},
		MongoSession:  s.Session,
		AdminPassword: "dummy-secret",
	})
	c.Assert(err, gc.ErrorMatches,
		`validating initialization args: validating credential "dummy/initialize-admin/borken" for cloud "dummy": supported auth-types \["access-key" "oauth1"\], "userpass" not supported`,
	)
}

func (s *InitializeSuite) TestInitializeWithControllerInheritedConfig(c *gc.C) {
	cfg := testing.ModelConfig(c)
	uuid := cfg.UUID()
	initial := cfg.AllAttrs()
	controllerInheritedConfigIn := map[string]interface{}{
		"default-series": initial["default-series"],
	}
	owner := names.NewLocalUserTag("initialize-admin")
	controllerCfg := testing.FakeControllerConfig()

	ctlr, err := state.Initialize(state.InitializeParams{
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  cfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		ControllerInheritedConfig: controllerInheritedConfigIn,
		MongoSession:              s.Session,
		AdminPassword:             "dummy-secret",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctlr, gc.NotNil)
	st := ctlr.SystemState()
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	modelTag := m.ModelTag()
	c.Assert(modelTag.Id(), gc.Equals, uuid)

	err = ctlr.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.openState(c, modelTag)

	controllerInheritedConfig, err := s.State.ReadSettings(state.GlobalSettingsC, state.CloudGlobalKey("dummy"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerInheritedConfig.Map(), jc.DeepEquals, controllerInheritedConfigIn)

	expected := cfg.AllAttrs()
	for k, v := range config.ConfigDefaults() {
		if _, ok := expected[k]; !ok {
			expected[k] = v
		}
	}
	// Config as read from state has resources tags coerced to a map.
	expected["resource-tags"] = map[string]string{}
	cfg, err = s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs(), jc.DeepEquals, expected)
}

func (s *InitializeSuite) TestDoubleInitializeConfig(c *gc.C) {
	cfg := testing.ModelConfig(c)
	owner := names.NewLocalUserTag("initialize-admin")

	controllerCfg := testing.FakeControllerConfig()

	args := state.InitializeParams{
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  cfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		MongoSession:  s.Session,
		AdminPassword: "dummy-secret",
	}
	ctlr, err := state.Initialize(args)
	c.Assert(err, jc.ErrorIsNil)
	err = ctlr.Close()
	c.Check(err, jc.ErrorIsNil)

	ctlr, err = state.Initialize(args)
	c.Check(err, gc.ErrorMatches, "already initialized")
	c.Check(ctlr, gc.IsNil)
}

func (s *InitializeSuite) TestModelConfigWithAdminSecret(c *gc.C) {
	update := map[string]interface{}{"admin-secret": "foo"}
	remove := []string{}
	s.testBadModelConfig(c, update, remove, "admin-secret should never be written to the state")
}

func (s *InitializeSuite) TestModelConfigWithCAPrivateKey(c *gc.C) {
	update := map[string]interface{}{"ca-private-key": "foo"}
	remove := []string{}
	s.testBadModelConfig(c, update, remove, "ca-private-key should never be written to the state")
}

func (s *InitializeSuite) TestModelConfigWithoutAgentVersion(c *gc.C) {
	update := map[string]interface{}{}
	remove := []string{"agent-version"}
	s.testBadModelConfig(c, update, remove, "agent-version must always be set in state")
}

func (s *InitializeSuite) testBadModelConfig(c *gc.C, update map[string]interface{}, remove []string, expect string) {
	good := testing.CustomModelConfig(c, testing.Attrs{"uuid": testing.ModelTag.Id()})
	bad, err := good.Apply(update)
	c.Assert(err, jc.ErrorIsNil)
	bad, err = bad.Remove(remove)
	c.Assert(err, jc.ErrorIsNil)

	owner := names.NewLocalUserTag("initialize-admin")
	controllerCfg := testing.FakeControllerConfig()

	args := state.InitializeParams{
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			CloudRegion:             "dummy-region",
			Owner:                   owner,
			Config:                  bad,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
			Regions:   []cloud.Region{{Name: "dummy-region"}},
		},
		MongoSession:  s.Session,
		AdminPassword: "dummy-secret",
	}
	_, err = state.Initialize(args)
	c.Assert(err, gc.ErrorMatches, expect)

	args.ControllerModelArgs.Config = good
	ctlr, err := state.Initialize(args)
	c.Assert(err, jc.ErrorIsNil)
	modelUUID := ctlr.SystemState().ModelUUID()
	ctlr.Close()

	s.openState(c, names.NewModelTag(modelUUID))
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = m.UpdateModelConfig(update, remove)
	c.Assert(err, gc.ErrorMatches, expect)

	// ModelConfig remains inviolate.
	cfg, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	goodWithDefaults, err := config.New(config.UseDefaults, good.AllAttrs())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs(), jc.DeepEquals, goodWithDefaults.AllAttrs())
}

func (s *InitializeSuite) TestCloudConfigWithForbiddenValues(c *gc.C) {
	badAttrNames := []string{
		"admin-secret",
		"ca-private-key",
		config.AgentVersionKey,
	}
	for _, attr := range controller.ControllerOnlyConfigAttributes {
		badAttrNames = append(badAttrNames, attr)
	}

	modelCfg := testing.ModelConfig(c)
	controllerCfg := testing.FakeControllerConfig()
	args := state.InitializeParams{
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   names.NewLocalUserTag("initialize-admin"),
			Config:                  modelCfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		MongoSession:  s.Session,
		AdminPassword: "dummy-secret",
	}

	for _, badAttrName := range badAttrNames {
		badAttrs := map[string]interface{}{badAttrName: "foo"}
		args.ControllerInheritedConfig = badAttrs
		_, err := state.Initialize(args)
		c.Assert(err, gc.ErrorMatches, "local cloud config cannot contain .*")
	}
}

func (s *InitializeSuite) TestInitializeWithCloudRegionConfig(c *gc.C) {
	cfg := testing.ModelConfig(c)
	uuid := cfg.UUID()

	// Phony region-config
	regionInheritedConfigIn := cloud.RegionConfig{
		"a-region": cloud.Attrs{
			"a-key": "a-value",
		},
		"b-region": cloud.Attrs{
			"b-key": "b-value",
		},
	}
	owner := names.NewLocalUserTag("initialize-admin")
	controllerCfg := testing.FakeControllerConfig()

	ctlr, err := state.Initialize(state.InitializeParams{
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  cfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:         "dummy",
			Type:         "dummy",
			AuthTypes:    []cloud.AuthType{cloud.EmptyAuthType},
			RegionConfig: regionInheritedConfigIn, // Init with phony region-config
		},
		MongoSession:  s.Session,
		AdminPassword: "dummy-secret",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctlr, gc.NotNil)
	st := ctlr.SystemState()
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	modelTag := m.ModelTag()
	c.Assert(modelTag.Id(), gc.Equals, uuid)

	err = ctlr.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.openState(c, modelTag)

	for k := range regionInheritedConfigIn {
		// Query for config for each region
		regionInheritedConfig, err := s.State.ReadSettings(
			state.GlobalSettingsC,
			"dummy#"+k)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(
			cloud.Attrs(regionInheritedConfig.Map()),
			jc.DeepEquals,
			regionInheritedConfigIn[k])
	}
}

func (s *InitializeSuite) TestInitializeWithCloudRegionMisses(c *gc.C) {
	cfg := testing.ModelConfig(c)
	uuid := cfg.UUID()
	controllerInheritedConfigIn := map[string]interface{}{
		"no-proxy": "local",
	}
	// Phony region-config
	regionInheritedConfigIn := cloud.RegionConfig{
		"a-region": cloud.Attrs{
			"no-proxy": "a-value",
		},
		"b-region": cloud.Attrs{
			"no-proxy": "b-value",
		},
	}
	owner := names.NewLocalUserTag("initialize-admin")
	controllerCfg := testing.FakeControllerConfig()

	ctlr, err := state.Initialize(state.InitializeParams{
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  cfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:         "dummy",
			Type:         "dummy",
			AuthTypes:    []cloud.AuthType{cloud.EmptyAuthType},
			RegionConfig: regionInheritedConfigIn, // Init with phony region-config
		},
		ControllerInheritedConfig: controllerInheritedConfigIn,
		MongoSession:              s.Session,
		AdminPassword:             "dummy-secret",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctlr, gc.NotNil)
	m, err := ctlr.SystemState().Model()
	c.Assert(err, jc.ErrorIsNil)
	modelTag := m.ModelTag()
	c.Assert(modelTag.Id(), gc.Equals, uuid)

	err = ctlr.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.openState(c, modelTag)

	var attrs map[string]interface{}
	rspec := &environs.CloudRegionSpec{Cloud: "dummy", Region: "c-region"}
	got, err := s.State.ComposeNewModelConfig(attrs, rspec)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(got["no-proxy"], gc.Equals, "local")
}

func (s *InitializeSuite) TestInitializeWithCloudRegionHits(c *gc.C) {
	cfg := testing.ModelConfig(c)
	uuid := cfg.UUID()

	controllerInheritedConfigIn := map[string]interface{}{
		"no-proxy": "local",
	}
	// Phony region-config
	regionInheritedConfigIn := cloud.RegionConfig{
		"a-region": cloud.Attrs{
			"no-proxy": "a-value",
		},
		"b-region": cloud.Attrs{
			"no-proxy": "b-value",
		},
	}
	owner := names.NewLocalUserTag("initialize-admin")
	controllerCfg := testing.FakeControllerConfig()

	ctlr, err := state.Initialize(state.InitializeParams{
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:                    state.ModelTypeIAAS,
			CloudName:               "dummy",
			Owner:                   owner,
			Config:                  cfg,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		Cloud: cloud.Cloud{
			Name:         "dummy",
			Type:         "dummy",
			AuthTypes:    []cloud.AuthType{cloud.EmptyAuthType},
			RegionConfig: regionInheritedConfigIn, // Init with phony region-config
		},
		ControllerInheritedConfig: controllerInheritedConfigIn,
		MongoSession:              s.Session,
		AdminPassword:             "dummy-secret",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctlr, gc.NotNil)
	m, err := ctlr.SystemState().Model()
	c.Assert(err, jc.ErrorIsNil)
	modelTag := m.ModelTag()
	c.Assert(modelTag.Id(), gc.Equals, uuid)

	err = ctlr.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.openState(c, modelTag)

	var attrs map[string]interface{}
	for r := range regionInheritedConfigIn {
		rspec := &environs.CloudRegionSpec{Cloud: "dummy", Region: r}
		got, err := s.State.ComposeNewModelConfig(attrs, rspec)
		c.Check(err, jc.ErrorIsNil)
		c.Assert(got["no-proxy"], gc.Equals, regionInheritedConfigIn[r]["no-proxy"])
	}
}
