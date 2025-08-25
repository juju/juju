// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/apiserver/facades/client/modelconfig"
	"github.com/juju/juju/apiserver/facades/client/modelconfig/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/params"
	secretsprovider "github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type modelconfigSuite struct {
	testing.IsolationSuite
	coretesting.JujuOSEnvSuite
	backend    *mockBackend
	authorizer apiservertesting.FakeAuthorizer
	api        *modelconfig.ModelConfigAPIV3
}

var _ = gc.Suite(&modelconfigSuite{})

func (s *modelconfigSuite) SetUpTest(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	mockModel := mocks.NewMockModel(ctrl)
	mockModel.EXPECT().Cloud().Return(cloud.Cloud{Type: "lxd"}, nil)
	s.SetInitialFeatureFlags(feature.DeveloperMode)
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      names.NewUserTag("bruce@local"),
		AdminTag: names.NewUserTag("bruce@local"),
	}
	s.backend = &mockBackend{
		cfg: config.ConfigValues{
			"type":            {Value: "dummy", Source: "model"},
			"agent-version":   {Value: "1.2.3.4", Source: "model"},
			"ftp-proxy":       {Value: "http://proxy", Source: "model"},
			"authorized-keys": {Value: coretesting.FakeAuthKeys, Source: "model"},
			"charmhub-url":    {Value: "http://meshuggah.rocks", Source: "model"},
		},
		secretBackend: &coresecrets.SecretBackend{
			ID:          "backend-1",
			Name:        "backend-1",
			BackendType: "vault",
			Config: map[string]interface{}{
				"endpoint": "http://0.0.0.0:8200",
			},
		},
		model: mockModel,
	}
	var err error
	s.api, err = modelconfig.NewModelConfigAPI(s.backend, &s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestAdminModelGet(c *gc.C) {
	result, err := s.api.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config, jc.DeepEquals, map[string]params.ConfigValue{
		"type":           {Value: "dummy", Source: "model"},
		"ftp-proxy":      {Value: "http://proxy", Source: "model"},
		"agent-version":  {Value: "1.2.3.4", Source: "model"},
		"charmhub-url":   {Value: "http://meshuggah.rocks", Source: "model"},
		"default-series": {Value: "", Source: "default"},
	})
}

func (s *modelconfigSuite) TestUserModelGet(c *gc.C) {
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:         names.NewUserTag("bruce@local"),
		HasWriteTag: names.NewUserTag("bruce@local"),
		AdminTag:    names.NewUserTag("mary@local"),
	}
	result, err := s.api.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config, jc.DeepEquals, map[string]params.ConfigValue{
		"type":           {Value: "dummy", Source: "model"},
		"ftp-proxy":      {Value: "http://proxy", Source: "model"},
		"agent-version":  {Value: "1.2.3.4", Source: "model"},
		"charmhub-url":   {Value: "http://meshuggah.rocks", Source: "model"},
		"default-series": {Value: "", Source: "default"},
	})
}

func (s *modelconfigSuite) assertConfigValue(c *gc.C, key string, expected interface{}) {
	value, found := s.backend.cfg[key]
	c.Assert(found, jc.IsTrue)
	c.Assert(value.Value, gc.Equals, expected)
}

func (s *modelconfigSuite) assertConfigValueMissing(c *gc.C, key string) {
	_, found := s.backend.cfg[key]
	c.Assert(found, jc.IsFalse)
}

func (s *modelconfigSuite) TestAdminModelSet(c *gc.C) {
	params := params.ModelSet{
		Config: map[string]interface{}{
			"some-key":  "value",
			"other-key": "other value",
		},
	}
	err := s.api.ModelSet(params)
	c.Assert(err, jc.ErrorIsNil)
	s.assertConfigValue(c, "some-key", "value")
	s.assertConfigValue(c, "other-key", "other value")
}

func (s *modelconfigSuite) blockAllChanges(c *gc.C, msg string) {
	s.backend.msg = msg
	s.backend.b = state.ChangeBlock
}

func (s *modelconfigSuite) assertBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue, gc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), jc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *modelconfigSuite) assertModelSetBlocked(c *gc.C, args map[string]interface{}, msg string) {
	err := s.api.ModelSet(params.ModelSet{Config: args})
	s.assertBlocked(c, err, msg)
}

func (s *modelconfigSuite) TestBlockChangesModelSet(c *gc.C) {
	s.blockAllChanges(c, "TestBlockChangesModelSet")
	args := map[string]interface{}{"some-key": "value"}
	s.assertModelSetBlocked(c, args, "TestBlockChangesModelSet")
}

func (s *modelconfigSuite) TestModelSetCannotChangeAgentVersion(c *gc.C) {
	old, err := config.New(config.UseDefaults, dummy.SampleConfig().Merge(coretesting.Attrs{
		"agent-version": "1.2.3.4",
	}))
	c.Assert(err, jc.ErrorIsNil)
	s.backend.old = old
	args := params.ModelSet{
		Config: map[string]interface{}{"agent-version": "9.9.9"},
	}
	err = s.api.ModelSet(args)
	c.Assert(err, gc.ErrorMatches, "agent-version cannot be changed")

	// It's okay to pass config back with the same agent-version.
	result, err := s.api.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config["agent-version"], gc.NotNil)
	args.Config["agent-version"] = result.Config["agent-version"].Value
	err = s.api.ModelSet(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestModelSetCannotChangeCharmHubURL(c *gc.C) {
	old, err := config.New(config.UseDefaults, dummy.SampleConfig().Merge(coretesting.Attrs{
		"charmhub-url": "http://meshuggah.rocks",
	}))
	c.Assert(err, jc.ErrorIsNil)
	s.backend.old = old
	args := params.ModelSet{
		Config: map[string]interface{}{"charmhub-url": "http://another-url.com"},
	}
	err = s.api.ModelSet(args)
	c.Assert(err, gc.ErrorMatches, "charmhub-url cannot be changed")

	// It's okay to pass config back with the same charmhub-url.
	result, err := s.api.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config["charmhub-url"], gc.NotNil)
	args.Config["charmhub-url"] = result.Config["charmhub-url"].Value
	err = s.api.ModelSet(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestModelSetCannotChangeBothDefaultSeriesAndDefaultBaseWithSeries(c *gc.C) {
	old, err := config.New(config.UseDefaults, dummy.SampleConfig().Merge(coretesting.Attrs{
		"default-series": "jammy",
	}))
	c.Assert(err, jc.ErrorIsNil)

	s.backend.old = old
	args := params.ModelSet{
		Config: map[string]interface{}{
			"default-series": "jammy",
			"default-base":   "ubuntu@22.04",
		},
	}
	err = s.api.ModelSet(args)
	c.Assert(err, gc.ErrorMatches, "cannot set both default-series and default-base")

	err = s.api.ModelSet(params.ModelSet{
		Config: map[string]interface{}{
			"default-series": "jammy",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config["default-series"], gc.NotNil)
	c.Assert(result.Config["default-series"].Value, gc.Equals, "jammy")
	c.Assert(result.Config["default-base"].Value, gc.Equals, "ubuntu@22.04/stable")
}

func (s *modelconfigSuite) TestModelSetCannotChangeBothDefaultSeriesAndDefaultBaseWithBase(c *gc.C) {
	old, err := config.New(config.UseDefaults, dummy.SampleConfig().Merge(coretesting.Attrs{
		"default-base": "ubuntu@22.04",
	}))
	c.Assert(err, jc.ErrorIsNil)

	s.backend.old = old
	args := params.ModelSet{
		Config: map[string]interface{}{
			"default-series": "jammy",
			"default-base":   "ubuntu@22.04",
		},
	}
	err = s.api.ModelSet(args)
	c.Assert(err, gc.ErrorMatches, "cannot set both default-series and default-base")

	err = s.api.ModelSet(params.ModelSet{
		Config: map[string]interface{}{
			"default-series": "jammy",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config["default-series"], gc.NotNil)
	c.Assert(result.Config["default-series"].Value, gc.Equals, "jammy")
	c.Assert(result.Config["default-base"].Value, gc.Equals, "ubuntu@22.04/stable")
}

func (s *modelconfigSuite) TestModelSetCannotSetAuthorizedKeys(c *gc.C) {
	// Try to set the authorized-keys model config.
	args := params.ModelSet{
		Config: map[string]interface{}{"authorized-keys": "ssh-rsa new Juju:juju-client-key"},
	}
	err := s.api.ModelSet(args)
	c.Assert(err, gc.ErrorMatches, "authorized-keys cannot be set")
	// Make sure the authorized-keys still contains its original value.
	s.assertConfigValue(c, "authorized-keys", coretesting.FakeAuthKeys)
}

func (s *modelconfigSuite) TestAdminCanSetLogTrace(c *gc.C) {
	args := params.ModelSet{
		Config: map[string]interface{}{"logging-config": "<root>=DEBUG;somepackage=TRACE"},
	}
	err := s.api.ModelSet(args)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config["logging-config"].Value, gc.Equals, "<root>=DEBUG;somepackage=TRACE")
}

func (s *modelconfigSuite) TestUserCanSetLogNoTrace(c *gc.C) {
	args := params.ModelSet{
		Config: map[string]interface{}{"logging-config": "<root>=DEBUG;somepackage=ERROR"},
	}
	apiUser := names.NewUserTag("fred")
	s.authorizer.Tag = apiUser
	s.authorizer.HasWriteTag = apiUser
	err := s.api.ModelSet(args)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config["logging-config"].Value, gc.Equals, "<root>=DEBUG;somepackage=ERROR")
}

func (s *modelconfigSuite) TestUserReadAccess(c *gc.C) {
	apiUser := names.NewUserTag("read")
	s.authorizer.Tag = apiUser

	_, err := s.api.ModelGet()
	c.Assert(err, jc.ErrorIsNil)

	err = s.api.ModelSet(params.ModelSet{})
	c.Assert(errors.Cause(err), gc.ErrorMatches, "permission denied")
}

func (s *modelconfigSuite) TestUserCannotSetLogTrace(c *gc.C) {
	args := params.ModelSet{
		Config: map[string]interface{}{"logging-config": "<root>=DEBUG;somepackage=TRACE"},
	}
	apiUser := names.NewUserTag("fred")
	s.authorizer.Tag = apiUser
	s.authorizer.HasWriteTag = apiUser
	err := s.api.ModelSet(args)
	c.Assert(err, gc.ErrorMatches, `only controller admins can set a model's logging level to TRACE`)
}

func (s *modelconfigSuite) TestSetSecretBackend(c *gc.C) {
	args := params.ModelSet{
		Config: map[string]interface{}{"secret-backend": 1},
	}
	err := s.api.ModelSet(args)
	c.Assert(err, gc.ErrorMatches, `"secret-backend" config value is not a string`)

	args.Config = map[string]interface{}{"secret-backend": ""}
	err = s.api.ModelSet(args)
	c.Assert(err, gc.ErrorMatches, `empty "secret-backend" config value not valid`)

	args.Config = map[string]interface{}{"secret-backend": "auto"}
	err = s.api.ModelSet(args)
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.api.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config["secret-backend"].Value, gc.Equals, "auto")
}

func (s *modelconfigSuite) TestSetSecretBackendExternal(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	vaultProvider := mocks.NewMockSecretBackendProvider(ctrl)
	s.PatchValue(&commonsecrets.GetProvider, func(string) (secretsprovider.SecretBackendProvider, error) { return vaultProvider, nil })
	vaultBackend := mocks.NewMockSecretsBackend(ctrl)

	gomock.InOrder(
		vaultProvider.EXPECT().Type().Return("vault"),
		vaultProvider.EXPECT().NewBackend(&secretsprovider.ModelBackendConfig{
			BackendConfig: secretsprovider.BackendConfig{
				BackendType: "vault",
				Config:      s.backend.secretBackend.Config,
			},
		}).Return(vaultBackend, nil),
		vaultBackend.EXPECT().Ping().Return(nil),
	)

	args := params.ModelSet{
		Config: map[string]interface{}{"secret-backend": "backend-1"},
	}
	err := s.api.ModelSet(args)
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.api.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config["secret-backend"].Value, gc.Equals, "backend-1")
}

func (s *modelconfigSuite) TestSetSecretBackendExternalValidationFailed(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	vaultProvider := mocks.NewMockSecretBackendProvider(ctrl)
	s.PatchValue(&commonsecrets.GetProvider, func(string) (secretsprovider.SecretBackendProvider, error) { return vaultProvider, nil })
	vaultBackend := mocks.NewMockSecretsBackend(ctrl)

	gomock.InOrder(
		vaultProvider.EXPECT().Type().Return("vault"),
		vaultProvider.EXPECT().NewBackend(&secretsprovider.ModelBackendConfig{
			BackendConfig: secretsprovider.BackendConfig{
				BackendType: "vault",
				Config:      s.backend.secretBackend.Config,
			},
		}).Return(vaultBackend, nil),
		vaultBackend.EXPECT().Ping().Return(errors.New("not reachable")),
	)

	args := params.ModelSet{
		Config: map[string]interface{}{"secret-backend": "backend-1"},
	}
	err := s.api.ModelSet(args)
	c.Assert(err, gc.ErrorMatches, `cannot ping backend "backend-1": not reachable`)
}

func (s *modelconfigSuite) TestSetModelSetLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	model := mocks.NewMockModel(ctrl)
	s.backend.model = model
	model.EXPECT().MachinesLen().Return(0, nil)

	args := params.ModelSet{
		Config: map[string]interface{}{"project": "cool-project"},
	}
	err := s.api.ModelSet(args)
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.api.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config["project"].Value, gc.Equals, "cool-project")
}

func (s *modelconfigSuite) TestSetModelSetLXDProfileFailsBecauseModelIsNotEmpty(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	model := mocks.NewMockModel(ctrl)
	s.backend.model = model
	model.EXPECT().MachinesLen().Return(3, nil)
	model.EXPECT().Name().Return("my-model")

	args := params.ModelSet{
		Config: map[string]interface{}{"project": "cool-project"},
	}
	err := s.api.ModelSet(args)
	c.Assert(err, gc.ErrorMatches, "cannot change project because model \"my-model\" is non-empty")
}

func (s *modelconfigSuite) TestModelUnset(c *gc.C) {
	err := s.backend.UpdateModelConfig(map[string]interface{}{"abc": 123}, nil)
	c.Assert(err, jc.ErrorIsNil)

	args := params.ModelUnset{Keys: []string{"abc"}}
	err = s.api.ModelUnset(args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertConfigValueMissing(c, "abc")
}

func (s *modelconfigSuite) TestBlockModelUnset(c *gc.C) {
	err := s.backend.UpdateModelConfig(map[string]interface{}{"abc": 123}, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.blockAllChanges(c, "TestBlockModelUnset")

	args := params.ModelUnset{Keys: []string{"abc"}}
	err = s.api.ModelUnset(args)
	s.assertBlocked(c, err, "TestBlockModelUnset")
}

func (s *modelconfigSuite) TestModelUnsetMissing(c *gc.C) {
	// It's okay to unset a non-existent attribute.
	args := params.ModelUnset{Keys: []string{"not_there"}}
	err := s.api.ModelUnset(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestSetSupportCredentals(c *gc.C) {
	err := s.api.SetSLALevel(params.ModelSLA{
		ModelSLAInfo: params.ModelSLAInfo{Level: "level", Owner: "bob"},
		Credentials:  []byte("foobar"),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestClientSetModelConstraints(c *gc.C) {
	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.api.SetModelConstraints(params.SetConstraints{
		ApplicationName: "app",
		Constraints:     cons,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.backend.cons, gc.DeepEquals, cons)
}

func (s *modelconfigSuite) assertSetModelConstraintsBlocked(c *gc.C, msg string) {
	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.api.SetModelConstraints(params.SetConstraints{
		ApplicationName: "app",
		Constraints:     cons,
	})
	s.assertBlocked(c, err, msg)
}

func (s *modelconfigSuite) TestBlockChangesClientSetModelConstraints(c *gc.C) {
	s.blockAllChanges(c, "TestBlockChangesClientSetModelConstraints")
	s.assertSetModelConstraintsBlocked(c, "TestBlockChangesClientSetModelConstraints")
}

func (s *modelconfigSuite) TestClientGetModelConstraints(c *gc.C) {
	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	s.backend.cons = cons
	obtained, err := s.api.GetModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained.Constraints, gc.DeepEquals, cons)
}

type mockBackend struct {
	cfg           config.ConfigValues
	old           *config.Config
	b             state.BlockType
	msg           string
	cons          constraints.Value
	secretBackend *coresecrets.SecretBackend
	model         modelconfig.Model
}

func (m *mockBackend) Model() modelconfig.Model {
	return m.model
}

func (m *mockBackend) SetModelConstraints(value constraints.Value) error {
	m.cons = value
	return nil
}

func (m *mockBackend) ModelConstraints() (constraints.Value, error) {
	return m.cons, nil
}

func (m *mockBackend) ModelConfigValues() (config.ConfigValues, error) {
	return m.cfg, nil
}

func (m *mockBackend) Sequences() (map[string]int, error) {
	return nil, nil
}

func (m *mockBackend) UpdateModelConfig(update map[string]interface{}, remove []string,
	validate ...state.ValidateConfigFunc) error {
	for _, validateFunc := range validate {
		if err := validateFunc(update, remove, m.old); err != nil {
			return err
		}
	}
	for k, v := range update {
		m.cfg[k] = config.ConfigValue{Value: v, Source: "model"}
	}
	for _, n := range remove {
		delete(m.cfg, n)
	}
	return nil
}

func (m *mockBackend) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	if m.b == t {
		return &mockBlock{t: t, m: m.msg}, true, nil
	} else {
		return nil, false, nil
	}
}

func (m *mockBackend) ModelTag() names.ModelTag {
	return names.NewModelTag("deadbeef-2f18-4fd2-967d-db9663db7bea")
}

func (m *mockBackend) ControllerTag() names.ControllerTag {
	return names.NewControllerTag("deadbeef-babe-4fd2-967d-db9663db7bea")
}

func (m *mockBackend) SetSLA(level, owner string, credentials []byte) error {
	return nil
}

func (m *mockBackend) SLALevel() (string, error) {
	return "mock-level", nil
}

func (m *mockBackend) SpaceByName(string) error {
	return nil
}

func (m *mockBackend) GetSecretBackend(name string) (*coresecrets.SecretBackend, error) {
	if name == "invalid" {
		return nil, errors.NotFoundf("invalid")
	}
	return m.secretBackend, nil
}

type mockBlock struct {
	state.Block
	t state.BlockType
	m string
}

func (m mockBlock) Id() string { return "" }

func (m mockBlock) Tag() (names.Tag, error) { return names.NewModelTag("mocktesting"), nil }

func (m mockBlock) Type() state.BlockType { return m.t }

func (m mockBlock) Message() string { return m.m }

func (m mockBlock) ModelUUID() string { return "" }
