// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

type PoolCreateSuite struct {
	SubStorageSuite
	mockAPI *mockPoolCreateAPI
}

func TestPoolCreateSuite(t *stdtesting.T) { tc.Run(t, &PoolCreateSuite{}) }
func (s *PoolCreateSuite) SetUpTest(c *tc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockPoolCreateAPI{}
}

func (s *PoolCreateSuite) runPoolCreate(c *tc.C, args []string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, storage.NewPoolCreateCommandForTest(s.mockAPI, s.store), args...)
}

func (s *PoolCreateSuite) TestPoolCreateOneArg(c *tc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine"})
	c.Check(err, tc.ErrorMatches, "pool creation requires names, provider type and optional attributes for configuration")
}

func (s *PoolCreateSuite) TestPoolCreateNoArgs(c *tc.C) {
	_, err := s.runPoolCreate(c, []string{""})
	c.Check(err, tc.ErrorMatches, "pool creation requires names, provider type and optional attributes for configuration")
}

func (s *PoolCreateSuite) TestPoolCreateTwoArgs(c *tc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine", "lollypop"})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Creates), tc.Equals, 1)
	createdConfigs := s.mockAPI.Creates[0]
	c.Assert(createdConfigs.Name, tc.Equals, "sunshine")
	c.Assert(createdConfigs.Provider, tc.Equals, "lollypop")
	c.Assert(createdConfigs.Config, tc.DeepEquals, map[string]interface{}{})
}

func (s *PoolCreateSuite) TestPoolCreateAttrMissingKey(c *tc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine", "lollypop", "=too"})
	c.Check(err, tc.ErrorMatches, `expected "key=value", got "=too"`)
}

func (s *PoolCreateSuite) TestPoolCreateAttrMissingPoolName(c *tc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine=again", "lollypop"})
	c.Check(err, tc.ErrorMatches, `pool creation requires names and provider type before optional attributes for configuration`)
}

func (s *PoolCreateSuite) TestPoolCreateAttrMissingProvider(c *tc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine", "lollypop=again"})
	c.Check(err, tc.ErrorMatches, `pool creation requires names and provider type before optional attributes for configuration`)
}

func (s *PoolCreateSuite) TestPoolCreateAttrMissingValue(c *tc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine", "lollypop", "something="})
	c.Check(err, tc.ErrorMatches, `expected "key=value", got "something="`)
}

func (s *PoolCreateSuite) TestPoolCreateAttrEmptyValue(c *tc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine", "lollypop", `something=""`})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Creates), tc.Equals, 1)
	createdConfigs := s.mockAPI.Creates[0]
	c.Assert(createdConfigs.Name, tc.Equals, "sunshine")
	c.Assert(createdConfigs.Provider, tc.Equals, "lollypop")
	c.Assert(createdConfigs.Config, tc.DeepEquals, map[string]interface{}{"something": "\"\""})
}

func (s *PoolCreateSuite) TestPoolCreateOneAttr(c *tc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine", "lollypop", "something=too"})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Creates), tc.Equals, 1)
	createdConfigs := s.mockAPI.Creates[0]
	c.Assert(createdConfigs.Name, tc.Equals, "sunshine")
	c.Assert(createdConfigs.Provider, tc.Equals, "lollypop")
	c.Assert(createdConfigs.Config, tc.DeepEquals, map[string]interface{}{"something": "too"})
}

func (s *PoolCreateSuite) TestPoolCreateManyAttrs(c *tc.C) {
	_, err := s.runPoolCreate(c, []string{"sunshine", "lollypop", "something=too", "another=one"})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Creates), tc.Equals, 1)
	createdConfigs := s.mockAPI.Creates[0]
	c.Assert(createdConfigs.Name, tc.Equals, "sunshine")
	c.Assert(createdConfigs.Provider, tc.Equals, "lollypop")
	c.Assert(createdConfigs.Config, tc.DeepEquals, map[string]interface{}{"something": "too", "another": "one"})
}

func (s *PoolCreateSuite) TestCAASPoolCreateDefaultProvider(c *tc.C) {
	m := s.store.Models["testing"].Models["admin/controller"]
	m.ModelType = model.CAAS
	s.store.Models["testing"].Models["admin/controller"] = m
	_, err := s.runPoolCreate(c, []string{"sunshine"})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Creates), tc.Equals, 1)
	createdConfigs := s.mockAPI.Creates[0]
	c.Assert(createdConfigs.Name, tc.Equals, "sunshine")
	c.Assert(createdConfigs.Provider, tc.Equals, "kubernetes")
	c.Assert(createdConfigs.Config, tc.DeepEquals, map[string]interface{}{})
}

func (s *PoolCreateSuite) TestCAASPoolCreateDefaultProviderWithArgs(c *tc.C) {
	m := s.store.Models["testing"].Models["admin/controller"]
	m.ModelType = model.CAAS
	s.store.Models["testing"].Models["admin/controller"] = m
	_, err := s.runPoolCreate(c, []string{"sunshine", "something=too"})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Creates), tc.Equals, 1)
	createdConfigs := s.mockAPI.Creates[0]
	c.Assert(createdConfigs.Name, tc.Equals, "sunshine")
	c.Assert(createdConfigs.Provider, tc.Equals, "kubernetes")
	c.Assert(createdConfigs.Config, tc.DeepEquals, map[string]interface{}{"something": "too"})
}

func (s *PoolCreateSuite) TestCAASPoolCreateNonDefaultProvider(c *tc.C) {
	m := s.store.Models["testing"].Models["admin/controller"]
	m.ModelType = model.CAAS
	s.store.Models["testing"].Models["admin/controller"] = m
	_, err := s.runPoolCreate(c, []string{"sunshine", "tmpfs", "something=too"})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Creates), tc.Equals, 1)
	createdConfigs := s.mockAPI.Creates[0]
	c.Assert(createdConfigs.Name, tc.Equals, "sunshine")
	c.Assert(createdConfigs.Provider, tc.Equals, "tmpfs")
	c.Assert(createdConfigs.Config, tc.DeepEquals, map[string]interface{}{"something": "too"})
}

type mockCreateData struct {
	Name     string
	Provider string
	Config   map[string]interface{}
}

type mockPoolCreateAPI struct {
	Creates []mockCreateData
}

func (s *mockPoolCreateAPI) CreatePool(ctx context.Context, pname, ptype string, pconfig map[string]interface{}) error {
	s.Creates = append(s.Creates, mockCreateData{Name: pname, Provider: ptype, Config: pconfig})
	return nil
}

func (s mockPoolCreateAPI) Close() error {
	return nil
}
