// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/juju/collections/set"
	"gopkg.in/yaml.v3"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/logging"
	loggingerrors "github.com/juju/juju/domain/logging/errors"
	"github.com/juju/juju/domain/provisioner"
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
	// GetPreludeProvisioningInfo retrieves model-wide provisioning data
	// that is the same for all machines. This should be called once
	// per batch request.
	GetPreludeProvisioningInfo(ctx context.Context) (provisioner.SharedProvisioningInfoState, error)

	// GetMachineProvisioningInfo retrieves per-machine provisioning data
	// in a single transaction from the model database.
	GetMachineProvisioningInfo(ctx context.Context, machineName string, isControllerModel bool) (provisioner.ProvisioningInfoState, error)
}

// ControllerState provides direct database access to the controller
// database for provisioning info retrieval.
type ControllerState interface {
	// GetCloudEndpoint retrieves the cloud endpoint for a given cloud name
	// and region. If the region has a specific endpoint it is returned,
	// otherwise the cloud-level endpoint is returned.
	GetCloudEndpoint(ctx context.Context, cloudName, regionName string) (string, error)

	// GetCachedImageMetadata retrieves cached image metadata from the
	// controller database matching the given version, architecture, region,
	// stream, and image ID. Empty string parameters are treated as wildcards.
	GetCachedImageMetadata(ctx context.Context, version, arch, region, stream, imageID string) ([]provisioner.CloudImageMetadata, error)

	// GetLokiConfig returns the configured Loki push API endpoint, CA
	// certificate, and insecure skip verify setting. If no endpoint is
	// configured, an error satisfying
	// [github.com/juju/juju/domain/logging/errors.LokiConfigNotFound] is
	// returned.
	GetLokiConfig(ctx context.Context) (logging.LokiConfig, error)
}

// ImageMetadataFetcher fetches image metadata from external sources
// (simplestreams) when cached metadata is unavailable. It also caches
// fetched metadata for future lookups.
type ImageMetadataFetcher interface {
	// FetchImageMetadata fetches image metadata from external data
	// sources for the given image constraint. It returns the metadata
	// found or an error if none could be located.
	FetchImageMetadata(ctx context.Context, constraint provisioner.ImageConstraint) ([]provisioner.CloudImageMetadata, error)
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

// GetPreludeProvisioningInfo retrieves model-wide provisioning data that is
// the same for all machines. This should be called once per batch request,
// then passed into each per-machine GetProvisioningInfo call.
func (s *Service) GetPreludeProvisioningInfo(ctx context.Context) (provisioner.SharedProvisioningInfo, error) {
	// Step 1: Fetch shared model-DB data.
	sharedState, err := s.modelSt.GetPreludeProvisioningInfo(ctx)
	if err != nil {
		return provisioner.SharedProvisioningInfo{}, errors.Errorf(
			"getting shared provisioning info: %w", err,
		)
	}

	// Step 2: Fetch cloud endpoint from controller DB.
	cloudEndpoint, err := s.controllerSt.GetCloudEndpoint(ctx, sharedState.CloudName, sharedState.CloudRegion)
	if err != nil {
		return provisioner.SharedProvisioningInfo{}, errors.Errorf(
			"getting cloud endpoint: %w", err,
		)
	}

	// Step 3: Fetch the controller-wide Loki config. When Loki is not
	// active the config is empty and agents start in logsink mode.
	lokiConfig, err := s.controllerSt.GetLokiConfig(ctx)
	if err != nil && !errors.Is(err, loggingerrors.LokiConfigNotFound) {
		return provisioner.SharedProvisioningInfo{}, errors.Errorf(
			"getting controller loki config: %w", err,
		)
	}

	return provisioner.SharedProvisioningInfo{
		Spaces:                 sharedState.Spaces,
		ModelName:              sharedState.ModelName,
		CloudInitUserData:      sharedState.CloudInitUserData,
		ImageStream:            sharedState.ImageStream,
		ResourceTags:           sharedState.ResourceTags,
		CloudType:              sharedState.CloudType,
		CloudRegion:            sharedState.CloudRegion,
		CloudName:              sharedState.CloudName,
		CloudEndpoint:          cloudEndpoint,
		LokiEndpoint:           lokiConfig.Endpoint,
		LokiCACert:             lokiConfig.CACertificate,
		LokiInsecureSkipVerify: lokiConfig.InsecureSkipVerify,
	}, nil
}

// GetProvisioningInfo returns the complete provisioning information for a
// machine, consolidating all data from the model and controller databases into
// a single call. The shared parameter holds model-wide data fetched once per
// batch; pass it from GetPreludeProvisioningInfo.
//
// The following errors may be returned:
//   - [github.com/juju/juju/domain/machine/errors.MachineNotFound] if the
//     machine does not exist.
func (s *Service) GetProvisioningInfo(
	ctx context.Context,
	machineName coremachine.Name,
	isControllerModel bool,
	shared provisioner.SharedProvisioningInfo,
) (provisioner.ProvisioningInfo, error) {
	if err := machineName.Validate(); err != nil {
		return provisioner.ProvisioningInfo{}, errors.Errorf(
			"validating machine name %q: %w", machineName, err,
		)
	}

	// Step 1: Fetch per-machine model-DB data in a single transaction.
	stateInfo, err := s.modelSt.GetMachineProvisioningInfo(ctx, machineName.String(), isControllerModel)
	if err != nil {
		return provisioner.ProvisioningInfo{}, errors.Errorf(
			"getting provisioning info for machine %q: %w", machineName, err,
		)
	}

	// Extract controller UUID from the config.
	controllerUUID := shared.ControllerConfig.ControllerUUID()

	// Step 2: Fetch cached image metadata from controller DB.
	var version string
	if stateInfo.Base.Channel.Track != "" {
		version = stateInfo.Base.Channel.Track
	}
	var arch string
	if stateInfo.Constraints.HasArch() {
		arch = *stateInfo.Constraints.Arch
	}
	var imageID string
	if stateInfo.Constraints.ImageID != nil {
		imageID = *stateInfo.Constraints.ImageID
	}
	stream := imageStream(shared.ImageStream)
	cachedImageMetadata, err := s.controllerSt.GetCachedImageMetadata(ctx, version, arch, shared.CloudRegion, stream, imageID)
	if err != nil {
		// Log and continue — fall through to external datasources if cache
		// lookup fails (matches original graceful-degradation behaviour).
		s.logger.Infof(ctx, "could not get image metadata from controller: %v", err)
	}

	// Step 3: Resolve endpoint bindings to space provider IDs/names.
	endpointBindings, boundSpaceNames := s.resolveEndpointBindings(stateInfo.EndpointBindings, shared.Spaces)

	// Step 4: Validate space constraints against bindings.
	machineSpaces, err := s.machineSpaces(stateInfo.Constraints, boundSpaceNames)
	if err != nil {
		return provisioner.ProvisioningInfo{}, errors.Capture(err)
	}

	// Step 5: Construct network topology.
	spaceSubnets, subnetAZs, err := s.buildNetworkTopology(
		ctx,
		machineName.String(),
		stateInfo.Constraints,
		machineSpaces,
		shared.Spaces,
		shared.CloudType,
	)
	if err != nil {
		return provisioner.ProvisioningInfo{}, errors.Errorf(
			"building network topology for machine %q: %w", machineName, err,
		)
	}

	// Step 6: Resolve image metadata (cached or fallback to external).
	imageMetadata, err := s.resolveImageMetadata(ctx, stateInfo, shared, cachedImageMetadata)
	if err != nil {
		return provisioner.ProvisioningInfo{}, errors.Errorf(
			"resolving image metadata: %w", err,
		)
	}

	// Step 7: Compute instance tags.
	resourceTags, resourceTagsFound := parseResourceTags(shared.ResourceTags)
	machineTags := s.computeTags(
		stateInfo.UnitNames,
		machineName,
		stateInfo.IsController,
		resourceTags,
		resourceTagsFound,
		controllerUUID,
		shared.ModelName,
	)

	// Step 8: Determine machine jobs.
	jobs := s.computeJobs(isControllerModel, stateInfo.IsController)

	// Step 9: Build volume params.
	volumes, volumeAttachments := s.buildVolumeParams(machineName, stateInfo, resourceTags, controllerUUID)

	// Step 10: Build root disk params.
	rootDisk := s.buildRootDisk(stateInfo.RootDiskStoragePool)

	return provisioner.ProvisioningInfo{
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
		CloudInitUserData:  parseCloudInitUserData(shared.CloudInitUserData),
		ControllerConfig:   shared.ControllerConfig,
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
// by the provider for provisioner. Returns an error if a required space
// is not found or has no usable subnets.
func (s *Service) buildNetworkTopology(
	ctx context.Context,
	machineID string,
	cons constraints.Value,
	spaceNames []network.SpaceName,
	allSpaces network.SpaceInfos,
	cloudType string,
) (map[string][]string, map[string][]string, error) {
	// If there are no space names, or the only space is alpha and it
	// wasn't explicitly constrained, return empty topology.
	consHasOnlyAlpha := len(cons.IncludeSpaces()) == 1 && cons.IncludeSpaces()[0] == network.AlphaSpaceName.String()
	if len(spaceNames) < 1 ||
		((len(spaceNames) == 1 && spaceNames[0] == network.AlphaSpaceName) && !consHasOnlyAlpha) {
		return nil, nil, nil
	}

	subnetAZs := make(map[string][]string)
	spaceSubnets := make(map[string][]string)

	for _, spaceName := range spaceNames {
		space := allSpaces.GetByName(spaceName)
		if space == nil {
			return nil, nil, errors.Errorf(
				"space %q not found", spaceName)
		}

		subnets := space.Subnets
		if len(subnets) == 0 {
			return nil, nil, errors.Errorf(
				"cannot use space %q as deployment target: no subnets", spaceName)
		}

		subnetIDs := s.spaceSubnetProviderIDs(ctx, machineID, spaceName, subnets, cloudType, subnetAZs)
		spaceSubnets[spaceName.String()] = subnetIDs
	}

	return spaceSubnets, subnetAZs, nil
}

// spaceSubnetProviderIDs returns the provider IDs for usable subnets in
// a space, populating subnetAZs as a side effect.
func (s *Service) spaceSubnetProviderIDs(
	ctx context.Context,
	machineID string,
	spaceName network.SpaceName,
	subnets network.SubnetInfos,
	cloudType string,
	subnetAZs map[string][]string,
) []string {
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
	return subnetIDs
}

// resolveImageMetadata returns image metadata from cache or falls back
// to the external fetcher.
func (s *Service) resolveImageMetadata(
	ctx context.Context,
	stateInfo provisioner.ProvisioningInfoState,
	shared provisioner.SharedProvisioningInfo,
	cachedImageMetadata []provisioner.CloudImageMetadata,
) ([]provisioner.CloudImageMetadata, error) {
	if len(cachedImageMetadata) > 0 {
		metadata := slices.SortedFunc(slices.Values(cachedImageMetadata), func(a, b provisioner.CloudImageMetadata) int {
			return cmp.Compare(a.Priority, b.Priority)
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

	constraint := provisioner.ImageConstraint{
		Releases: []string{base.Channel.Track},
		Arches:   arches,
		Stream:   imageStream(shared.ImageStream),
		Region:   shared.CloudRegion,
		Endpoint: shared.CloudEndpoint,
	}
	if stateInfo.Constraints.ImageID != nil {
		constraint.ImageID = stateInfo.Constraints.ImageID
	}

	metadata, err := s.imageMetadataFetcher.FetchImageMetadata(ctx, constraint)
	if err != nil {
		// Do not block provisioning if simplestreams lookup fails — some
		// providers can select images on their own without explicit metadata.
		s.logger.Warningf(ctx, "fetching image metadata from external sources: %v", err)
		return nil, nil
	}

	return slices.SortedFunc(slices.Values(metadata), func(a, b provisioner.CloudImageMetadata) int {
		return cmp.Compare(a.Priority, b.Priority)
	}), nil
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

	machineID := fmt.Sprintf("%s-machine-%s", modelName, machineName.String())
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
// final volume params, computing volume tags from model/controller metadata.
func (s *Service) buildVolumeParams(
	machineName coremachine.Name,
	stateInfo provisioner.ProvisioningInfoState,
	resourceTags map[string]string,
	controllerUUID string,
) ([]provisioner.VolumeParams, []provisioner.VolumeAttachmentParams) {
	// Compute model-level storage tags (resource tags + controller/model UUIDs).
	modelTags := make(map[string]string, len(resourceTags)+2)
	for k, v := range resourceTags {
		if !strings.HasPrefix(k, tags.JujuTagPrefix) {
			modelTags[k] = v
		}
	}
	modelTags[tags.JujuController] = controllerUUID
	modelTags[tags.JujuModel] = string(s.modelUUID)

	capturedVolumes := make(map[string]provisioner.VolumeParams, len(stateInfo.VolumeParams))

	for _, vp := range stateInfo.VolumeParams {
		attr := make(map[string]any, len(vp.Attributes))
		for k, v := range vp.Attributes {
			attr[k] = v
		}

		// Compute per-volume tags.
		vTags := make(map[string]string, len(modelTags)+2)
		maps.Copy(vTags, modelTags)
		storageInstTagVal := fmt.Sprintf("%s/%s", vp.StorageName, vp.StorageID)
		vTags[tags.JujuStorageInstance] = storageInstTagVal
		if vp.StorageOwnerUnitName != nil {
			vTags[tags.JujuStorageOwner] = *vp.StorageOwnerUnitName
		}

		capturedVolumes[vp.UUID] = provisioner.VolumeParams{
			VolumeID:   vp.ID,
			Provider:   vp.Provider,
			SizeMiB:    vp.RequestedSizeMiB,
			Attributes: attr,
			Tags:       vTags,
		}
	}

	var retVAParams []provisioner.VolumeAttachmentParams
	for _, ap := range stateInfo.VolumeAttachmentParams {
		attachParams := provisioner.VolumeAttachmentParams{
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

	volumes := make([]provisioner.VolumeParams, 0, len(capturedVolumes))
	for _, v := range capturedVolumes {
		volumes = append(volumes, v)
	}

	return volumes, retVAParams
}

// buildRootDisk converts the storage pool into root disk params.
func (s *Service) buildRootDisk(pool *provisioner.StoragePool) *provisioner.VolumeParams {
	if pool == nil {
		return nil
	}

	result := &provisioner.VolumeParams{
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

// parseCloudInitUserData parses a YAML string into a map.
// Returns nil if the string is empty or invalid.
func parseCloudInitUserData(raw string) map[string]any {
	if raw == "" {
		return nil
	}
	var result map[string]any
	if err := yaml.Unmarshal([]byte(raw), &result); err != nil {
		return nil
	}
	return result
}

// parseResourceTags parses a space-separated "key=value" string into a map.
// Returns the parsed tags and whether any were found.
func parseResourceTags(raw string) (map[string]string, bool) {
	if raw == "" {
		return nil, false
	}
	tags := make(map[string]string)
	for part := range strings.FieldsSeq(raw) {
		k, v, ok := strings.Cut(part, "=")
		if ok {
			tags[k] = v
		}
	}
	if len(tags) == 0 {
		return nil, false
	}
	return tags, true
}

// imageStream returns the image stream value, defaulting to "released"
// if the raw value is empty.
func imageStream(raw string) string {
	if raw == "" {
		return "released"
	}
	return raw
}
