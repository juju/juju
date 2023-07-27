// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/core/model"
)

type PoolCreateSuite struct {
	SubStorageSuite
	mockAPI *mockPoolCreateAPI
}

var _ = gc.Suite(&PoolCreateSuite{})

func (s *PoolCreateSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockPoolCreateAPI{}
}

func (s *PoolCreateSuite) runPoolCreate(c *gc.C, args []string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, storage.NewPoolCreateCommandForTest(s.mockAPI, s.store), args...)
}

func (s *PoolCreateSuite) TestPoolCreateOneArg(c *gc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine"})
	c.Check(err, gc.ErrorMatches, "pool creation requires names, provider type and optional attributes for configuration")
}

func (s *PoolCreateSuite) TestPoolCreateNoArgs(c *gc.C) {
	_, err := s.runPoolCreate(c, []string{""})
	c.Check(err, gc.ErrorMatches, "pool creation requires names, provider type and optional attributes for configuration")
}

func (s *PoolCreateSuite) TestPoolCreateTwoArgs(c *gc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine", "lollypop"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Creates), gc.Equals, 1)
	createdConfigs := s.mockAPI.Creates[0]
	c.Assert(createdConfigs.Name, gc.Equals, "sunshine")
	c.Assert(createdConfigs.Provider, gc.Equals, "lollypop")
	c.Assert(createdConfigs.Config, gc.DeepEquals, map[string]interface{}{})
}

func (s *PoolCreateSuite) TestPoolCreateAttrMissingKey(c *gc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine", "lollypop", "=too"})
	c.Check(err, gc.ErrorMatches, `expected "key=value", got "=too"`)
}

func (s *PoolCreateSuite) TestPoolCreateAttrMissingPoolName(c *gc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine=again", "lollypop"})
	c.Check(err, gc.ErrorMatches, `pool creation requires names and provider type before optional attributes for configuration`)
}

func (s *PoolCreateSuite) TestPoolCreateAttrMissingProvider(c *gc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine", "lollypop=again"})
	c.Check(err, gc.ErrorMatches, `pool creation requires names and provider type before optional attributes for configuration`)
}

func (s *PoolCreateSuite) TestPoolCreateAttrMissingValue(c *gc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine", "lollypop", "something="})
	c.Check(err, gc.ErrorMatches, `expected "key=value", got "something="`)
}

func (s *PoolCreateSuite) TestPoolCreateAttrEmptyValue(c *gc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine", "lollypop", `something=""`})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Creates), gc.Equals, 1)
	createdConfigs := s.mockAPI.Creates[0]
	c.Assert(createdConfigs.Name, gc.Equals, "sunshine")
	c.Assert(createdConfigs.Provider, gc.Equals, "lollypop")
	c.Assert(createdConfigs.Config, gc.DeepEquals, map[string]interface{}{"something": "\"\""})
}

func (s *PoolCreateSuite) TestPoolCreateOneAttr(c *gc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine", "lollypop", "something=too"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Creates), gc.Equals, 1)
	createdConfigs := s.mockAPI.Creates[0]
	c.Assert(createdConfigs.Name, gc.Equals, "sunshine")
	c.Assert(createdConfigs.Provider, gc.Equals, "lollypop")
	c.Assert(createdConfigs.Config, gc.DeepEquals, map[string]interface{}{"something": "too"})
}

func (s *PoolCreateSuite) TestPoolCreateManyAttrs(c *gc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine", "lollypop", "something=too", "another=one"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Creates), gc.Equals, 1)
	createdConfigs := s.mockAPI.Creates[0]
	c.Assert(createdConfigs.Name, gc.Equals, "sunshine")
	c.Assert(createdConfigs.Provider, gc.Equals, "lollypop")
	c.Assert(createdConfigs.Config, gc.DeepEquals, map[string]interface{}{"something": "too", "another": "one"})
}

func (s *PoolCreateSuite) TestCAASPoolCreateDefaultProvider(c *gc.C) {
	m := s.store.Models["testing"].Models["admin/controller"]
	m.ModelType = model.CAAS
	s.store.Models["testing"].Models["admin/controller"] = m
	_, err := s.runPoolCreate(c, []string{"sunshine"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Creates), gc.Equals, 1)
	createdConfigs := s.mockAPI.Creates[0]
	c.Assert(createdConfigs.Name, gc.Equals, "sunshine")
	c.Assert(createdConfigs.Provider, gc.Equals, "kubernetes")
	c.Assert(createdConfigs.Config, gc.DeepEquals, map[string]interface{}{})
}

func (s *PoolCreateSuite) TestCAASPoolCreateDefaultProviderWithArgs(c *gc.C) {
	m := s.store.Models["testing"].Models["admin/controller"]
	m.ModelType = model.CAAS
	s.store.Models["testing"].Models["admin/controller"] = m
	_, err := s.runPoolCreate(c, []string{"sunshine", "something=too"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Creates), gc.Equals, 1)
	createdConfigs := s.mockAPI.Creates[0]
	c.Assert(createdConfigs.Name, gc.Equals, "sunshine")
	c.Assert(createdConfigs.Provider, gc.Equals, "kubernetes")
	c.Assert(createdConfigs.Config, gc.DeepEquals, map[string]interface{}{"something": "too"})
}

func (s *PoolCreateSuite) TestCAASPoolCreateNonDefaultProvider(c *gc.C) {
	m := s.store.Models["testing"].Models["admin/controller"]
	m.ModelType = model.CAAS
	s.store.Models["testing"].Models["admin/controller"] = m
	_, err := s.runPoolCreate(c, []string{"sunshine", "tmpfs", "something=too"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Creates), gc.Equals, 1)
	createdConfigs := s.mockAPI.Creates[0]
	c.Assert(createdConfigs.Name, gc.Equals, "sunshine")
	c.Assert(createdConfigs.Provider, gc.Equals, "tmpfs")
	c.Assert(createdConfigs.Config, gc.DeepEquals, map[string]interface{}{"something": "too"})
}

type mockCreateData struct {
	Name     string
	Provider string
	Config   map[string]interface{}
}

type mockPoolCreateAPI struct {
	Creates []mockCreateData
}

func (s *mockPoolCreateAPI) CreatePool(pname, ptype string, pconfig map[string]interface{}) error {
	s.Creates = append(s.Creates, mockCreateData{Name: pname, Provider: ptype, Config: pconfig})
	return nil
}

func (s mockPoolCreateAPI) Close() error {
	return nil
}
