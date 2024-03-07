// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/rpc/params"
)

type poolCreateSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&poolCreateSuite{})

func (s *poolCreateSuite) TestCreatePool(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.storageService = storage.NewMockStorageService(ctrl)
	s.storageService.EXPECT().CreateStoragePool(gomock.Any(), "pname", provider.LoopProviderType, nil).Return(nil)

	args := params.StoragePoolArgs{
		Pools: []params.StoragePool{{
			Name:     "pname",
			Provider: "loop",
			Attrs:    nil,
		}},
	}
	results, err := s.api.CreatePool(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *poolCreateSuite) TestCreatePoolError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.storageService = storage.NewMockStorageService(ctrl)
	s.storageService.EXPECT().CreateStoragePool(gomock.Any(), "doesnt-matter", gomock.Any(), gomock.Any()).Return(errors.New("as expected"))

	args := params.StoragePoolArgs{
		Pools: []params.StoragePool{{
			Name: "doesnt-matter",
		}},
	}
	results, err := s.api.CreatePool(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: "as expected",
	})
}
