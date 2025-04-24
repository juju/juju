// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type spaceSuite struct {
	testing.IsolationSuite

	st                                *MockState
	providerWithNetworking            *MockProviderWithNetworking
	providerWithZones                 *MockProviderWithZones
	networkProviderGetter             func(context.Context) (ProviderWithNetworking, error)
	notSupportedNetworkProviderGetter func(context.Context) (ProviderWithNetworking, error)
	zoneProviderGetter                func(context.Context) (ProviderWithZones, error)
	notSupportedZoneProviderGetter    func(context.Context) (ProviderWithZones, error)
}

var _ = gc.Suite(&spaceSuite{})

func (s *spaceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	s.providerWithNetworking = NewMockProviderWithNetworking(ctrl)
	s.networkProviderGetter = func(ctx context.Context) (ProviderWithNetworking, error) {
		return s.providerWithNetworking, nil
	}
	s.notSupportedNetworkProviderGetter = func(ctx context.Context) (ProviderWithNetworking, error) {
		return nil, errors.Errorf("provider %w", coreerrors.NotSupported)
	}

	s.providerWithZones = NewMockProviderWithZones(ctrl)
	s.zoneProviderGetter = func(ctx context.Context) (ProviderWithZones, error) {
		return s.providerWithZones, nil
	}
	s.notSupportedZoneProviderGetter = func(ctx context.Context) (ProviderWithZones, error) {
		return nil, errors.Errorf("provider %w", coreerrors.NotSupported)
	}

	return ctrl
}

func (s *spaceSuite) TestAddSpaceInvalidNameEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Make sure no calls to state are done
	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	_, err := NewService(s.st, loggertesting.WrapCheckLog(c)).AddSpace(
		context.Background(),
		network.SpaceInfo{})
	c.Assert(err, jc.ErrorIs, networkerrors.SpaceNameNotValid)
}

func (s *spaceSuite) TestAddSpaceInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Make sure no calls to state are done
	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	_, err := NewService(s.st, loggertesting.WrapCheckLog(c)).AddSpace(
		context.Background(),
		network.SpaceInfo{
			Name:       "-bad name-",
			ProviderId: "provider-id",
		})
	c.Assert(err, jc.ErrorIs, networkerrors.SpaceNameNotValid)
}

func (s *spaceSuite) TestAddSpaceErrorAdding(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), "0", network.Id("provider-id"), []string{"0"}).
		Return(errors.Errorf("updating subnet %q using space uuid \"space0\"", "0"))

	_, err := NewService(s.st, loggertesting.WrapCheckLog(c)).AddSpace(
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

	returnedUUID, err := NewService(s.st, loggertesting.WrapCheckLog(c)).AddSpace(
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
	err := NewService(s.st, loggertesting.WrapCheckLog(c)).UpdateSpace(context.Background(), network.AlphaSpaceId, newName)
	c.Assert(err, jc.ErrorIsNil)
}

// TestUpdateSpaceNotFound checks that if we try to call Service.UpdateSpace on
// a space that doesn't exist, an error is returned matching
// networkerrors.SpaceNotFound.
func (s *spaceSuite) TestUpdateSpaceNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	spaceID := "unknown-space"
	s.st.EXPECT().UpdateSpace(gomock.Any(), spaceID, "newname").
		Return(errors.Errorf("space %q: %w", spaceID, networkerrors.SpaceNotFound))

	svc := NewService(s.st, loggertesting.WrapCheckLog(c))
	err := svc.UpdateSpace(context.Background(), spaceID, "newname")
	c.Assert(err, jc.ErrorIs, networkerrors.SpaceNotFound)
}

func (s *spaceSuite) TestRetrieveSpaceByID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSpace(gomock.Any(), network.AlphaSpaceId)
	_, err := NewService(s.st, loggertesting.WrapCheckLog(c)).Space(context.Background(), network.AlphaSpaceId)
	c.Assert(err, jc.ErrorIsNil)
}

// TestRetrieveSpaceByIDNotFound checks that if we try to call Service.Space on
// a space that doesn't exist, an error is returned matching
// networkerrors.SpaceNotFound.
func (s *spaceSuite) TestRetrieveSpaceByIDNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSpace(gomock.Any(), "unknown-space").
		Return(nil, errors.Errorf("space %q: %w", "unknown-space", networkerrors.SpaceNotFound))
	_, err := NewService(s.st, loggertesting.WrapCheckLog(c)).Space(context.Background(), "unknown-space")
	c.Assert(err, jc.ErrorIs, networkerrors.SpaceNotFound)
}

func (s *spaceSuite) TestRetrieveSpaceByName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSpaceByName(gomock.Any(), network.AlphaSpaceName)
	_, err := NewService(s.st, loggertesting.WrapCheckLog(c)).SpaceByName(context.Background(), network.AlphaSpaceName)
	c.Assert(err, jc.ErrorIsNil)
}

// TestRetrieveSpaceByNameNotFound checks that if we try to call
// Service.SpaceByName on a space that doesn't exist, an error is returned
// matching networkerrors.SpaceNotFound.
func (s *spaceSuite) TestRetrieveSpaceByNameNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSpaceByName(gomock.Any(), "unknown-space-name").
		Return(nil, errors.Errorf("space with name %q: %w", "unknown-space-name", networkerrors.SpaceNotFound))
	_, err := NewService(s.st, loggertesting.WrapCheckLog(c)).SpaceByName(context.Background(), "unknown-space-name")
	c.Assert(err, jc.ErrorIs, networkerrors.SpaceNotFound)
}

func (s *spaceSuite) TestRetrieveAllSpaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetAllSpaces(gomock.Any())
	_, err := NewService(s.st, loggertesting.WrapCheckLog(c)).GetAllSpaces(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestRemoveSpace(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().DeleteSpace(gomock.Any(), "space0")
	err := NewService(s.st, loggertesting.WrapCheckLog(c)).RemoveSpace(context.Background(), "space0")
	c.Assert(err, jc.ErrorIsNil)
}

// TestRemoveSpaceNotFound checks that if we try to call Service.RemoveSpace on
// a space that doesn't exist, an error is returned matching
// networkerrors.SpaceNotFound.
func (s *spaceSuite) TestRemoveSpaceNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	spaceID := "unknown-space"
	s.st.EXPECT().DeleteSpace(gomock.Any(), spaceID).
		Return(errors.Errorf("space %q: %w", spaceID, networkerrors.SpaceNotFound))

	svc := NewService(s.st, loggertesting.WrapCheckLog(c))
	err := svc.RemoveSpace(context.Background(), spaceID)
	c.Assert(err, jc.ErrorIs, networkerrors.SpaceNotFound)
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

	s.st.EXPECT().UpsertSubnets(
		gomock.Any(),
		gomock.Any()).Do(
		func(cxt context.Context, subnets []network.SubnetInfo) error {
			c.Check(subnets, gc.HasLen, 2)
			c.Check(subnets[0].ProviderId, gc.Equals, twoSubnets[0].ProviderId)
			c.Check(subnets[0].AvailabilityZones, jc.SameContents, twoSubnets[0].AvailabilityZones)
			c.Check(subnets[0].CIDR, gc.Equals, twoSubnets[0].CIDR)
			c.Check(subnets[1].ProviderId, gc.Equals, twoSubnets[1].ProviderId)
			c.Check(subnets[1].AvailabilityZones, jc.SameContents, twoSubnets[1].AvailabilityZones)
			c.Check(subnets[1].CIDR, gc.Equals, twoSubnets[1].CIDR)
			return nil
		})

	err := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c)).saveProviderSubnets(context.Background(), twoSubnets, "")
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

	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Do(
		func(ctx context.Context, subnets []network.SubnetInfo) error {
			c.Check(subnets, gc.HasLen, 2)
			c.Check(subnets[0].ProviderId, gc.Equals, twoSubnets[0].ProviderId)
			c.Check(subnets[0].AvailabilityZones, jc.SameContents, twoSubnets[0].AvailabilityZones)
			c.Check(subnets[0].CIDR, gc.Equals, twoSubnets[0].CIDR)
			c.Check(subnets[1].ProviderId, gc.Equals, twoSubnets[1].ProviderId)
			c.Check(subnets[1].AvailabilityZones, jc.SameContents, twoSubnets[1].AvailabilityZones)
			c.Check(subnets[1].CIDR, gc.Equals, twoSubnets[1].CIDR)
			return nil
		},
	)

	err := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c)).saveProviderSubnets(context.Background(), twoSubnets, "")
	c.Assert(err, jc.ErrorIsNil)

	anotherSubnet := []network.SubnetInfo{
		{
			ProviderId:        "3",
			AvailabilityZones: []string{"1", "2"},
			CIDR:              "10.0.1.1/24",
		},
	}

	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Do(
		func(ctx context.Context, subnets []network.SubnetInfo) error {
			c.Check(subnets, gc.HasLen, 1)
			c.Check(subnets[0].ProviderId, gc.Equals, anotherSubnet[0].ProviderId)
			c.Check(subnets[0].AvailabilityZones, jc.SameContents, anotherSubnet[0].AvailabilityZones)
			c.Check(subnets[0].CIDR, gc.Equals, anotherSubnet[0].CIDR)
			return nil
		},
	)

	err = NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c)).saveProviderSubnets(context.Background(), anotherSubnet, "")
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

	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Do(
		func(ctx context.Context, subnets []network.SubnetInfo) error {
			c.Check(subnets, gc.HasLen, 1)
			c.Check(subnets[0].ProviderId, gc.Equals, oneSubnet[0].ProviderId)
			c.Check(subnets[0].AvailabilityZones, jc.SameContents, oneSubnet[0].AvailabilityZones)
			c.Check(subnets[0].CIDR, gc.Equals, oneSubnet[0].CIDR)
			return nil
		},
	)

	err := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c)).saveProviderSubnets(context.Background(), oneSubnet, "")
	c.Assert(err, jc.ErrorIsNil)

	// We expect the same subnets to be passed to the state methods.
	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Do(
		func(ctx context.Context, subnets []network.SubnetInfo) error {
			c.Check(subnets, gc.HasLen, 1)
			c.Check(subnets[0].ProviderId, gc.Equals, oneSubnet[0].ProviderId)
			c.Check(subnets[0].AvailabilityZones, jc.SameContents, oneSubnet[0].AvailabilityZones)
			c.Check(subnets[0].CIDR, gc.Equals, oneSubnet[0].CIDR)
			return nil
		},
	)
	err = NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c)).saveProviderSubnets(context.Background(), oneSubnet, "")
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
	err := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c)).saveProviderSubnets(context.Background(), oneSubnet, "")
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
	err := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c)).saveProviderSubnets(context.Background(), oneSubnet, "")
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
	err := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c)).saveProviderSubnets(context.Background(), oneSubnet, "")
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
	err := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c)).saveProviderSubnets(context.Background(), oneSubnet, "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestReloadSpacesFromProvider(c *gc.C) {
	defer s.setupMocks(c).Finish()

	twoSpaces := network.SpaceInfos{
		{
			Name:       "name-origin-0",
			ProviderId: "provider-id-0",
			Subnets: network.SubnetInfos{
				{
					CIDR:       "10.0.0.0/24",
					ProviderId: "subnet-0",
					SpaceID:    "bar",
				},
				{
					CIDR:       "10.0.1.0/24",
					ProviderId: "subnet-1",
					SpaceID:    "bar",
				},
			},
		},
		{
			Name:       "name-origin-1",
			ProviderId: "provider-id-1",
			Subnets: network.SubnetInfos{
				{
					CIDR:       "10.1.0.0/24",
					ProviderId: "subnet-2",
					SpaceID:    "foo",
				},
				{
					CIDR:       "10.1.1.0/24",
					ProviderId: "subnet-3",
					SpaceID:    "foo",
				},
			},
		},
	}

	s.providerWithNetworking.EXPECT().SupportsSpaceDiscovery().Return(true, nil)
	s.providerWithNetworking.EXPECT().Spaces(gomock.Any()).Return(twoSpaces, nil)
	s.st.EXPECT().GetAllSpaces(gomock.Any()).Return([]network.SpaceInfo{
		{
			ID:   network.AlphaSpaceId,
			Name: network.AlphaSpaceName,
		},
	}, nil)

	var (
		spUUID0, spUUID1 string
	)
	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), string(twoSpaces[0].Name), twoSpaces[0].ProviderId, []string{}).
		Do(func(ctx context.Context, uuid string, name string, providerID network.Id, subnetIDs []string) error {
			spUUID0 = uuid
			return nil
		})
	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), string(twoSpaces[1].Name), twoSpaces[1].ProviderId, []string{}).
		Do(func(ctx context.Context, uuid string, name string, providerID network.Id, subnetIDs []string) error {
			spUUID1 = uuid
			return nil
		})
	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Do(
		func(ctx context.Context, subnets []network.SubnetInfo) error {
			c.Check(subnets, gc.HasLen, 2)
			c.Check(subnets[0].CIDR, gc.Equals, twoSpaces[0].Subnets[0].CIDR)
			c.Check(subnets[1].CIDR, gc.Equals, twoSpaces[0].Subnets[1].CIDR)
			c.Check(subnets[0].SpaceID, gc.Equals, spUUID0)
			c.Check(subnets[1].SpaceID, gc.Equals, spUUID0)
			return nil
		},
	)
	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Do(
		func(ctx context.Context, subnets []network.SubnetInfo) error {
			c.Check(subnets, gc.HasLen, 2)
			c.Check(subnets[0].CIDR, gc.Equals, twoSpaces[1].Subnets[0].CIDR)
			c.Check(subnets[1].CIDR, gc.Equals, twoSpaces[1].Subnets[1].CIDR)
			c.Check(subnets[0].SpaceID, gc.Equals, spUUID1)
			c.Check(subnets[1].SpaceID, gc.Equals, spUUID1)
			return nil
		},
	)

	err := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c)).
		ReloadSpaces(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestReloadSpacesUsingSubnets(c *gc.C) {
	defer s.setupMocks(c).Finish()

	twoSubnets := []network.SubnetInfo{
		{CIDR: "10.0.0.1/12"},
		{CIDR: "10.12.24.1/24"},
	}

	s.providerWithNetworking.EXPECT().SupportsSpaceDiscovery().Return(false, nil)
	s.providerWithNetworking.EXPECT().Subnets(gomock.Any(), nil).Return(twoSubnets, nil)

	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Do(
		func(ctx context.Context, subnets []network.SubnetInfo) error {
			c.Check(subnets, gc.HasLen, 2)
			c.Check(subnets[0].CIDR, gc.Equals, twoSubnets[0].CIDR)
			c.Check(subnets[1].CIDR, gc.Equals, twoSubnets[1].CIDR)
			return nil
		},
	)

	err := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c)).
		ReloadSpaces(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *spaceSuite) TestReloadSpacesUsingSubnetsFailsOnSave(c *gc.C) {
	defer s.setupMocks(c).Finish()

	twoSubnets := []network.SubnetInfo{
		{CIDR: "10.0.0.1/12"},
		{CIDR: "10.12.24.1/24"},
	}

	s.providerWithNetworking.EXPECT().SupportsSpaceDiscovery().Return(false, nil)
	s.providerWithNetworking.EXPECT().Subnets(gomock.Any(), nil).Return(twoSubnets, nil)

	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Do(
		func(ctx context.Context, subnets []network.SubnetInfo) error {
			c.Check(subnets, gc.HasLen, 2)
			c.Check(subnets[0].CIDR, gc.Equals, twoSubnets[0].CIDR)
			c.Check(subnets[1].CIDR, gc.Equals, twoSubnets[1].CIDR)
			return nil
		},
	).Return(errors.New("boom"))

	err := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c)).
		ReloadSpaces(context.Background())
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *spaceSuite) TestReloadSpacesNotNetworkEnviron(c *gc.C) {
	defer s.setupMocks(c).Finish()

	providerGetterFails := func(ctx context.Context) (ProviderWithNetworking, error) {
		return nil, coreerrors.NotSupported
	}
	err := NewProviderService(s.st, providerGetterFails, s.notSupportedZoneProviderGetter, loggertesting.WrapCheckLog(c)).
		ReloadSpaces(context.Background())

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

	oneSubnet := network.SubnetInfos{
		{
			CIDR:    "10.0.0.1/12",
			SpaceID: "1",
		},
	}
	spaces := []network.SpaceInfo{
		{ProviderId: network.Id("1"), Subnets: oneSubnet},
	}
	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Do(
		func(ctx context.Context, subnets []network.SubnetInfo) error {
			c.Check(subnets, gc.HasLen, 1)
			c.Check(subnets[0].CIDR, gc.Equals, oneSubnet[0].CIDR)
			c.Check(subnets[0].SpaceID, gc.Equals, oneSubnet[0].SpaceID)
			return nil
		},
	)

	providerService := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))
	provider := NewProviderSpaces(providerService, loggertesting.WrapCheckLog(c))
	err := provider.saveSpaces(context.Background(), spaces)
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

	oneSubnet := network.SubnetInfos{
		{
			CIDR: "10.0.0.1/12",
		},
	}
	spaces := []network.SpaceInfo{
		{ProviderId: network.Id("2"), Subnets: oneSubnet},
	}

	var receivedSpaceID string
	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), "empty", network.Id("2"), []string{}).
		Do(func(ctx context.Context, uuid string, name string, providerID network.Id, subnetIDs []string) error {
			receivedSpaceID = uuid
			return nil
		}).
		Return(nil)
	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Do(
		func(ctx context.Context, subnets []network.SubnetInfo) error {
			c.Check(subnets, gc.HasLen, 1)
			c.Check(subnets[0].CIDR, gc.Equals, oneSubnet[0].CIDR)
			return nil
		},
	)

	providerService := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))
	provider := NewProviderSpaces(providerService, loggertesting.WrapCheckLog(c))
	err := provider.saveSpaces(context.Background(), spaces)
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

	providerService := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))
	provider := NewProviderSpaces(providerService, loggertesting.WrapCheckLog(c))
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

	oneSubnet := network.SubnetInfos{
		{
			CIDR: "10.0.0.1/12",
		},
	}
	spaces := []network.SpaceInfo{
		{ProviderId: network.Id("2"), Subnets: oneSubnet},
	}

	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), "empty", network.Id("2"), []string{}).
		Return(nil)
	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Do(
		func(ctx context.Context, subnets []network.SubnetInfo) error {
			c.Check(subnets, gc.HasLen, 1)
			c.Check(subnets[0].CIDR, gc.Equals, oneSubnet[0].CIDR)
			return nil
		},
	)

	providerService := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))
	provider := NewProviderSpaces(providerService, loggertesting.WrapCheckLog(c))
	err := provider.saveSpaces(context.Background(), spaces)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.deltaSpaces(), gc.DeepEquals, network.MakeIDSet(network.Id("1")))
}

func (s *spaceSuite) TestDeleteProviderSpacesWithNoDeltas(c *gc.C) {
	defer s.setupMocks(c).Finish()

	providerService := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))
	provider := NewProviderSpaces(providerService, loggertesting.WrapCheckLog(c))
	warnings, err := provider.deleteSpaces(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(warnings, gc.DeepEquals, []string(nil))
}

func (s *spaceSuite) TestDeleteProviderSpaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().DeleteSpace(gomock.Any(), "1")
	s.st.EXPECT().IsSpaceUsedInConstraints(gomock.Any(), network.SpaceName("1")).Return(false, nil)

	providerService := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))
	provider := NewProviderSpaces(providerService, loggertesting.WrapCheckLog(c))
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

	providerService := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))
	provider := NewProviderSpaces(providerService, loggertesting.WrapCheckLog(c))
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

	providerService := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))
	provider := NewProviderSpaces(providerService, loggertesting.WrapCheckLog(c))
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
	defer s.setupMocks(c).Finish()

	providerService := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))
	provider := NewProviderSpaces(providerService, loggertesting.WrapCheckLog(c))
	provider.modelSpaceMap = map[network.Id]network.SpaceInfo{
		network.Id("1"): {
			ID:   "1",
			Name: "1",
		},
	}
	s.st.EXPECT().IsSpaceUsedInConstraints(gomock.Any(), network.SpaceName("1")).Return(true, nil)

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

	oneSubnet := network.SubnetInfos{
		{
			CIDR: "10.0.0.1/12",
		},
	}
	spaces := []network.SpaceInfo{
		{ProviderId: network.Id("2"), Subnets: oneSubnet},
	}

	var receivedSpaceID string
	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), "empty", network.Id("2"), []string{}).
		Do(func(ctx context.Context, uuid string, name string, providerID network.Id, subnetIDs []string) error {
			receivedSpaceID = uuid
			return nil
		}).
		Return(nil)
	s.st.EXPECT().UpsertSubnets(gomock.Any(), gomock.Any()).Do(
		func(ctx context.Context, subnets []network.SubnetInfo) error {
			c.Check(subnets, gc.HasLen, 1)
			c.Check(subnets[0].CIDR, gc.Equals, oneSubnet[0].CIDR)
			return nil
		},
	)
	s.st.EXPECT().DeleteSpace(gomock.Any(), "1")
	s.st.EXPECT().IsSpaceUsedInConstraints(gomock.Any(), network.SpaceName("space1")).Return(false, nil)

	providerService := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))
	provider := NewProviderSpaces(providerService, loggertesting.WrapCheckLog(c))
	err := provider.saveSpaces(context.Background(), spaces)
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

func (s *spaceSuite) TestSupportsSpaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	providerService := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))

	s.providerWithNetworking.EXPECT().SupportsSpaces().Return(true, nil)

	supported, err := providerService.SupportsSpaces(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(supported, jc.IsTrue)
}

func (s *spaceSuite) TestSupportsSpacesNotSupported(c *gc.C) {
	defer s.setupMocks(c).Finish()

	providerService := NewProviderService(s.st, s.notSupportedNetworkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))

	supported, err := providerService.SupportsSpaces(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(supported, jc.IsFalse)
}

func (s *spaceSuite) TestSupportsSpaceDiscovery(c *gc.C) {
	defer s.setupMocks(c).Finish()

	providerService := NewProviderService(s.st, s.networkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))

	s.providerWithNetworking.EXPECT().SupportsSpaceDiscovery().Return(true, nil)

	supported, err := providerService.SupportsSpaceDiscovery(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(supported, jc.IsTrue)
}

func (s *spaceSuite) TestSupportsSpaceDiscoveryNotSupported(c *gc.C) {
	defer s.setupMocks(c).Finish()

	providerService := NewProviderService(s.st, s.notSupportedNetworkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))

	supported, err := providerService.SupportsSpaceDiscovery(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(supported, jc.IsFalse)
}
