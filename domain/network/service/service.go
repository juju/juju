// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"net"

	"github.com/google/uuid"
	"github.com/juju/collections/set"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/trace"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/errors"
)

// Service provides the API for working with the network domain.
type Service struct {
	st     State
	logger logger.Logger
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}

// ProviderService provides the API for working with network spaces.
type ProviderService struct {
	Service
	providerWithNetworking providertracker.ProviderGetter[ProviderWithNetworking]
	providerWithZones      providertracker.ProviderGetter[ProviderWithZones]
}

// NewProviderService returns a new service reference wrapping the input state.
func NewProviderService(
	st State,
	providerWithNetworking providertracker.ProviderGetter[ProviderWithNetworking],
	providerWithZones providertracker.ProviderGetter[ProviderWithZones],
	logger logger.Logger,
) *ProviderService {
	return &ProviderService{
		Service: Service{
			st:     st,
			logger: logger,
		},
		providerWithNetworking: providerWithNetworking,
		providerWithZones:      providerWithZones,
	}
}

// ReloadSpaces loads spaces and subnets from the provider into state.
func (s *ProviderService) ReloadSpaces(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	networkProvider, err := s.providerWithNetworking(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return errors.Errorf("spaces discovery in a non-networking environ %w", coreerrors.NotSupported)
	}
	if err != nil {
		return errors.Capture(err)
	}

	canDiscoverSpaces, err := networkProvider.SupportsSpaceDiscovery()
	if err != nil {
		return errors.Capture(err)
	}

	if canDiscoverSpaces {
		spaces, err := networkProvider.Spaces(ctx)
		if err != nil {
			return errors.Capture(err)
		}

		s.logger.Infof(ctx, "discovered spaces: %s", spaces.String())

		providerSpaces := newSpaceOperations(s, s.logger)
		if err := providerSpaces.saveSpaces(ctx, spaces); err != nil {
			return errors.Capture(err)
		}
		warnings, err := providerSpaces.deleteSpaces(ctx)
		if err != nil {
			return errors.Capture(err)
		}
		for _, warning := range warnings {
			s.logger.Tracef(ctx, warning)
		}
		return nil
	}

	s.logger.Debugf(ctx, "environ does not support space discovery, falling back to subnet discovery")
	subnets, err := networkProvider.Subnets(ctx, nil)
	if err != nil {
		return errors.Capture(err)
	}
	// TODO(nvinuesa): Here, the alpha space is scaffolding, it should be
	// replaced with the model's default space.
	return errors.Capture(s.saveProviderSubnets(ctx, subnets, network.AlphaSpaceId))
}

// SaveProviderSubnets loads subnets into state.
// Currently it does not delete removed subnets.
func (s *ProviderService) saveProviderSubnets(
	ctx context.Context,
	subnets []network.SubnetInfo,
	spaceUUID network.SpaceUUID,
) error {
	var subnetsToUpsert []network.SubnetInfo
	for _, subnet := range subnets {
		ip, _, err := net.ParseCIDR(subnet.CIDR)
		if err != nil {
			return errors.Capture(err)
		}
		if ip.IsInterfaceLocalMulticast() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
			continue
		}

		// Add the subnet with the provided space UUID to the upsert list.
		subnetToUpsert := subnet
		subnetToUpsert.SpaceID = spaceUUID
		subnetsToUpsert = append(subnetsToUpsert, subnetToUpsert)
	}

	if len(subnetsToUpsert) > 0 {
		return errors.Capture(s.upsertProviderSubnets(ctx, subnetsToUpsert))
	}

	return nil
}

// upsertProviderSubnets shims the state method for upserting subnets, and also
// makes sure a uuid is inserted by checking if one was provided otherwise
// create a new UUID v7.
func (s *ProviderService) upsertProviderSubnets(ctx context.Context, subnetsToUpsert network.SubnetInfos) error {
	for i, sn := range subnetsToUpsert {
		if sn.ID.String() == "" {
			uuid, err := uuid.NewV7()
			if err != nil {
				return errors.Capture(err)
			}
			subnetsToUpsert[i].ID = network.Id(uuid.String())
		}

	}
	if err := s.st.UpsertSubnets(ctx, subnetsToUpsert); err != nil && !errors.Is(err, coreerrors.AlreadyExists) {
		return errors.Capture(err)
	}
	return nil
}

// SupportsSpaces returns whether the provider supports spaces.
func (s *ProviderService) SupportsSpaces(ctx context.Context) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	networkProvider, err := s.providerWithNetworking(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return networkProvider.SupportsSpaces()
}

// SupportsSpaceDiscovery returns whether the provider supports discovering
// spaces from the provider.
func (s *ProviderService) SupportsSpaceDiscovery(ctx context.Context) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	networkProvider, err := s.providerWithNetworking(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return networkProvider.SupportsSpaceDiscovery()
}

// AllocateContainerAddresses allocates a static address for each of the
// container NICs in preparedInfo, hosted by the hostInstanceID, if the
// provider supports it. Returns the network config including all allocated
// addresses on success.
// Returns [networkerrors.ContainerAddressesNotSupported] if the provider
// does not support container addressing.
func (s *ProviderService) AllocateContainerAddresses(ctx context.Context,
	hostInstanceID instance.Id,
	containerName string,
	preparedInfo network.InterfaceInfos,
) (network.InterfaceInfos, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, err := s.providerWithNetworking(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	if !provider.SupportsContainerAddresses() {
		return nil, networkerrors.ContainerAddressesNotSupported
	}

	newInfo, err := provider.AllocateContainerAddresses(ctx, hostInstanceID, containerName, preparedInfo)
	return newInfo, errors.Capture(err)
}

// DevicesForGuest returns the network devices that should be configured in the
// guest machine with the input UUID, based on the host machine's bridges.
func (s *ProviderService) DevicesForGuest(
	ctx context.Context, hostUUID, guestUUID machine.UUID,
) ([]domainnetwork.NetInterface, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	nodeUUID, spaceUUIDs, nics, err := s.spacesAndDevicesForMachine(ctx, guestUUID, hostUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	guestDevices, err := s.guestDevices(ctx, hostUUID, nodeUUID, spaceUUIDs, nics)
	return guestDevices, errors.Capture(err)
}

func (s *ProviderService) guestDevices(
	ctx context.Context,
	mUUID machine.UUID,
	nodeUUID string,
	spaceUUIDs []string,
	nics map[string][]domainnetwork.NetInterface,
) ([]domainnetwork.NetInterface, error) {
	var (
		guestDevices []domainnetwork.NetInterface
		deviceIndex  int
	)
	spacesToSatisfy := set.NewStrings(spaceUUIDs...)

	networkingProvider, err := s.providerWithNetworking(ctx)
	if err != nil {
		return nil, errors.Errorf("retrieving networking provider: %w", err)
	}

	// In most cases, the container will rely on DHCP assigned addresses.
	// If the provider supports allocating addresses to containers,
	// each device's address will be obtained downstream, and we indicate
	// that said address is configured statically.
	configMethod := network.ConfigDHCP
	if networkingProvider.SupportsContainerAddresses() {
		configMethod = network.ConfigStatic
	}

	for spaceUUID, spaceNics := range nics {
		if !spacesToSatisfy.Contains(spaceUUID) {
			continue
		}

		s.logger.Debugf(ctx, "looking for bridges in space %q", spaceUUID)

		var bridgeToUse *domainnetwork.NetInterface
		for _, nic := range spaceNics {
			if nic.Type == network.BridgeDevice || nic.VirtualPortType == network.OvsPort {
				bridgeToUse = &nic
				break
			}
		}

		if bridgeToUse == nil {
			return nil, errors.Errorf(
				"no bridge found in space %q for machine %q", spaceUUID, mUUID,
			).Add(networkerrors.SpaceRequirementsUnsatisfiable)
		}

		s.logger.Debugf(ctx, "found bridge %q in space %q for machine %q", bridgeToUse.Name, spaceUUID, mUUID)

		newDev := domainnetwork.NetInterface{
			Name: fmt.Sprintf("eth%d", deviceIndex),
			// When using the Fan, we used to locate the VXLAN device
			// associated with the bridge and use that MTU.
			// We no longer support Fan networking, but this is worth being
			// aware of in situations where the MTU set turns out to be
			// incompatible with the bridged network.
			MTU:              bridgeToUse.MTU,
			Type:             network.EthernetDevice,
			ParentDeviceName: bridgeToUse.Name,
			VirtualPortType:  bridgeToUse.VirtualPortType,
			IsEnabled:        true,
			IsAutoStart:      true,
		}

		mac := network.GenerateVirtualMACAddress()
		newDev.MACAddress = &mac

		cidr, err := s.st.GetSubnetCIDRForDevice(ctx, nodeUUID, bridgeToUse.Name, spaceUUID)
		if err != nil {
			return nil, errors.Errorf(
				"retrieving CIDR for device %q in space %q on machine %q: %w", bridgeToUse.Name, spaceUUID, mUUID, err)
		}

		newDev.Addrs = []domainnetwork.NetAddr{{
			AddressValue: cidr,
			ConfigType:   configMethod,
		}}

		deviceIndex++
		guestDevices = append(guestDevices, newDev)
	}

	return guestDevices, nil
}

// spaceOperations defines a set of operations to perform when dealing with
// provider spaces. SaveSpaces, DeleteSpaces are operations for setting state
// in the persistence layer.
type spaceOperations struct {
	modelSpaceMap map[network.Id]network.SpaceInfo
	updatedSpaces network.IDSet
	spaceService  *ProviderService
	logger        logger.Logger
}

func newSpaceOperations(spaceService *ProviderService, logger logger.Logger) *spaceOperations {
	return &spaceOperations{
		spaceService: spaceService,
		logger:       logger,

		modelSpaceMap: make(map[network.Id]network.SpaceInfo),
		updatedSpaces: network.MakeIDSet(),
	}
}

// saveSpaces consumes provider spaces and saves the spaces as subnets on a
// provider.
func (s *spaceOperations) saveSpaces(ctx context.Context, providerSpaces []network.SpaceInfo) error {
	stateSpaces, err := s.spaceService.GetAllSpaces(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	spaceNames := set.NewStrings()
	for _, space := range stateSpaces {
		s.modelSpaceMap[space.ProviderId] = space
		spaceNames.Add(string(space.Name))
	}

	for _, spaceInfo := range providerSpaces {
		// Check if the space is already in state,
		// in which case we know its name.
		var spaceID network.SpaceUUID
		stateSpace, ok := s.modelSpaceMap[spaceInfo.ProviderId]
		if ok {
			spaceID = stateSpace.ID
		} else {
			// The space is new, we need to create a valid name for it in state.
			// Convert the name into a valid name that is not already in use.
			spaceName := network.ConvertSpaceName(string(spaceInfo.Name), spaceNames)

			s.logger.Debugf(ctx, "Adding space %s from providerWithNetworking %s", spaceName, string(spaceInfo.ProviderId))
			spaceUUID, err := s.spaceService.AddSpace(
				ctx,
				network.SpaceInfo{
					Name:       network.SpaceName(spaceName),
					ProviderId: spaceInfo.ProviderId,
				},
			)
			if err != nil {
				return errors.Capture(err)
			}

			spaceNames.Add(spaceName)

			// To ensure that we can remove spaces, we back-fill the new spaces
			// onto the modelSpaceMap.
			s.modelSpaceMap[spaceInfo.ProviderId] = network.SpaceInfo{
				ID:         spaceUUID,
				Name:       network.SpaceName(spaceName),
				ProviderId: spaceInfo.ProviderId,
			}
			spaceID = spaceUUID
		}

		err = s.spaceService.saveProviderSubnets(ctx, spaceInfo.Subnets, spaceID)
		if err != nil {
			return errors.Capture(err)
		}

		s.updatedSpaces.Add(spaceInfo.ProviderId)
	}

	return nil
}

// deltaSpaces returns all the spaces that haven't been updated.
func (s *spaceOperations) deltaSpaces() network.IDSet {
	// Workout the difference between all the current spaces vs what was
	// actually changed.
	allStateSpaces := network.MakeIDSet()
	for providerID := range s.modelSpaceMap {
		allStateSpaces.Add(providerID)
	}

	return allStateSpaces.Difference(s.updatedSpaces)
}

// deleteSpaces will attempt to delete any unused spaces after a SaveSpaces has
// been called.
// If there are no spaces to be deleted, it will exit out early.
func (s *spaceOperations) deleteSpaces(ctx context.Context) ([]string, error) {
	// Exit early if there is nothing to do.
	if len(s.modelSpaceMap) == 0 {
		return nil, nil
	}

	// Then check if the delta spaces are empty, if it's also empty, exit again.
	// We do it after modelSpaceMap as we create a types to create this, which
	// seems pretty wasteful.
	remnantSpaces := s.deltaSpaces()
	if len(remnantSpaces) == 0 {
		return nil, nil
	}

	// TODO (manadart 2024-01-29): The alpha space ID here is scaffolding and
	// should be replaced with the configured model default space upon
	// migrating this logic to Dqlite.
	defaultEndpointBinding := network.AlphaSpaceId

	var warnings []string
	for _, providerID := range remnantSpaces.SortedValues() {
		// If the space is not in state or the name is not in space names, then
		// we can ignore it.
		space, ok := s.modelSpaceMap[providerID]
		if !ok {
			// No warning here, the space was just not found.
			continue
		} else if space.Name == network.AlphaSpaceName ||
			space.ID == defaultEndpointBinding {

			warning := fmt.Sprintf("Unable to delete space %q. Space is used as the default space.", space.Name)
			warnings = append(warnings, warning)
			continue
		}

		violations, err := s.spaceService.st.RemoveSpace(ctx, space.Name, false, false)
		if err != nil {
			return warnings, errors.Errorf("removing space %q: %w", space.Name, err)
		}
		if !violations.IsEmpty() {
			warning := fmt.Sprintf("Unable to delete space %q: %s", space.Name, violations)
			warnings = append(warnings, warning)
		}
	}

	return warnings, nil
}
