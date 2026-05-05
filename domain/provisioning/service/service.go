// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/juju/collections/set"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain/provisioning"
	provisioningerrors "github.com/juju/juju/domain/provisioning/errors"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/errors"
)

// ControllerUUIDKey is the controller config key for the controller UUID.
const ControllerUUIDKey = "controller-uuid"

// ModelState provides direct database access to the model database for
// provisioning info retrieval. All methods execute within a single
// transaction.
type ModelState interface {
	// GetProvisioningInfo retrieves all provisioning data for a machine
	// in a single transaction from the model database.
	GetProvisioningInfo(ctx context.Context, machineName string, isControllerModel bool) (provisioning.ProvisioningInfoState, error)
}

// ControllerState provides direct database access to the controller
// database for provisioning info retrieval.
type ControllerState interface {
	// GetControllerConfig retrieves controller configuration from the
	// controller database.
	GetControllerConfig(ctx context.Context) (map[string]any, error)
}

// ImageMetadataFetcher fetches image metadata from external sources
// (simplestreams) when cached metadata is unavailable. It also caches
// fetched metadata for future lookups.
type ImageMetadataFetcher interface {
	// FetchImageMetadata fetches image metadata from external data
	// sources for the given image constraint. It returns the metadata
	// found or an error if none could be located.
	FetchImageMetadata(ctx context.Context, constraint provisioning.ImageConstraint) ([]provisioning.CloudImageMetadata, error)
}

// Service provides access to provisioning info aggregation.
type Service struct {
	modelSt              ModelState
	controllerSt         ControllerState
	imageMetadataFetcher ImageMetadataFetcher
	modelUUID            model.UUID
	logger               logger.Logger
}

// NewService returns a new provisioning service.
func NewService(
	modelSt ModelState,
	controllerSt ControllerState,
	imageMetadataFetcher ImageMetadataFetcher,
	modelUUID model.UUID,
	logger logger.Logger,
) *Service {
	return &Service{
		modelSt:              modelSt,
		controllerSt:         controllerSt,
		imageMetadataFetcher: imageMetadataFetcher,
		modelUUID:            modelUUID,
		logger:               logger,
	}
}

// GetProvisioningInfo returns the complete provisioning information for a
// machine, consolidating all data from the model and controller databases
// into a single call.
//
// The following errors may be returned:
//   - [provisioningerrors.MachineNotFound] if the machine does not exist.
func (s *Service) GetProvisioningInfo(
	ctx context.Context,
	machineName coremachine.Name,
	isControllerModel bool,
) (provisioning.ProvisioningInfo, error) {
	if err := machineName.Validate(); err != nil {
		return provisioning.ProvisioningInfo{}, errors.Errorf(
			"validating machine name %q: %w", machineName, err,
		)
	}

	// Step 1: Fetch all model-DB data in a single transaction.
	stateInfo, err := s.modelSt.GetProvisioningInfo(ctx, machineName.String(), isControllerModel)
	if err != nil {
		return provisioning.ProvisioningInfo{}, errors.Errorf(
			"getting provisioning info for machine %q: %w", machineName, err,
		)
	}

	// Step 2: Fetch controller config (separate DB/transaction).
	controllerConfig, err := s.controllerSt.GetControllerConfig(ctx)
	if err != nil {
		return provisioning.ProvisioningInfo{}, errors.Errorf(
			"getting controller config: %w", err,
		)
	}

	// Extract controller UUID from the config.
	controllerUUID, _ := controllerConfig[ControllerUUIDKey].(string)

	// Step 3: Resolve endpoint bindings to space provider IDs/names.
	endpointBindings, boundSpaceNames := s.resolveEndpointBindings(stateInfo.EndpointBindings, stateInfo.Spaces)

	// Step 4: Validate space constraints against bindings.
	machineSpaces, err := s.machineSpaces(stateInfo.Constraints, boundSpaceNames)
	if err != nil {
		return provisioning.ProvisioningInfo{}, errors.Capture(err)
	}

	// Step 5: Construct network topology.
	spaceSubnets, subnetAZs := s.buildNetworkTopology(ctx, machineName.String(), stateInfo.Constraints, machineSpaces, stateInfo.Spaces, stateInfo.CloudType)

	// Step 6: Resolve image metadata (cached or fallback to external).
	imageMetadata, err := s.resolveImageMetadata(ctx, stateInfo)
	if err != nil {
		return provisioning.ProvisioningInfo{}, errors.Errorf(
			"resolving image metadata: %w", err,
		)
	}

	// Step 7: Compute instance tags.
	machineTags := s.computeTags(stateInfo.UnitNames, machineName, stateInfo.IsController, stateInfo.ResourceTags, stateInfo.ResourceTagsFound, controllerUUID, stateInfo.ModelName)

	// Step 8: Determine machine jobs.
	jobs := s.computeJobs(isControllerModel, stateInfo.IsController)

	// Step 9: Build volume params.
	volumes, volumeAttachments := s.buildVolumeParams(machineName, stateInfo)

	// Step 10: Build root disk params.
	rootDisk := s.buildRootDisk(stateInfo.RootDiskStoragePool)

	return provisioning.ProvisioningInfo{
		MachineUUID:        stateInfo.MachineUUID,
		Base:               stateInfo.Base,
		PlacementDirective: stateInfo.PlacementDirective,
		Constraints:        stateInfo.Constraints,
		Jobs:               jobs,
		EndpointBindings:   endpointBindings,
		Volumes:            volumes,
		VolumeAttachments:  volumeAttachments,
		RootDisk:           rootDisk,
		ImageMetadata:      imageMetadata,
		Tags:               machineTags,
		SpaceSubnets:       spaceSubnets,
		SubnetAZs:          subnetAZs,
		CloudInitUserData:  stateInfo.CloudInitUserData,
		ControllerConfig:   controllerConfig,
	}, nil
}

// resolveEndpointBindings translates endpoint bindings (with space UUIDs)
// into provider-visible values (space provider IDs or space names).
func (s *Service) resolveEndpointBindings(
	endpointBindings map[string]map[string]network.SpaceUUID,
	spaces network.SpaceInfos,
) (map[string]string, []network.SpaceName) {
	combinedBindings := make(map[string]string)
	var boundSpaceNames []network.SpaceName

	for _, bindings := range endpointBindings {
		for endpoint, spaceID := range bindings {
			space := spaces.GetByID(spaceID)
			if space == nil {
				continue
			}
			boundSpaceNames = append(boundSpaceNames, space.Name)
			bound := string(space.ProviderId)
			if bound == "" {
				bound = space.Name.String()
			}
			combinedBindings[endpoint] = bound
		}
	}
	return combinedBindings, boundSpaceNames
}

// machineSpaces returns the list of spaces that the machine must be in.
func (s *Service) machineSpaces(
	cons constraints.Value,
	boundSpaceNames []network.SpaceName,
) ([]network.SpaceName, error) {
	includeSpaces := set.NewStrings(cons.IncludeSpaces()...)
	excludeSpaces := set.NewStrings(cons.ExcludeSpaces()...)

	for _, spaceName := range boundSpaceNames {
		if excludeSpaces.Contains(spaceName.String()) {
			return nil, errors.Errorf(
				"machine is bound to space %q which conflicts with negative space constraint",
				spaceName)
		}
		includeSpaces.Add(spaceName.String())
	}

	sorted := includeSpaces.SortedValues()
	result := make([]network.SpaceName, len(sorted))
	for i, s := range sorted {
		result[i] = network.SpaceName(s)
	}
	return result, nil
}

// buildNetworkTopology constructs the space-subnet-AZ topology needed
// by the provider for provisioning.
func (s *Service) buildNetworkTopology(
	ctx context.Context,
	machineID string,
	cons constraints.Value,
	spaceNames []network.SpaceName,
	allSpaces network.SpaceInfos,
	cloudType string,
) (map[string][]string, map[string][]string) {
	// If there are no space names, or the only space is alpha and it
	// wasn't explicitly constrained, return empty topology.
	consHasOnlyAlpha := len(cons.IncludeSpaces()) == 1 && cons.IncludeSpaces()[0] == network.AlphaSpaceName.String()
	if len(spaceNames) < 1 ||
		((len(spaceNames) == 1 && spaceNames[0] == network.AlphaSpaceName) && !consHasOnlyAlpha) {
		return nil, nil
	}

	subnetAZs := make(map[string][]string)
	spaceSubnets := make(map[string][]string)

	for _, spaceName := range spaceNames {
		space := allSpaces.GetByName(spaceName)
		if space == nil {
			s.logger.Warningf(ctx, "space %q not found in model spaces", spaceName)
			continue
		}

		subnets := space.Subnets
		if len(subnets) == 0 {
			s.logger.Warningf(ctx, "cannot use space %q as deployment target: no subnets", spaceName)
			continue
		}

		subnetIDs := make([]string, 0, len(subnets))
		for _, subnet := range subnets {
			providerID := subnet.ProviderId
			if providerID == "" {
				s.logger.Warningf(ctx, "not using subnet %q in space %q for machine %q provisioning: no ProviderId set",
					subnet.CIDR, spaceName, machineID)
				continue
			}

			zones := subnet.AvailabilityZones
			if len(zones) == 0 {
				if cloudType != "azure" && cloudType != "openstack" {
					s.logger.Warningf(ctx, "not using subnet %q in space %q for machine %q provisioning: no availability zone(s) set",
						subnet.CIDR, spaceName, machineID)
					continue
				}
			}

			subnetAZs[string(providerID)] = zones
			subnetIDs = append(subnetIDs, string(providerID))
		}
		spaceSubnets[spaceName.String()] = subnetIDs
	}

	return spaceSubnets, subnetAZs
}

// resolveImageMetadata returns image metadata from cache or falls back
// to the external fetcher.
func (s *Service) resolveImageMetadata(
	ctx context.Context,
	stateInfo provisioning.ProvisioningInfoState,
) ([]provisioning.CloudImageMetadata, error) {
	if len(stateInfo.CachedImageMetadata) > 0 {
		// Sort by priority.
		metadata := slices.Clone(stateInfo.CachedImageMetadata)
		sort.Slice(metadata, func(i, j int) bool {
			return metadata[i].Priority < metadata[j].Priority
		})
		return metadata, nil
	}

	// Build the image constraint for external lookup.
	base, err := corebase.ParseBase(stateInfo.Base.OS, stateInfo.Base.Channel.String())
	if err != nil {
		return nil, errors.Capture(err)
	}

	var arches []string
	if stateInfo.Constraints.HasArch() {
		arches = []string{*stateInfo.Constraints.Arch}
	}

	constraint := provisioning.ImageConstraint{
		Releases: []string{base.Channel.Track},
		Arches:   arches,
		Stream:   stateInfo.ImageStream,
		Region:   stateInfo.CloudRegion,
		Endpoint: stateInfo.CloudEndpoint,
	}
	if stateInfo.Constraints.ImageID != nil {
		constraint.ImageID = stateInfo.Constraints.ImageID
	}

	metadata, err := s.imageMetadataFetcher.FetchImageMetadata(ctx, constraint)
	if err != nil {
		return nil, errors.Errorf("fetching image metadata from external sources: %w", err)
	}

	if len(metadata) == 0 {
		return nil, errors.Errorf(
			"image metadata for version %v, arch %v: %w",
			constraint.Releases, constraint.Arches,
			provisioningerrors.ImageMetadataNotFound,
		)
	}

	sort.Slice(metadata, func(i, j int) bool {
		return metadata[i].Priority < metadata[j].Priority
	})
	return metadata, nil
}

// computeTags returns the instance tags for the machine.
func (s *Service) computeTags(
	unitNames []coreunit.NameWithPrincipal,
	machineName coremachine.Name,
	isController bool,
	resourceTagsMap map[string]string,
	resourceTagsFound bool,
	controllerUUID string,
	modelName string,
) map[string]string {
	var resourceTagger tags.ResourceTagger
	if resourceTagsFound {
		resourceTagger = resourceTagsWrapper{tags: resourceTagsMap}
	} else {
		resourceTagger = resourceTagsWrapper{}
	}

	machineTags := instancecfg.InstanceTags(string(s.modelUUID), controllerUUID, resourceTagger, isController)

	// Compute principal unit names for the tag.
	principalUnitNames := make([]string, 0, len(unitNames))
	for _, unitName := range unitNames {
		principalUnit := unitName.Name
		if unitName.IsSubordinate() {
			principalUnit = *unitName.Principal
		}
		principalUnitNames = append(principalUnitNames, principalUnit.String())
	}
	slices.Sort(principalUnitNames)
	principalUnitNames = slices.Compact(principalUnitNames)

	if len(unitNames) > 0 {
		machineTags[tags.JujuUnitsDeployed] = strings.Join(principalUnitNames, " ")
	}

	machineID := fmt.Sprintf("%s-%s", modelName, "machine-"+machineName.String())
	machineTags[tags.JujuMachine] = machineID

	return machineTags
}

// computeJobs determines the jobs for the machine.
func (s *Service) computeJobs(isControllerModel, isController bool) []model.MachineJob {
	jobs := []model.MachineJob{model.JobHostUnits}
	if isControllerModel && isController {
		jobs = append(jobs, model.JobManageModel)
	}
	return jobs
}

// buildVolumeParams converts the state-level volume data into the
// final volume params.
func (s *Service) buildVolumeParams(
	machineName coremachine.Name,
	stateInfo provisioning.ProvisioningInfoState,
) ([]provisioning.VolumeParams, []provisioning.VolumeAttachmentParams) {
	capturedVolumes := make(map[string]provisioning.VolumeParams, len(stateInfo.VolumeParams))

	for _, vp := range stateInfo.VolumeParams {
		attr := make(map[string]any, len(vp.Attributes))
		for k, v := range vp.Attributes {
			attr[k] = v
		}
		capturedVolumes[vp.UUID] = provisioning.VolumeParams{
			VolumeID:   vp.ID,
			Provider:   vp.Provider,
			SizeMiB:    vp.RequestedSizeMiB,
			Attributes: attr,
			Tags:       vp.Tags,
		}
	}

	var retVAParams []provisioning.VolumeAttachmentParams
	for _, ap := range stateInfo.VolumeAttachmentParams {
		attachParams := provisioning.VolumeAttachmentParams{
			MachineID:  machineName.String(),
			Provider:   ap.Provider,
			ReadOnly:   ap.ReadOnly,
			ProviderID: ap.VolumeProviderID,
			VolumeID:   ap.VolumeID,
		}

		if volParam, exists := capturedVolumes[ap.VolumeUUID]; exists {
			volParam.Attachment = &attachParams
			capturedVolumes[ap.VolumeUUID] = volParam
		} else {
			retVAParams = append(retVAParams, attachParams)
		}
	}

	volumes := make([]provisioning.VolumeParams, 0, len(capturedVolumes))
	for _, v := range capturedVolumes {
		volumes = append(volumes, v)
	}

	return volumes, retVAParams
}

// buildRootDisk converts the storage pool into root disk params.
func (s *Service) buildRootDisk(pool *provisioning.StoragePool) *provisioning.VolumeParams {
	if pool == nil {
		return nil
	}

	result := &provisioning.VolumeParams{
		Provider: pool.Provider,
	}

	if len(pool.Attrs) > 0 {
		result.Attributes = make(map[string]any, len(pool.Attrs))
		for k, v := range pool.Attrs {
			result.Attributes[k] = v
		}
	}

	return result
}

// resourceTagsWrapper implements tags.ResourceTagger.
type resourceTagsWrapper struct {
	tags map[string]string
}

func (r resourceTagsWrapper) ResourceTags() (map[string]string, bool) {
	if r.tags == nil {
		return nil, false
	}
	return r.tags, true
}
