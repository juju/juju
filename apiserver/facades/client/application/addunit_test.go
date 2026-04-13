// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	domainapplicationservice "github.com/juju/juju/domain/application/service"
	domainstorage "github.com/juju/juju/domain/storage"
)

type addUnitSuite struct{}

func TestAddUnit(t *testing.T) {
	tc.Run(t, &addUnitSuite{})
}

func (s *addUnitSuite) TestMakeAddUnitArgsWithoutPlacementOrStorage(c *tc.C) {
	result := makeAddUnitArgs(3, nil, nil)
	c.Assert(result, tc.DeepEquals, []domainapplicationservice.AddUnitArg{
		{}, {}, {},
	})
}

func (s *addUnitSuite) TestMakeAddUnitArgsWithPlacementAndStorage(c *tc.C) {
	placements := []*instance.Placement{
		{Scope: "zone", Directive: "az1"},
		{Scope: "zone", Directive: "az2"},
	}
	storageInstances := [][]domainstorage.StorageInstanceUUID{
		{"storage-uuid-1"},
		{"storage-uuid-2", "storage-uuid-3"},
	}

	result := makeAddUnitArgs(3, placements, storageInstances)
	c.Assert(result, tc.DeepEquals, []domainapplicationservice.AddUnitArg{{
		Placement: &instance.Placement{
			Scope: "zone", Directive: "az1",
		},
		StorageInstancesToAttach: []domainstorage.StorageInstanceUUID{
			"storage-uuid-1",
		},
	}, {
		Placement: &instance.Placement{
			Scope: "zone", Directive: "az2",
		},
		StorageInstancesToAttach: []domainstorage.StorageInstanceUUID{
			"storage-uuid-2", "storage-uuid-3",
		},
	}, {
		Placement:                nil,
		StorageInstancesToAttach: nil,
	}})
}

func (s *addUnitSuite) TestMakeIAASAddUnitArgsWithoutPlacementOrStorage(c *tc.C) {
	result := makeIAASAddUnitArgs(2, nil, nil)
	c.Assert(result, tc.DeepEquals, []domainapplicationservice.AddIAASUnitArg{
		{AddUnitArg: domainapplicationservice.AddUnitArg{}},
		{AddUnitArg: domainapplicationservice.AddUnitArg{}},
	})
}

func (s *addUnitSuite) TestMakeIAASAddUnitArgsWithPlacementAndStorage(c *tc.C) {
	placements := []*instance.Placement{
		{Scope: "zone", Directive: "az1"},
	}
	storageInstances := [][]domainstorage.StorageInstanceUUID{
		{"storage-uuid-1"},
	}

	result := makeIAASAddUnitArgs(2, placements, storageInstances)
	c.Assert(result, tc.DeepEquals, []domainapplicationservice.AddIAASUnitArg{{
		AddUnitArg: domainapplicationservice.AddUnitArg{
			Placement: &instance.Placement{
				Scope: "zone", Directive: "az1",
			},
			StorageInstancesToAttach: []domainstorage.StorageInstanceUUID{
				"storage-uuid-1",
			},
		},
	}, {
		AddUnitArg: domainapplicationservice.AddUnitArg{
			Placement:                nil,
			StorageInstancesToAttach: nil,
		},
	}})
}

func (s *addUnitSuite) TestMakeCAASAddUnitArgsWithoutPlacementOrStorage(c *tc.C) {
	result := makeCAASAddUnitArgs(2, nil, nil)
	c.Assert(result, tc.DeepEquals, []domainapplicationservice.AddUnitArg{
		{}, {},
	})
}

func (s *addUnitSuite) TestMakeCAASAddUnitArgsWithPlacementAndStorage(c *tc.C) {
	placements := []*instance.Placement{
		{Scope: "zone", Directive: "az1"},
	}
	storageInstances := [][]domainstorage.StorageInstanceUUID{
		{"storage-uuid-1"},
	}

	result := makeCAASAddUnitArgs(2, placements, storageInstances)
	c.Assert(result, tc.DeepEquals, []domainapplicationservice.AddUnitArg{{
		Placement: &instance.Placement{
			Scope: "zone", Directive: "az1",
		},
		StorageInstancesToAttach: []domainstorage.StorageInstanceUUID{
			"storage-uuid-1",
		},
	}, {
		Placement:                nil,
		StorageInstancesToAttach: nil,
	}})
}
