// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

type PoolUpdateSuite struct {
	SubStorageSuite
	mockAPI *mockPoolUpdateAPI
}

func TestPoolUpdateSuite(t *stdtesting.T) {
	tc.Run(t, &PoolUpdateSuite{})
}

func (s *PoolUpdateSuite) SetUpTest(c *tc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockPoolUpdateAPI{}
}

func (s *PoolUpdateSuite) runPoolUpdate(c *tc.C, args []string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, storage.NewPoolUpdateCommandForTest(s.mockAPI, s.store), args...)
}

func (s *PoolUpdateSuite) TestPoolUpdateOneArg(c *tc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine"})
	c.Check(err, tc.ErrorMatches, "pool update requires name and configuration attributes")
	c.Assert(len(s.mockAPI.Updates), tc.Equals, 0)
}

func (s *PoolUpdateSuite) TestPoolUpdateNoArgs(c *tc.C) {
	_, err := s.runPoolUpdate(c, []string{""})
	c.Check(err, tc.ErrorMatches, "pool update requires name and configuration attributes")
	c.Assert(len(s.mockAPI.Updates), tc.Equals, 0)
}

func (s *PoolUpdateSuite) TestPoolUpdateWithAttrArgs(c *tc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", "lollypop=true"})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Updates), tc.Equals, 1)
	updatedConfigs := s.mockAPI.Updates[0]
	c.Assert(updatedConfigs.Name, tc.Equals, "sunshine")
	c.Assert(updatedConfigs.Config, tc.DeepEquals, map[string]interface{}{"lollypop": "true"})
}

func (s *PoolUpdateSuite) TestPoolUpdateAttrMissingKey(c *tc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", "=too"})
	c.Check(err, tc.ErrorMatches, `expected "key=value", got "=too"`)
	c.Assert(len(s.mockAPI.Updates), tc.Equals, 0)
}

func (s *PoolUpdateSuite) TestPoolUpdateAttrMissingValue(c *tc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", "something="})
	c.Check(err, tc.ErrorMatches, `expected "key=value", got "something="`)
	c.Assert(len(s.mockAPI.Updates), tc.Equals, 0)
}

func (s *PoolUpdateSuite) TestPoolUpdateAttrEmptyValue(c *tc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", `something=""`})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Updates), tc.Equals, 1)
	updatedConfigs := s.mockAPI.Updates[0]
	c.Assert(updatedConfigs.Name, tc.Equals, "sunshine")
	c.Assert(updatedConfigs.Config, tc.DeepEquals, map[string]interface{}{"something": "\"\""})
}

func (s *PoolUpdateSuite) TestPoolUpdateOneAttr(c *tc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", "something=too"})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Updates), tc.Equals, 1)
	updatedConfigs := s.mockAPI.Updates[0]
	c.Assert(updatedConfigs.Name, tc.Equals, "sunshine")
	c.Assert(updatedConfigs.Config, tc.DeepEquals, map[string]interface{}{"something": "too"})
}

func (s *PoolUpdateSuite) TestPoolUpdateManyAttrs(c *tc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", "something=too", "another=one"})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(s.mockAPI.Updates), tc.Equals, 1)
	updatedConfigs := s.mockAPI.Updates[0]
	c.Assert(updatedConfigs.Name, tc.Equals, "sunshine")
	c.Assert(updatedConfigs.Config, tc.DeepEquals, map[string]interface{}{"something": "too", "another": "one"})
}

type mockUpdateData struct {
	Name     string
	Provider string
	Config   map[string]interface{}
}

type mockPoolUpdateAPI struct {
	Updates []mockUpdateData
}

func (s *mockPoolUpdateAPI) UpdatePool(ctx context.Context, pname, provider string, pconfig map[string]interface{}) error {
	s.Updates = append(s.Updates, mockUpdateData{Name: pname, Provider: provider, Config: pconfig})
	return nil
}

func (s mockPoolUpdateAPI) Close() error {
	return nil
}
