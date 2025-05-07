// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/rpc/params"
)

type PoolRemoveSuite struct {
	SubStorageSuite
	mockAPI *mockPoolRemoveAPI
}

var _ = tc.Suite(&PoolRemoveSuite{})

func (s *PoolRemoveSuite) SetUpTest(c *tc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockPoolRemoveAPI{}
}

func (s *PoolRemoveSuite) runPoolRemove(c *tc.C, args []string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, storage.NewPoolRemoveCommandForTest(s.mockAPI, s.store), args...)
}

func (s *PoolRemoveSuite) TestPoolRemoveOneArg(c *tc.C) {
	_, err := s.runPoolRemove(c, []string{"sunshine"})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(s.mockAPI.RemovedPools), tc.Equals, 1)
	c.Assert(s.mockAPI.RemovedPools[0], tc.Equals, "sunshine")
}

func (s *PoolRemoveSuite) TestPoolRemoveNoArgs(c *tc.C) {
	_, err := s.runPoolRemove(c, []string{})
	c.Check(err, tc.ErrorMatches, "pool removal requires storage pool name")
	c.Assert(len(s.mockAPI.RemovedPools), tc.Equals, 0)
}

func (s *PoolRemoveSuite) TestPoolRemoveErrorsManyArgs(c *tc.C) {
	_, err := s.runPoolRemove(c, []string{"sunshine", "lollypop"})
	c.Check(err, tc.ErrorMatches, `unrecognized args: \["lollypop"\]`)
	c.Assert(len(s.mockAPI.RemovedPools), tc.Equals, 0)
}

func (s *PoolRemoveSuite) TestPoolRemoveNotFound(c *tc.C) {
	s.mockAPI.err = params.Error{
		Code: params.CodeNotFound,
	}
	_, err := s.runPoolRemove(c, []string{"sunshine"})
	c.Assert(errors.Cause(err), tc.Equals, cmd.ErrSilent)
}

type mockPoolRemoveAPI struct {
	RemovedPools []string
	err          error
}

func (s *mockPoolRemoveAPI) RemovePool(ctx context.Context, pname string) error {
	s.RemovedPools = append(s.RemovedPools, pname)
	return s.err
}

func (s mockPoolRemoveAPI) Close() error {
	return nil
}
