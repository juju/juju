// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	corestatus "github.com/juju/juju/core/status"
	statusservice "github.com/juju/juju/domain/status/service"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/rpc/params"
)

type storageSuite struct {
	baseStorageSuite
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

// TestListStorageDetailsPersistent verifies that a block storage instance
// marked as persistent propagates Persistent=true to the API response, and
// that a filesystem-kind instance without a backing volume remains false.
func (s *storageSuite) TestListStorageDetailsPersistent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.statusService.EXPECT().GetAllStorageInstanceStatuses(gomock.Any()).Return(
		[]statusservice.StorageInstance{
			{
				ID:         "single-blk/0",
				Kind:       domainstorage.StorageKindBlock,
				Persistent: true,
				Status:     corestatus.StatusInfo{Status: corestatus.Attached},
			}, {
				ID:         "single-fs/1",
				Kind:       domainstorage.StorageKindFilesystem,
				Persistent: false,
				Status:     corestatus.StatusInfo{Status: corestatus.Attached},
			},
		}, nil,
	)

	api := s.makeTestAPIForIAASModel(c)
	results, err := api.ListStorageDetails(c.Context(), params.StorageFilters{
		Filters: []params.StorageFilter{{}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)

	details := results.Results[0].Result
	c.Assert(details, tc.HasLen, 2)
	c.Check(details[0].Persistent, tc.IsTrue)
	c.Check(details[1].Persistent, tc.IsFalse)
}
