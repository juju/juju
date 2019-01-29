// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/storage"
	_ "github.com/juju/juju/provider/dummy"
)

type PoolUpdateSuite struct {
	SubStorageSuite
	mockAPI *mockPoolUpdateAPI
}

var _ = gc.Suite(&PoolUpdateSuite{})

func (s *PoolUpdateSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockPoolUpdateAPI{APIVersion: 5}
}

func (s *PoolUpdateSuite) runPoolUpdate(c *gc.C, args []string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, storage.NewPoolUpdateCommandForTest(s.mockAPI, s.store), args...)
}

func (s *PoolUpdateSuite) TestPoolUpdateOneArg(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine"})
	c.Check(err, gc.ErrorMatches, "pool update requires name and configuration attributes")
}

func (s *PoolUpdateSuite) TestPoolUpdateNoArgs(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{""})
	c.Check(err, gc.ErrorMatches, "pool update requires name and configuration attributes")
}

func (s *PoolUpdateSuite) TestPoolUpdateWithAttrArgs(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", "lollypop=true"})
	c.Check(err, jc.ErrorIsNil)
}

func (s *PoolUpdateSuite) TestPoolUpdateAttrMissingKey(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", "=too"})
	c.Check(err, gc.ErrorMatches, `expected "key=value", got "=too"`)
}

func (s *PoolUpdateSuite) TestPoolUpdateAttrMissingValue(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", "something="})
	c.Check(err, gc.ErrorMatches, `expected "key=value", got "something="`)
}

func (s *PoolUpdateSuite) TestPoolUpdateAttrEmptyValue(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", `something=""`})
	c.Check(err, jc.ErrorIsNil)
}

func (s *PoolUpdateSuite) TestPoolUpdateOneAttr(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", "something=too"})
	c.Check(err, jc.ErrorIsNil)
}

func (s *PoolUpdateSuite) TestPoolUpdateEmptyAttr(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", ""})
	c.Check(err, gc.ErrorMatches, `expected "key=value", got ""`)
}

func (s *PoolUpdateSuite) TestPoolUpdateManyAttrs(c *gc.C) {
	_, err := s.runPoolUpdate(c, []string{"sunshine", "something=too", "another=one"})
	c.Check(err, jc.ErrorIsNil)
}

func (s *PoolUpdateSuite) TestPoolUpdateUnsupportedAPIVersion(c *gc.C) {
	s.mockAPI.APIVersion = 3
	_, err := s.runPoolUpdate(c, []string{"sunshine", "something=too", "another=one"})
	c.Check(err, gc.ErrorMatches, "updating storage pools is not supported by this API server")
}

type mockPoolUpdateAPI struct {
	APIVersion int
}

func (s mockPoolUpdateAPI) UpdatePool(pname string, pconfig map[string]interface{}) error {
	return nil
}

func (s mockPoolUpdateAPI) Close() error {
	return nil
}

func (s mockPoolUpdateAPI) BestAPIVersion() int {
	return s.APIVersion
}
