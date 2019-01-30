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
	c.Assert(len(s.mockAPI.DeletedPools), gc.Equals, 1)
	c.Assert(s.mockAPI.DeletedPools[0], gc.Equals, "sunshine")
}

func (s *PoolDeleteSuite) TestPoolDeleteNoArgs(c *gc.C) {
	_, err := s.runPoolDelete(c, []string{})
	c.Check(err, gc.ErrorMatches, "pool deletion requires storage pool name")
	c.Assert(len(s.mockAPI.DeletedPools), gc.Equals, 0)
}

func (s *PoolDeleteSuite) TestPoolDeleteErrorsManyArgs(c *gc.C) {
	_, err := s.runPoolDelete(c, []string{"sunshine", "lollypop"})
	c.Check(err, gc.ErrorMatches, `unrecognized args: \["lollypop"\]`)
	c.Assert(len(s.mockAPI.DeletedPools), gc.Equals, 0)
}

func (s *PoolDeleteSuite) TestPoolDeleteUnsupportedAPIVersion(c *gc.C) {
	s.mockAPI.APIVersion = 3
	_, err := s.runPoolDelete(c, []string{"sunshine"})
	c.Check(err, gc.ErrorMatches, "deleting storage pools is not supported by this version of Juju")
	c.Assert(len(s.mockAPI.DeletedPools), gc.Equals, 0)
}

type mockPoolDeleteAPI struct {
	APIVersion   int
	DeletedPools []string
}

func (s *mockPoolDeleteAPI) DeletePool(pname string) error {
	s.DeletedPools = append(s.DeletedPools, pname)
	return nil
}

func (s mockPoolDeleteAPI) Close() error {
	return nil
}

func (s mockPoolDeleteAPI) BestAPIVersion() int {
	return s.APIVersion
}
