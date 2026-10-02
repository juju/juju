// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

type poolUpdateSuite struct {
	baseStorageSuite
}

func TestPoolUpdateSuite(t *testing.T) {
	tc.Run(t, &poolUpdateSuite{})
}

func (s *poolUpdateSuite) TestUpdatePool(c *tc.C) {
	defer s.setupMocks(c).Finish()
	user := names.NewUserTag("test-user")
	s.authorizer = apiservertesting.FakeAuthorizer{Tag: user, HasWriteTag: user}
	api := s.makeTestAPIForIAASModel(c)
	attrs := map[string]any{"size": 10, "type": "fast"}

	s.storageService.EXPECT().ReplaceStoragePool(
		c.Context(), "first", internalstorage.ProviderType("ebs"), attrs,
	).Return(nil)
	s.storageService.EXPECT().ReplaceStoragePool(
		c.Context(), "second", internalstorage.ProviderType(""), nil,
	).Return(nil)

	result, err := api.UpdatePool(c.Context(), params.StoragePoolArgs{Pools: []params.StoragePool{
		{Name: "first", Provider: "ebs", Attrs: attrs},
		{Name: "second"},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}, {}}})
}

func (s *poolUpdateSuite) TestUpdatePoolErrors(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIForIAASModel(c)
	tests := []struct {
		err  error
		code string
	}{
		{domainstorageerrors.StoragePoolNotFound, params.CodeNotFound},
		{domainstorageerrors.StoragePoolNameInvalid, params.CodeNotValid},
		{domainstorageerrors.ProviderTypeInvalid, params.CodeNotValid},
		{domainstorageerrors.ProviderTypeNotFound, params.CodeNotFound},
		{domainstorageerrors.StoragePoolAttributeInvalid{Key: "size", Message: "invalid size"}, params.CodeNotValid},
		{errors.New("unexpected failure"), ""},
	}
	args := params.StoragePoolArgs{}
	expected := params.ErrorResults{}
	for i, test := range tests {
		name := fmt.Sprintf("pool-%d", i)
		wrapped := errors.Errorf("replacing pool: %w", test.err)
		s.storageService.EXPECT().ReplaceStoragePool(
			c.Context(), name, internalstorage.ProviderType(""), nil,
		).Return(wrapped)
		args.Pools = append(args.Pools, params.StoragePool{Name: name})
		expected.Results = append(expected.Results, params.ErrorResult{
			Error: &params.Error{Code: test.code, Message: wrapped.Error()},
		})
	}
	s.storageService.EXPECT().ReplaceStoragePool(
		c.Context(), "success", internalstorage.ProviderType(""), nil,
	).Return(nil)
	args.Pools = append(args.Pools, params.StoragePool{Name: "success"})
	expected.Results = append(expected.Results, params.ErrorResult{})

	result, err := api.UpdatePool(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, expected)
}

func (s *poolUpdateSuite) TestUpdatePoolReadOnly(c *tc.C) {
	defer s.setupMocks(c).Finish()
	user := names.NewUserTag("test-user")
	s.authorizer = apiservertesting.FakeAuthorizer{Tag: user, HasReadTag: user}
	api := s.makeTestAPIForIAASModel(c)

	result, err := api.UpdatePool(c.Context(), params.StoragePoolArgs{Pools: []params.StoragePool{{Name: "pool"}}})
	paramsErr, ok := errors.AsType[*params.Error](err)
	c.Assert(ok, tc.IsTrue)
	c.Check(paramsErr.Code, tc.Equals, params.CodeUnauthorized)
	c.Check(result.Results, tc.HasLen, 0)
}

func (s *poolUpdateSuite) TestUpdatePoolNoPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.authorizer = apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("test-user")}
	api := s.makeTestAPIForIAASModel(c)

	result, err := api.UpdatePool(c.Context(), params.StoragePoolArgs{})
	paramsErr, ok := errors.AsType[*params.Error](err)
	c.Assert(ok, tc.IsTrue)
	c.Check(paramsErr.Code, tc.Equals, params.CodeUnauthorized)
	c.Check(result.Results, tc.HasLen, 0)
}

func (s *poolUpdateSuite) TestUpdatePoolEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIForIAASModel(c)

	result, err := api.UpdatePool(c.Context(), params.StoragePoolArgs{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 0)
}
