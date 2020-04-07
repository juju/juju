// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/storage"
	_ "github.com/juju/juju/provider/dummy"
)

type PoolRemoveSuite struct {
	SubStorageSuite
	mockAPI *mockPoolRemoveAPI
}

var _ = gc.Suite(&PoolRemoveSuite{})

func (s *PoolRemoveSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockPoolRemoveAPI{APIVersion: 5}
}

func (s *PoolRemoveSuite) runPoolRemove(c *gc.C, args []string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, storage.NewPoolRemoveCommandForTest(s.mockAPI, s.store), args...)
}

func (s *PoolRemoveSuite) TestPoolRemoveOneArg(c *gc.C) {
	_, err := s.runPoolRemove(c, []string{"sunshine"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(s.mockAPI.RemovedPools), gc.Equals, 1)
	c.Assert(s.mockAPI.RemovedPools[0], gc.Equals, "sunshine")
}

func (s *PoolRemoveSuite) TestPoolRemoveNoArgs(c *gc.C) {
	_, err := s.runPoolRemove(c, []string{})
	c.Check(err, gc.ErrorMatches, "pool removal requires storage pool name")
	c.Assert(len(s.mockAPI.RemovedPools), gc.Equals, 0)
}

func (s *PoolRemoveSuite) TestPoolRemoveErrorsManyArgs(c *gc.C) {
	_, err := s.runPoolRemove(c, []string{"sunshine", "lollypop"})
	c.Check(err, gc.ErrorMatches, `unrecognized args: \["lollypop"\]`)
	c.Assert(len(s.mockAPI.RemovedPools), gc.Equals, 0)
}

func (s *PoolRemoveSuite) TestPoolRemoveUnsupportedAPIVersion(c *gc.C) {
	s.mockAPI.APIVersion = 3
	_, err := s.runPoolRemove(c, []string{"sunshine"})
	c.Check(err, gc.ErrorMatches, "removing storage pools is not supported by this version of Juju")
	c.Assert(len(s.mockAPI.RemovedPools), gc.Equals, 0)
}

func (s *PoolRemoveSuite) TestPoolRemoveNotFound(c *gc.C) {
	s.mockAPI.err = params.Error{
		Code: params.CodeNotFound,
	}
	_, err := s.runPoolRemove(c, []string{"sunshine"})
	c.Assert(errors.Cause(err), gc.Equals, cmd.ErrSilent)
}

type mockPoolRemoveAPI struct {
	APIVersion   int
	RemovedPools []string
	err          error
}

func (s *mockPoolRemoveAPI) RemovePool(pname string) error {
	s.RemovedPools = append(s.RemovedPools, pname)
	return s.err
}

func (s mockPoolRemoveAPI) Close() error {
	return nil
}

func (s mockPoolRemoveAPI) BestAPIVersion() int {
	return s.APIVersion
}
