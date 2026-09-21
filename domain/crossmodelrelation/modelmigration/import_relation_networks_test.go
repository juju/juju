// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/description/v12"
	"github.com/juju/tc"

	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/crossmodelrelation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type importRelationNetworksSuite struct {
	testhelpers.IsolationSuite

	importService *MockRelationNetworkImportService
}

func TestImportRelationNetworksSuite(t *testing.T) {
	tc.Run(t, &importRelationNetworksSuite{})
}

func (s *importRelationNetworksSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockRelationNetworkImportService(ctrl)

	c.Cleanup(func() {
		s.importService = nil
	})

	return ctrl
}

func (s *importRelationNetworksSuite) TestImportRelationNetworks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddRelationNetwork(description.RelationNetworkArgs{
		ID:          "mysql:db remote-13ea:db:ingress:default",
		RelationKey: "mysql:db remote-13ea:db",
		CIDRS:       []string{"10.0.0.0/24"},
	})
	model.AddRelationNetwork(description.RelationNetworkArgs{
		ID:          "mysql:db remote-13ea:db:egress:default",
		RelationKey: "mysql:db remote-13ea:db",
		CIDRS:       []string{"192.168.0.0/16"},
	})
	// The admin override takes precedence over the default networks of the
	// same direction.
	model.AddRelationNetwork(description.RelationNetworkArgs{
		ID:          "mysql:db remote-13ea:db:egress:override",
		RelationKey: "mysql:db remote-13ea:db",
		CIDRS:       []string{"192.168.1.0/24"},
	})
	model.AddRelationNetwork(description.RelationNetworkArgs{
		ID:          "wordpress:db mysql:db:egress:default",
		RelationKey: "wordpress:db mysql:db",
		CIDRS:       []string{"10.1.0.0/16"},
	})

	key, err := relation.NewKeyFromString("mysql:db remote-13ea:db")
	c.Assert(err, tc.ErrorIsNil)
	otherKey, err := relation.NewKeyFromString("wordpress:db mysql:db")
	c.Assert(err, tc.ErrorIsNil)

	// The ingress and override egress networks of the first relation are
	// imported, along with the egress networks of the second relation. The
	// default egress networks of the first relation are dropped in favour of
	// the override.
	s.importService.EXPECT().ImportRelationNetworks(gomock.Any(), []crossmodelrelation.RelationNetworkImport{
		{
			RelationKey: key,
			Direction:   crossmodelrelation.RelationNetworkIngress,
			CIDRs:       []string{"10.0.0.0/24"},
		},
	}).Return(nil)
	s.importService.EXPECT().ImportRelationNetworks(gomock.Any(), []crossmodelrelation.RelationNetworkImport{
		{
			RelationKey: key,
			Direction:   crossmodelrelation.RelationNetworkEgress,
			CIDRs:       []string{"192.168.1.0/24"},
		},
	}).Return(nil)
	s.importService.EXPECT().ImportRelationNetworks(gomock.Any(), []crossmodelrelation.RelationNetworkImport{
		{
			RelationKey: otherKey,
			Direction:   crossmodelrelation.RelationNetworkEgress,
			CIDRs:       []string{"10.1.0.0/16"},
		},
	}).Return(nil)

	op := importRelationNetworksOperation{
		importService: s.importService,
		logger:        loggertesting.WrapCheckLog(c),
	}

	err = op.Execute(c.Context(), model)

	c.Assert(err, tc.ErrorIsNil)
}

func (s *importRelationNetworksSuite) TestImportRelationNetworksRelationNotFoundSkipped(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddRelationNetwork(description.RelationNetworkArgs{
		ID:          "mysql:db remote-13ea:db:ingress:default",
		RelationKey: "mysql:db remote-13ea:db",
		CIDRS:       []string{"10.0.0.0/24"},
	})
	model.AddRelationNetwork(description.RelationNetworkArgs{
		ID:          "wordpress:db mysql:db:egress:default",
		RelationKey: "wordpress:db mysql:db",
		CIDRS:       []string{"10.1.0.0/16"},
	})

	// A network referencing a relation that was not migrated only skips that
	// network; the remaining networks are still imported.
	s.importService.EXPECT().ImportRelationNetworks(gomock.Any(), gomock.Any()).
		Return(relationerrors.RelationNotFound)
	s.importService.EXPECT().ImportRelationNetworks(gomock.Any(), gomock.Any()).
		Return(nil)

	op := importRelationNetworksOperation{
		importService: s.importService,
		logger:        loggertesting.WrapCheckLog(c),
	}

	err := op.Execute(c.Context(), model)

	c.Assert(err, tc.ErrorIsNil)
}

func (s *importRelationNetworksSuite) TestImportRelationNetworksError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddRelationNetwork(description.RelationNetworkArgs{
		ID:          "mysql:db remote-13ea:db:ingress:default",
		RelationKey: "mysql:db remote-13ea:db",
		CIDRS:       []string{"10.0.0.0/24"},
	})

	s.importService.EXPECT().ImportRelationNetworks(gomock.Any(), gomock.Any()).
		Return(errors.New("boom"))

	op := importRelationNetworksOperation{
		importService: s.importService,
		logger:        loggertesting.WrapCheckLog(c),
	}

	err := op.Execute(c.Context(), model)

	c.Assert(err, tc.ErrorMatches, ".*boom")
}

func (s *importRelationNetworksSuite) TestImportRelationNetworksNoNetworks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	op := importRelationNetworksOperation{
		importService: s.importService,
		logger:        loggertesting.WrapCheckLog(c),
	}

	err := op.Execute(c.Context(), description.NewModel(description.ModelArgs{}))

	c.Assert(err, tc.ErrorIsNil)
}

func (s *importRelationNetworksSuite) TestImportRelationNetworksBadID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddRelationNetwork(description.RelationNetworkArgs{
		ID:          "not-a-relation-network-id",
		RelationKey: "mysql:db remote-13ea:db",
		CIDRS:       []string{"10.0.0.0/24"},
	})

	op := importRelationNetworksOperation{
		importService: s.importService,
		logger:        loggertesting.WrapCheckLog(c),
	}

	err := op.Execute(c.Context(), model)

	c.Assert(err, tc.ErrorMatches, ".*parsing relation network ID.*expected at least 3 colon separated parts.*")
}
