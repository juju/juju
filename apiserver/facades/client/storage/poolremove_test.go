// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

type poolRemoveSuite struct {
	baseStorageSuite
}

func TestPoolRemoveSuite(t *testing.T) {
	tc.Run(t, &poolRemoveSuite{})
}

func (s *poolRemoveSuite) TestRemovePool(c *tc.C) {
	defer s.setupMocks(c).Finish()
	user := names.NewUserTag("test-user")
	s.authorizer = apiservertesting.FakeAuthorizer{Tag: user, HasWriteTag: user}
	api := s.makeTestAPIForIAASModel(c)

	s.storageService.EXPECT().DeleteStoragePool(c.Context(), "first").Return(nil)
	s.storageService.EXPECT().DeleteStoragePool(c.Context(), "missing").Return(
		errors.Errorf("deleting pool: %w", domainstorageerrors.StoragePoolNotFound),
	)
	s.storageService.EXPECT().DeleteStoragePool(c.Context(), "broken").Return(errors.New("unexpected failure"))
	s.storageService.EXPECT().DeleteStoragePool(c.Context(), "used").Return(
		errors.Errorf("deleting storage pool %q: %w", "used", domainstorageerrors.StoragePoolInUse),
	)
	s.storageService.EXPECT().DeleteStoragePool(c.Context(), "last").Return(nil)

	result, err := api.RemovePool(c.Context(), params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{
			{Name: "first"}, {Name: "missing"}, {Name: "broken"}, {Name: "used"}, {Name: "last"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{
		{},
		{Error: &params.Error{Code: params.CodeNotFound, Message: "deleting pool: storage pool is not found"}},
		{Error: &params.Error{Message: "unexpected failure"}},
		{Error: &params.Error{Code: params.CodeNotValid, Message: `deleting storage pool "used": storage pool is in use`}},
		{},
	}})
}

func (s *poolRemoveSuite) TestRemovePoolReadOnly(c *tc.C) {
	defer s.setupMocks(c).Finish()
	user := names.NewUserTag("test-user")
	s.authorizer = apiservertesting.FakeAuthorizer{Tag: user, HasReadTag: user}
	api := s.makeTestAPIForIAASModel(c)

	result, err := api.RemovePool(c.Context(), params.StoragePoolDeleteArgs{
		Pools: []params.StoragePoolDeleteArg{{Name: "pool"}},
	})
	paramsErr, ok := errors.AsType[*params.Error](err)
	c.Assert(ok, tc.IsTrue)
	c.Check(paramsErr.Code, tc.Equals, params.CodeUnauthorized)
	c.Check(result.Results, tc.HasLen, 0)
}

func (s *poolRemoveSuite) TestRemovePoolNoPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.authorizer = apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("test-user")}
	api := s.makeTestAPIForIAASModel(c)

	result, err := api.RemovePool(c.Context(), params.StoragePoolDeleteArgs{})
	paramsErr, ok := errors.AsType[*params.Error](err)
	c.Assert(ok, tc.IsTrue)
	c.Check(paramsErr.Code, tc.Equals, params.CodeUnauthorized)
	c.Check(result.Results, tc.HasLen, 0)
}

func (s *poolRemoveSuite) TestRemovePoolEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIForIAASModel(c)

	result, err := api.RemovePool(c.Context(), params.StoragePoolDeleteArgs{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 0)
}
