// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/storage"
)

type PoolUpdateSuite struct {
	SubStorageSuite
	mockAPI *mockPoolUpdateAPI
}

var _ = gc.Suite(&PoolUpdateSuite{})

func (s *PoolUpdateSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockPoolUpdateAPI{}
}

func (s *PoolUpdateSuite) runPoolUpdate(c *gc.C, args []string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, storage.NewPoolUpdateCommandForTest(s.mockAPI, s.store), args...)
}

func (s *PoolUpdateSuite) TestPoolUpdateOneArg(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine"})
	c.Check(err, gc.ErrorMatches, "pool update requires name and configuration attributes")
	c.Assert(len(s.mockAPI.Updates), gc.Equals, 0)
}

func (s *PoolUpdateSuite) TestPoolUpdateNoArgs(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{""})
	c.Check(err, gc.ErrorMatches, "pool update requires name and configuration attributes")
	c.Assert(len(s.mockAPI.Updates), gc.Equals, 0)
}

func (s *PoolUpdateSuite) TestPoolUpdateWithAttrArgs(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", "lollypop=true"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Updates), gc.Equals, 1)
	updatedConfigs := s.mockAPI.Updates[0]
	c.Assert(updatedConfigs.Name, gc.Equals, "sunshine")
	c.Assert(updatedConfigs.Config, gc.DeepEquals, map[string]interface{}{"lollypop": "true"})
}

func (s *PoolUpdateSuite) TestPoolUpdateAttrMissingKey(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", "=too"})
	c.Check(err, gc.ErrorMatches, `expected "key=value", got "=too"`)
	c.Assert(len(s.mockAPI.Updates), gc.Equals, 0)
}

func (s *PoolUpdateSuite) TestPoolUpdateAttrMissingValue(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", "something="})
	c.Check(err, gc.ErrorMatches, `expected "key=value", got "something="`)
	c.Assert(len(s.mockAPI.Updates), gc.Equals, 0)
}

func (s *PoolUpdateSuite) TestPoolUpdateAttrEmptyValue(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", `something=""`})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Updates), gc.Equals, 1)
	updatedConfigs := s.mockAPI.Updates[0]
	c.Assert(updatedConfigs.Name, gc.Equals, "sunshine")
	c.Assert(updatedConfigs.Config, gc.DeepEquals, map[string]interface{}{"something": "\"\""})
}

func (s *PoolUpdateSuite) TestPoolUpdateOneAttr(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", "something=too"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Updates), gc.Equals, 1)
	updatedConfigs := s.mockAPI.Updates[0]
	c.Assert(updatedConfigs.Name, gc.Equals, "sunshine")
	c.Assert(updatedConfigs.Config, gc.DeepEquals, map[string]interface{}{"something": "too"})
}

func (s *PoolUpdateSuite) TestPoolUpdateManyAttrs(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", "something=too", "another=one"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Updates), gc.Equals, 1)
	updatedConfigs := s.mockAPI.Updates[0]
	c.Assert(updatedConfigs.Name, gc.Equals, "sunshine")
	c.Assert(updatedConfigs.Config, gc.DeepEquals, map[string]interface{}{"something": "too", "another": "one"})
}

type mockUpdateData struct {
	Name     string
	Provider string
	Config   map[string]interface{}
}

type mockPoolUpdateAPI struct {
	Updates []mockUpdateData
}

func (s *mockPoolUpdateAPI) UpdatePool(pname, provider string, pconfig map[string]interface{}) error {
	s.Updates = append(s.Updates, mockUpdateData{Name: pname, Provider: provider, Config: pconfig})
	return nil
}

func (s mockPoolUpdateAPI) Close() error {
	return nil
}
