// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig_test

import (
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/modelconfig"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/dummy"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type modelconfigSuite struct {
	gitjujutesting.IsolationSuite
	backend    *mockBackend
	authorizer apiservertesting.FakeAuthorizer
	api        *modelconfig.ModelConfigAPI
}

var _ = gc.Suite(&modelconfigSuite{})

func (s *modelconfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      names.NewUserTag("bruce@local"),
		AdminTag: names.NewUserTag("bruce@local"),
	}
	s.backend = &mockBackend{
		cfg: config.ConfigValues{
			"type":            {"dummy", "model"},
			"agent-version":   {"1.2.3.4", "model"},
			"ftp-proxy":       {"http://proxy", "model"},
			"authorized-keys": {testing.FakeAuthKeys, "model"},
		},
	}
	var err error
	s.api, err = modelconfig.NewModelConfigAPI(s.backend, &s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestModelGet(c *gc.C) {
	result, err := s.api.ModelGet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config, jc.DeepEquals, map[string]params.ConfigValue{
		"type":          {"dummy", "model"},
		"ftp-proxy":     {"http://proxy", "model"},
		"agent-version": {Value: "1.2.3.4", Source: "model"},
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

func (s *modelconfigSuite) TestModelSet(c *gc.C) {
	params := params.ModelSet{
		Config: map[string]interface{}{
			"some-key":  "value",
			"other-key": "other value"},
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
	err := s.api.ModelSet(params.ModelSet{args})
	s.assertBlocked(c, err, msg)
}

func (s *modelconfigSuite) TestBlockChangesModelSet(c *gc.C) {
	s.blockAllChanges(c, "TestBlockChangesModelSet")
	args := map[string]interface{}{"some-key": "value"}
	s.assertModelSetBlocked(c, args, "TestBlockChangesModelSet")
}

func (s *modelconfigSuite) TestModelSetCannotChangeAgentVersion(c *gc.C) {
	old, err := config.New(config.UseDefaults, dummy.SampleConfig().Merge(testing.Attrs{
		"agent-version": "1.2.3.4",
	}))
	c.Assert(err, jc.ErrorIsNil)
	s.backend.old = old
	args := params.ModelSet{
		map[string]interface{}{"agent-version": "9.9.9"},
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

func (s *modelconfigSuite) TestModelUnset(c *gc.C) {
	err := s.backend.UpdateModelConfig(map[string]interface{}{"abc": 123}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	args := params.ModelUnset{[]string{"abc"}}
	err = s.api.ModelUnset(args)
	c.Assert(err, jc.ErrorIsNil)
	s.assertConfigValueMissing(c, "abc")
}

func (s *modelconfigSuite) TestBlockModelUnset(c *gc.C) {
	err := s.backend.UpdateModelConfig(map[string]interface{}{"abc": 123}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.blockAllChanges(c, "TestBlockModelUnset")

	args := params.ModelUnset{[]string{"abc"}}
	err = s.api.ModelUnset(args)
	s.assertBlocked(c, err, "TestBlockModelUnset")
}

func (s *modelconfigSuite) TestModelUnsetMissing(c *gc.C) {
	// It's okay to unset a non-existent attribute.
	args := params.ModelUnset{[]string{"not_there"}}
	err := s.api.ModelUnset(args)
	c.Assert(err, jc.ErrorIsNil)
}

type mockBackend struct {
	cfg config.ConfigValues
	old *config.Config
	b   state.BlockType
	msg string
}

func (m *mockBackend) ModelConfigValues() (config.ConfigValues, error) {
	return m.cfg, nil
}

func (m *mockBackend) UpdateModelConfig(update map[string]interface{}, remove []string, validate state.ValidateConfigFunc) error {
	if validate != nil {
		err := validate(update, remove, m.old)
		if err != nil {
			return err
		}
	}
	for k, v := range update {
		m.cfg[k] = config.ConfigValue{v, "model"}
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
