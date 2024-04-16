// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
)

type spaceSuite struct {
	testing.IsolationSuite

	st             *MockState
	logger         *MockLogger
	provider       *MockProvider
	providerGetter func(context.Context) (Provider, error)
}

var _ = gc.Suite(&spaceSuite{})

func (s *spaceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.provider = NewMockProvider(ctrl)
	s.providerGetter = func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}

	return ctrl
}

func (s *spaceSuite) TestGenerateFanSubnetID(c *gc.C) {
	obtained := generateFanSubnetID("10.0.0.0/24", "provider-id")
	c.Check(obtained, gc.Equals, "provider-id-INFAN-10-0-0-0-24")
	// Empty providerID
	obtained = generateFanSubnetID("192.168.0.0/16", "")
	c.Check(obtained, gc.Equals, "-INFAN-192-168-0-0-16")
}

func (s *spaceSuite) TestAddSpaceInvalidNameEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Make sure no calls to state are done
	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	_, err := NewService(s.st, s.logger).AddSpace(
		context.Background(),
		network.SpaceInfo{})
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("space name \"\" not valid"))
}

func (s *spaceSuite) TestAddSpaceInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Make sure no calls to state are done
	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	_, err := NewService(s.st, s.logger).AddSpace(
		context.Background(),
		network.SpaceInfo{
			Name:       "-bad name-",
			ProviderId: "provider-id",
		})
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("space name \"-bad name-\" not valid"))
}

func (s *spaceSuite) TestAddSpaceErrorAdding(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), "0", network.Id("provider-id"), []string{"0"}).
		Return(errors.Errorf("updating subnet %q using space uuid \"space0\"", "0"))

	_, err := NewService(s.st, s.logger).AddSpace(
		context.Background(),
		network.SpaceInfo{
			Name:       "0",
			ProviderId: "provider-id",
			Subnets: network.SubnetInfos{
				{
					ID: network.Id("0"),
				},
			},
		})
	c.Assert(err, gc.ErrorMatches, "updating subnet \"0\" using space uuid \"space0\"")
}

func (s *spaceSuite) TestAddSpace(c *gc.C) {
	defer s.setupMocks(c).Finish()

	var expectedUUID string
	// Verify that the passed UUID is also returned.
	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), "space0", network.Id("provider-id"), []string{}).
		Do(
			func(
				ctx context.Context,
				uuid string,
				name string,
				providerID network.Id,
				subnetIDs []string,
			) error {
				expectedUUID = uuid
				return nil
			})

	returnedUUID, err := NewService(s.st, s.logger).AddSpace(
		context.Background(),
		network.SpaceInfo{
			Name:       "space0",
			ProviderId: "provider-id",
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(returnedUUID.String(), gc.Not(gc.Equals), "")
	c.Check(returnedUUID.String(), gc.Equals, expectedUUID)
}

func (s *spaceSuite) TestUpdateSpaceName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	newName := "new-space-name"
	s.st.EXPECT().UpdateSpace(gomock.Any(), network.AlphaSpaceId, newName).Return(nil)
	err := NewService(s.st, s.logger).UpdateSpace(context.Background(), network.AlphaSpaceId, newName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestRetrieveSpaceByID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSpace(gomock.Any(), network.AlphaSpaceId)
	_, err := NewService(s.st, s.logger).Space(context.Background(), network.AlphaSpaceId)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestRetrieveSpaceByIDNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSpace(gomock.Any(), "unknown-space").
		Return(nil, errors.NotFoundf("space %q", "unknown-space"))
	_, err := NewService(s.st, s.logger).Space(context.Background(), "unknown-space")
	c.Assert(err, gc.ErrorMatches, "space \"unknown-space\" not found")
}

func (s *spaceSuite) TestRetrieveSpaceByName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSpaceByName(gomock.Any(), network.AlphaSpaceName)
	_, err := NewService(s.st, s.logger).SpaceByName(context.Background(), network.AlphaSpaceName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestRetrieveSpaceByNameNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSpaceByName(gomock.Any(), "unknown-space-name").
		Return(nil, errors.NotFoundf("space with name %q", "unknown-space-name"))
	_, err := NewService(s.st, s.logger).SpaceByName(context.Background(), "unknown-space-name")
	c.Assert(err, gc.ErrorMatches, "space with name \"unknown-space-name\" not found")
}

func (s *spaceSuite) TestRetrieveAllSpaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetAllSpaces(gomock.Any())
	_, err := NewService(s.st, s.logger).GetAllSpaces(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestRemoveSpace(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().DeleteSpace(gomock.Any(), "space0")
	err := NewService(s.st, s.logger).RemoveSpace(context.Background(), "space0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestSaveProviderSubnetsWithoutSpaceUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	twoSubnets := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "10.0.0.1/24",
		},
		{
			ProviderId:        "2",
			AvailabilityZones: []string{"3", "4"},
			CIDR:              "10.100.30.1/24",
		},
	}

	s.st.EXPECT().UpsertSubnets(context.Background(), twoSubnets)

	err := NewProviderService(s.st, s.providerGetter, s.logger).saveProviderSubnets(context.Background(), twoSubnets, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestSaveProviderSubnetsOnlyAddsSubnets(c *gc.C) {
	defer s.setupMocks(c).Finish()

	twoSubnets := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "10.0.0.1/24",
		},
		{
			ProviderId:        "2",
			AvailabilityZones: []string{"3", "4"},
			CIDR:              "10.100.30.1/24",
		},
	}

	s.st.EXPECT().UpsertSubnets(context.Background(), twoSubnets)

	err := NewProviderService(s.st, s.providerGetter, s.logger).saveProviderSubnets(context.Background(), twoSubnets, "", nil)
	c.Assert(err, jc.ErrorIsNil)

	anotherSubnet := []network.SubnetInfo{
		{
			ProviderId:        "3",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "10.0.1.1/24",
		},
	}

	s.st.EXPECT().UpsertSubnets(context.Background(), anotherSubnet)

	err = NewProviderService(s.st, s.providerGetter, s.logger).saveProviderSubnets(context.Background(), anotherSubnet, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestSaveProviderSubnetsOnlyIdempotent(c *gc.C) {
	defer s.setupMocks(c).Finish()

	oneSubnet := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "10.0.0.1/24",
		},
	}

	s.st.EXPECT().UpsertSubnets(context.Background(), oneSubnet)
	err := NewProviderService(s.st, s.providerGetter, s.logger).saveProviderSubnets(context.Background(), oneSubnet, "", nil)
	c.Assert(err, jc.ErrorIsNil)

	// We expect the same subnets to be passed to the state methods.
	s.st.EXPECT().UpsertSubnets(context.Background(), oneSubnet)
	err = NewProviderService(s.st, s.providerGetter, s.logger).saveProviderSubnets(context.Background(), oneSubnet, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestSaveProviderSubnetsWithFAN(c *gc.C) {
	defer s.setupMocks(c).Finish()

	twoSubnets := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "10.0.0.1/24",
		},
		{
			ProviderId:        "2",
			AvailabilityZones: []string{"3", "4"},
			CIDR:              "10.100.30.1/24",
		},
	}
	expected := append(twoSubnets, network.SubnetInfo{
		ProviderId:        network.Id(fmt.Sprintf("2-%s-10-100-30-0-24", network.InFan)),
		AvailabilityZones: []string{"3", "4"},
		CIDR:              "253.30.0.0/16",
		FanInfo: &network.FanCIDRs{
			FanLocalUnderlay: "10.100.30.1/24",
			FanOverlay:       "253.0.0.0/8",
		}},
	)

	s.st.EXPECT().UpsertSubnets(context.Background(), gomock.Any()).Do(
		func(ctx context.Context, subnets []network.SubnetInfo) {
			c.Check(subnets, gc.HasLen, 3)
			c.Check(subnets, gc.DeepEquals, expected)
		},
	)

	fanConfig, err := network.ParseFanConfig("10.100.0.0/16=253.0.0.0/8")
	c.Assert(err, jc.ErrorIsNil)
	err = NewProviderService(s.st, s.providerGetter, s.logger).saveProviderSubnets(context.Background(), twoSubnets, "", fanConfig)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestSaveProviderSubnetsIgnoreInterfaceLocalMulticast(c *gc.C) {
	defer s.setupMocks(c).Finish()

	oneSubnet := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "ff51:dead:beef::/48",
		},
	}

	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Times(0)
	err := NewProviderService(s.st, s.providerGetter, s.logger).saveProviderSubnets(context.Background(), oneSubnet, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestSaveProviderSubnetsIgnoreLinkLocalMulticast(c *gc.C) {
	defer s.setupMocks(c).Finish()

	oneSubnet := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "ff32:dead:beef::/48",
		},
	}

	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Times(0)
	err := NewProviderService(s.st, s.providerGetter, s.logger).saveProviderSubnets(context.Background(), oneSubnet, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestSaveProviderSubnetsIgnoreIPV6LinkLocalUnicast(c *gc.C) {
	defer s.setupMocks(c).Finish()

	oneSubnet := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "fe80:dead:beef::/48",
		},
	}

	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Times(0)
	err := NewProviderService(s.st, s.providerGetter, s.logger).saveProviderSubnets(context.Background(), oneSubnet, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestSaveProviderSubnetsIgnoreIPV4LinkLocalUnicast(c *gc.C) {
	defer s.setupMocks(c).Finish()

	oneSubnet := []network.SubnetInfo{
		{
			ProviderId:        "1",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "169.254.13.0/24",
		},
	}

	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Times(0)
	err := NewProviderService(s.st, s.providerGetter, s.logger).saveProviderSubnets(context.Background(), oneSubnet, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestReloadSpacesUsingSubnets(c *gc.C) {
	defer s.setupMocks(c).Finish()

	subnets := []network.SubnetInfo{
		{CIDR: "10.0.0.1/12"},
		{CIDR: "10.12.24.1/24"},
	}

	s.provider.EXPECT().SupportsSpaceDiscovery(gomock.Any()).Return(false, nil)
	s.provider.EXPECT().Subnets(gomock.Any(), instance.UnknownId, nil).Return(subnets, nil)
	s.logger.EXPECT().Debugf("environ does not support space discovery, falling back to subnet discovery")
	s.st.EXPECT().UpsertSubnets(gomock.Any(), subnets)

	err := NewProviderService(s.st, s.providerGetter, s.logger).
		ReloadSpaces(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestReloadSpacesUsingSubnetsFailsOnSave(c *gc.C) {
	defer s.setupMocks(c).Finish()

	subnets := []network.SubnetInfo{
		{CIDR: "10.0.0.1/12"},
		{CIDR: "10.12.24.1/24"},
	}

	s.provider.EXPECT().SupportsSpaceDiscovery(gomock.Any()).Return(false, nil)
	s.provider.EXPECT().Subnets(gomock.Any(), instance.UnknownId, nil).Return(subnets, nil)
	s.logger.EXPECT().Debugf("environ does not support space discovery, falling back to subnet discovery")
	s.st.EXPECT().UpsertSubnets(gomock.Any(), subnets).Return(errors.New("boom"))

	err := NewProviderService(s.st, s.providerGetter, s.logger).
		ReloadSpaces(context.Background(), nil)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *spaceSuite) TestReloadSpacesNotNetworkEnviron(c *gc.C) {
	defer s.setupMocks(c).Finish()

	providerGetterFails := func(ctx context.Context) (Provider, error) {
		return nil, errors.NotSupported
	}
	err := NewProviderService(s.st, providerGetterFails, s.logger).
		ReloadSpaces(context.Background(), nil)

	c.Assert(err, gc.ErrorMatches, "spaces discovery in a non-networking environ not supported")
}

func (s *spaceSuite) TestSaveProviderSpaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	res := []network.SpaceInfo{
		{
			ID:         "1",
			Name:       "space1",
			ProviderId: network.Id("1"),
		},
	}
	s.st.EXPECT().GetAllSpaces(gomock.Any()).Return(res, nil)

	subnets := network.SubnetInfos{
		{
			CIDR:    "10.0.0.1/12",
			SpaceID: "1",
		},
	}
	spaces := []network.SpaceInfo{
		{ProviderId: network.Id("1"), Subnets: subnets},
	}
	s.st.EXPECT().UpsertSubnets(gomock.Any(), subnets)

	providerService := NewProviderService(s.st, s.providerGetter, s.logger)
	provider := NewProviderSpaces(providerService, s.logger)
	fanConfig, err := network.ParseFanConfig("10.100.0.0/16=253.0.0.0/8")
	c.Assert(err, jc.ErrorIsNil)
	err = provider.saveSpaces(context.Background(), spaces, fanConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.modelSpaceMap, gc.DeepEquals, map[network.Id]network.SpaceInfo{
		network.Id("1"): res[0],
	})
}

func (s *spaceSuite) TestSaveProviderSpacesWithoutProviderId(c *gc.C) {
	defer s.setupMocks(c).Finish()

	res := []network.SpaceInfo{
		{
			ID:         "1",
			Name:       "space1",
			ProviderId: network.Id("1"),
		},
	}
	s.st.EXPECT().GetAllSpaces(gomock.Any()).Return(res, nil)

	subnets := network.SubnetInfos{
		{
			CIDR: "10.0.0.1/12",
		},
	}
	spaces := []network.SpaceInfo{
		{ProviderId: network.Id("2"), Subnets: subnets},
	}
	s.logger.EXPECT().Debugf("Adding space %s from provider %s", "empty", "2")
	var receivedSpaceID string
	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), "empty", network.Id("2"), []string{}).
		Do(func(ctx context.Context, uuid string, name string, providerID network.Id, subnetIDs []string) error {
			receivedSpaceID = uuid
			return nil
		}).
		Return(nil)
	s.st.EXPECT().UpsertSubnets(gomock.Any(), subnets)

	providerService := NewProviderService(s.st, s.providerGetter, s.logger)
	provider := NewProviderSpaces(providerService, s.logger)
	fanConfig, err := network.ParseFanConfig("10.100.0.0/16=253.0.0.0/8")
	c.Assert(err, jc.ErrorIsNil)
	err = provider.saveSpaces(context.Background(), spaces, fanConfig)
	c.Assert(err, jc.ErrorIsNil)
	addedSpace := network.SpaceInfo{
		ID:         receivedSpaceID,
		Name:       "empty",
		ProviderId: network.Id("2"),
	}
	c.Assert(provider.modelSpaceMap, gc.DeepEquals, map[network.Id]network.SpaceInfo{
		network.Id("1"): res[0],
		network.Id("2"): addedSpace,
	})
}

func (s *spaceSuite) TestSaveProviderSpacesDeltaSpaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	providerService := NewProviderService(s.st, s.providerGetter, s.logger)
	provider := NewProviderSpaces(providerService, s.logger)
	c.Assert(provider.deltaSpaces(), gc.DeepEquals, network.MakeIDSet())
}

func (s *spaceSuite) TestSaveProviderSpacesDeltaSpacesAfterNotUpdated(c *gc.C) {
	defer s.setupMocks(c).Finish()

	res := []network.SpaceInfo{
		{
			ID:         "1",
			Name:       "space1",
			ProviderId: network.Id("1"),
		},
	}
	s.st.EXPECT().GetAllSpaces(gomock.Any()).Return(res, nil)

	subnets := network.SubnetInfos{
		{
			CIDR: "10.0.0.1/12",
		},
	}
	spaces := []network.SpaceInfo{
		{ProviderId: network.Id("2"), Subnets: subnets},
	}
	s.logger.EXPECT().Debugf("Adding space %s from provider %s", "empty", "2")
	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), "empty", network.Id("2"), []string{}).
		Return(nil)
	s.st.EXPECT().UpsertSubnets(gomock.Any(), subnets)

	providerService := NewProviderService(s.st, s.providerGetter, s.logger)
	provider := NewProviderSpaces(providerService, s.logger)
	fanConfig, err := network.ParseFanConfig("10.100.0.0/16=253.0.0.0/8")
	c.Assert(err, jc.ErrorIsNil)
	err = provider.saveSpaces(context.Background(), spaces, fanConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.deltaSpaces(), gc.DeepEquals, network.MakeIDSet(network.Id("1")))
}

func (s *spaceSuite) TestDeleteProviderSpacesWithNoDeltas(c *gc.C) {
	defer s.setupMocks(c).Finish()

	providerService := NewProviderService(s.st, s.providerGetter, s.logger)
	provider := NewProviderSpaces(providerService, s.logger)
	warnings, err := provider.deleteSpaces(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string(nil))
}

func (s *spaceSuite) TestDeleteProviderSpaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().DeleteSpace(gomock.Any(), "1")

	providerService := NewProviderService(s.st, s.providerGetter, s.logger)
	provider := NewProviderSpaces(providerService, s.logger)
	provider.modelSpaceMap = map[network.Id]network.SpaceInfo{
		network.Id("1"): {
			ID:         "1",
			Name:       "1",
			ProviderId: network.Id("1"),
		},
	}

	warnings, err := provider.deleteSpaces(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string(nil))
}

func (s *spaceSuite) TestDeleteProviderSpacesMatchesAlphaSpace(c *gc.C) {
	defer s.setupMocks(c).Finish()

	providerService := NewProviderService(s.st, s.providerGetter, s.logger)
	provider := NewProviderSpaces(providerService, s.logger)
	provider.modelSpaceMap = map[network.Id]network.SpaceInfo{
		network.Id("1"): {
			ID:   "1",
			Name: "alpha",
		},
	}

	warnings, err := provider.deleteSpaces(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string{
		`Unable to delete space "alpha". Space is used as the default space.`,
	})
}

func (s *spaceSuite) TestDeleteProviderSpacesMatchesDefaultBindingSpace(c *gc.C) {
	c.Skip("The default space is always alpha due to scaffolding in service of Dqlite migration.")

	defer s.setupMocks(c).Finish()

	providerService := NewProviderService(s.st, s.providerGetter, s.logger)
	provider := NewProviderSpaces(providerService, s.logger)
	provider.modelSpaceMap = map[network.Id]network.SpaceInfo{
		network.Id("1"): {
			ID:   "1",
			Name: "1",
		},
	}

	warnings, err := provider.deleteSpaces(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string{
		`Unable to delete space "1". Space is used as the default space.`,
	})
}

func (s *spaceSuite) TestDeleteProviderSpacesContainsConstraintsSpace(c *gc.C) {
	c.Skip("The check on spaces used in constraints before deleting has been removed until constraints are moved to dqlite.")

	defer s.setupMocks(c).Finish()

	providerService := NewProviderService(s.st, s.providerGetter, s.logger)
	provider := NewProviderSpaces(providerService, s.logger)
	provider.modelSpaceMap = map[network.Id]network.SpaceInfo{
		network.Id("1"): {
			ID:   "1",
			Name: "1",
		},
	}

	warnings, err := provider.deleteSpaces(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string{
		`Unable to delete space "1". Space is used in a constraint.`,
	})
}

func (s *spaceSuite) TestProviderSpacesRun(c *gc.C) {
	defer s.setupMocks(c).Finish()

	res := []network.SpaceInfo{
		{
			ID:         "1",
			Name:       "space1",
			ProviderId: network.Id("1"),
		},
	}
	s.st.EXPECT().GetAllSpaces(gomock.Any()).Return(res, nil)

	subnets := network.SubnetInfos{
		{
			CIDR: "10.0.0.1/12",
		},
	}
	spaces := []network.SpaceInfo{
		{ProviderId: network.Id("2"), Subnets: subnets},
	}
	s.logger.EXPECT().Debugf("Adding space %s from provider %s", "empty", "2")
	var receivedSpaceID string
	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), "empty", network.Id("2"), []string{}).
		Do(func(ctx context.Context, uuid string, name string, providerID network.Id, subnetIDs []string) error {
			receivedSpaceID = uuid
			return nil
		}).
		Return(nil)
	s.st.EXPECT().UpsertSubnets(gomock.Any(), subnets)
	s.st.EXPECT().DeleteSpace(gomock.Any(), "1")

	providerService := NewProviderService(s.st, s.providerGetter, s.logger)
	provider := NewProviderSpaces(providerService, s.logger)
	fanConfig, err := network.ParseFanConfig("10.100.0.0/16=253.0.0.0/8")
	c.Assert(err, jc.ErrorIsNil)
	err = provider.saveSpaces(context.Background(), spaces, fanConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.modelSpaceMap, gc.DeepEquals, map[network.Id]network.SpaceInfo{
		network.Id("1"): {
			ID:         "1",
			Name:       "space1",
			ProviderId: network.Id("1"),
		},
		network.Id("2"): {
			ID:         receivedSpaceID,
			Name:       "empty",
			ProviderId: network.Id("2"),
		},
	})

	warnings, err := provider.deleteSpaces(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string(nil))
}
