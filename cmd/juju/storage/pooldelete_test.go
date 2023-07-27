// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/rpc/params"
)

type PoolRemoveSuite struct {
	SubStorageSuite
	mockAPI *mockPoolRemoveAPI
}

var _ = gc.Suite(&PoolRemoveSuite{})

func (s *PoolRemoveSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockPoolRemoveAPI{}
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

func (s *PoolRemoveSuite) TestPoolRemoveNotFound(c *gc.C) {
	s.mockAPI.err = params.Error{
		Code: params.CodeNotFound,
	}
	_, err := s.runPoolRemove(c, []string{"sunshine"})
	c.Assert(errors.Cause(err), gc.Equals, cmd.ErrSilent)
}

type mockPoolRemoveAPI struct {
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
