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

type PoolDeleteSuite struct {
	SubStorageSuite
	mockAPI *mockPoolDeleteAPI
}

var _ = gc.Suite(&PoolDeleteSuite{})

func (s *PoolDeleteSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockPoolDeleteAPI{APIVersion: 5}
}

func (s *PoolDeleteSuite) runPoolDelete(c *gc.C, args []string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, storage.NewPoolDeleteCommandForTest(s.mockAPI, s.store), args...)
}

func (s *PoolDeleteSuite) TestPoolDeleteOneArg(c *gc.C) {
	_, err := s.runPoolDelete(c, []string{"sunshine"})
	c.Check(err, jc.ErrorIsNil)
}

func (s *PoolDeleteSuite) TestPoolDeleteNoArgs(c *gc.C) {
	_, err := s.runPoolDelete(c, []string{})
	c.Check(err, gc.ErrorMatches, "pool deletion requires storage pool name")
}

func (s *PoolDeleteSuite) TestPoolDeleteErrorsManyArgs(c *gc.C) {
	_, err := s.runPoolDelete(c, []string{"sunshine", "lollypop"})
	c.Check(err, gc.ErrorMatches, "pool deletion requires storage pool name")
}

func (s *PoolUpdateSuite) TestPoolDeleteUnsupportedAPIVersion(c *gc.C) {
	s.mockAPI.APIVersion = 3
	_, err := s.runPoolUpdate(c, []string{"sunshine"})
	c.Check(err, gc.ErrorMatches, "pool update requires name and configuration attributes")
}

type mockPoolDeleteAPI struct {
	APIVersion int
}

func (s mockPoolDeleteAPI) DeletePool(pname string) error {
	return nil
}

func (s mockPoolDeleteAPI) Close() error {
	return nil
}

func (s mockPoolDeleteAPI) BestAPIVersion() int {
	return s.APIVersion
}
