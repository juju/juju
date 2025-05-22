// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	storageerrors "github.com/juju/juju/domain/storage/errors"
	internalstorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

type poolUpdateSuite struct {
	baseStorageSuite
}

func TestPoolUpdateSuite(t *stdtesting.T) {
	tc.Run(t, &poolUpdateSuite{})
}

func (s *poolUpdateSuite) TestUpdatePool(c *tc.C) {
	defer s.setupMocks(c).Finish()

	poolName := fmt.Sprintf("%v%v", tstName, 0)
	newAttrs := map[string]interface{}{
		"foo1": "bar1",
		"zip":  "zoom",
	}
	s.storageService.EXPECT().ReplaceStoragePool(gomock.Any(), poolName, internalstorage.ProviderType(""), newAttrs).Return(nil)

	args := params.StoragePoolArgs{
		Pools: []params.StoragePool{{
			Name:  poolName,
			Attrs: newAttrs,
		}},
	}
	results, err := s.api.UpdatePool(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
}

func (s *poolUpdateSuite) TestUpdatePoolError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	poolName := fmt.Sprintf("%v%v", tstName, 0)
	args := params.StoragePoolArgs{
		Pools: []params.StoragePool{{
			Name: poolName,
		}},
	}
	s.storageService.EXPECT().ReplaceStoragePool(gomock.Any(), poolName, internalstorage.ProviderType(""), nil).Return(storageerrors.PoolNotFoundError)

	results, err := s.api.UpdatePool(c.Context(), args)
	c.Assert(err, tc.IsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "storage pool is not found")
}
