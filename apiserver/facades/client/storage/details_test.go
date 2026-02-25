// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	corelife "github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	domainlife "github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/rpc/params"
)

type storageDetailsSuite struct {
	baseStorageSuite
}

func TestStorageDetailsSuite(t *testing.T) {
	tc.Run(t, &storageDetailsSuite{})
}

func (s *storageDetachSuite) TestStorageDetailsWithReadPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := names.NewUserTag("tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasReadTag: userTag,
		Tag:        userTag,
	}

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageTag := names.NewStorageTag("1")
	unitTag := names.NewUnitTag("foo/1")

	s.storageService.EXPECT().GetStorageInstanceUUIDForID(gomock.Any(), "1").Return(
		storageInstanceUUID, nil,
	)
	s.storageService.EXPECT().GetStorageInstanceInfo(
		gomock.Any(), storageInstanceUUID,
	).Return(domainstorage.StorageInstanceInfo{
		ID:         "1",
		Life:       domainlife.Alive,
		Kind:       domainstorage.StorageKindFilesystem,
		Persistent: false,
		UnitAttachments: []domainstorage.StorageInstanceUnitAttachment{
			{
				Life:     domainlife.Alive,
				Location: "/mnt/foo",
				MachineAttachment: &domainstorage.StorageInstanceMachineAttachment{
					MachineName: "1",
					MachineUUID: tc.Must(c, coremachine.NewUUID),
				},
				UnitName: "foo/1",
				UnitUUID: tc.Must(c, coreunit.NewUUID),
			},
		},
		UUID: storageInstanceUUID,
	}, nil)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.StorageDetails(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewStorageTag("1").String()},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.StorageDetailsResults{
		Results: []params.StorageDetailsResult{
			{
				Result: &params.StorageDetails{
					Attachments: map[string]params.StorageAttachmentDetails{
						unitTag.String(): params.StorageAttachmentDetails{
							Life:       corelife.Alive,
							Location:   "/mnt/foo",
							MachineTag: names.NewMachineTag("0").String(),
							StorageTag: storageTag.String(),
							UnitTag:    unitTag.String(),
						},
					},
					StorageTag: storageTag.String(),
					OwnerTag:   names.NewUnitTag("foo/1").String(),
					Life:       corelife.Alive,
					Kind:       params.StorageKindFilesystem,
					Persistent: false,
				},
			},
		},
	})
}
