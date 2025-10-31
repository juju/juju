// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/cloudimagemetadata"
	cloudimagemetadataerrors "github.com/juju/juju/domain/cloudimagemetadata/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	networkerrors "github.com/juju/juju/domain/network/errors"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// ProvisioningInfo returns the provisioning information for each given machine entity.
// It supports all positive space constraints.
func (api *ProvisionerAPI) ProvisioningInfo(ctx context.Context, args params.Entities) (params.ProvisioningInfoResults, error) {
	result := params.ProvisioningInfoResults{
		Results: make([]params.ProvisioningInfoResult, len(args.Entities)),
	}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return result, errors.Capture(err)
	}

	allSpaces, err := api.networkService.GetAllSpaces(ctx)
	if err != nil {
		return result, errors.Errorf("getting all space infos: %w", err)
	}

	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil || !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		machineName := coremachine.Name(tag.Id())
		result.Results[i].Result, err = api.getProvisioningInfo(ctx, machineName, allSpaces)

		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (api *ProvisionerAPI) getProvisioningInfo(
	ctx context.Context,
	machineName coremachine.Name,
	allSpaces network.SpaceInfos,
) (*params.ProvisioningInfo, error) {
	machineUUID, err := api.machineService.GetMachineUUID(ctx, machineName)
	switch {
	case errors.Is(err, machineerrors.MachineNotFound):
		return nil, errors.Errorf(
			"machine %q does not exist", machineName,
		).Add(coreerrors.NotFound)
	case err != nil:
		return nil, errors.Errorf("getting machine %q uuid: %w", machineName, err)
	}

	modelInfo, err := api.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return nil, errors.Errorf("getting model info: %w", err)
	}

	modelConfig, err := api.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Errorf("getting model config: %w", err)
	}

	unitNames, err := api.applicationService.GetUnitNamesOnMachine(ctx, machineName)
	if err != nil {
		return nil, errors.Errorf("getting unit names on machine %q: %w", machineName, err)
	}

	endpointBindings, err := api.machineEndpointBindings(ctx, unitNames)
	if err != nil {
		return nil, apiservererrors.ServerError(errors.Errorf("cannot determine machine endpoint bindings: %w", err))
	}

	spaceBindings, boundSpaceNames, err := api.translateEndpointBindingsToSpaces(allSpaces, endpointBindings)
	if err != nil {
		return nil, apiservererrors.ServerError(errors.Errorf("cannot determine spaces for endpoint bindings: %w", err))
	}

	var result params.ProvisioningInfo
	result, err = api.getProvisioningInfoBase(
		ctx, machineName, machineUUID, unitNames, spaceBindings, modelConfig,
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	machineSpaces, err := api.machineSpaces(result.Constraints, boundSpaceNames)
	if err != nil {
		return nil, errors.Capture(err)
	}

	if result.ProvisioningNetworkTopology, err = api.machineSpaceTopology(ctx, machineName.String(), result.Constraints, machineSpaces, modelInfo.CloudType); err != nil {
		return nil, errors.Errorf("matching subnets to zones: %w", err)
	}

	return &result, nil
}

func (api *ProvisionerAPI) getProvisioningInfoBase(
	ctx context.Context,
	machineName coremachine.Name,
	machineUUID coremachine.UUID,
	unitNames []coreunit.Name,
	endpointBindings map[string]string,
	modelConfig *config.Config,
) (params.ProvisioningInfo, error) {
	base, err := api.machineService.GetMachineBase(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return params.ProvisioningInfo{}, apiservererrors.ServerError(jujuerrors.NotFoundf("machine %q", machineName))
	} else if err != nil {
		return params.ProvisioningInfo{}, errors.Errorf("getting machine base: %w", err)
	}
	result := params.ProvisioningInfo{
		Base:              params.Base{Name: base.OS, Channel: base.Channel.String()},
		CloudInitUserData: modelConfig.CloudInitUserData(),
		// EndpointBindings are used by MAAS by the provider. Operator defined
		// space bindings are reflected in ProvisioningNetworkTopology.
		EndpointBindings: endpointBindings,
	}
	placement, err := api.machineService.GetMachinePlacementDirective(ctx, machineName)
	if err != nil {
		return params.ProvisioningInfo{}, errors.Errorf("getting machine placement directive: %w", err)
	}
	if placement != nil {
		result.Placement = *placement
	}

	cons, err := api.machineService.GetMachineConstraints(ctx, machineName)
	if err != nil {
		return result, errors.Capture(err)
	}
	result.Constraints = cons

	// The root disk source constraint might refer to a storage pool.
	if result.Constraints.HasRootDiskSource() {
		sp, err := api.storagePoolGetter.GetStoragePoolByName(ctx, *result.Constraints.RootDiskSource)
		if err != nil && !errors.Is(err, storageerrors.PoolNotFoundError) {
			return result, errors.Errorf("cannot load storage pool: %w", err)
		}
		if err == nil {
			result.RootDisk = &params.VolumeParams{
				Provider: sp.Provider,
			}
			if len(sp.Attrs) > 0 {
				result.RootDisk.Attributes = make(map[string]any, len(sp.Attrs))
				for k, v := range sp.Attrs {
					result.RootDisk.Attributes[k] = v
				}
			}
		}
	}

	volParams, volAttachParams, err := api.machineVolumeParams(
		ctx, machineName, machineUUID,
	)
	if err != nil {
		return params.ProvisioningInfo{}, errors.Capture(err)
	}
	result.Volumes = volParams
	result.VolumeAttachments = volAttachParams

	if result.CharmLXDProfiles, err =
		api.machineService.UpdateLXDProfiles(
			ctx, api.modelName, api.modelUUID, machineName.String(),
		); err != nil {
		return result, errors.Errorf("cannot write lxd profiles: %w", err)
	}

	if result.ImageMetadata, err = api.availableImageMetadata(ctx, machineName, modelConfig.ImageStream()); err != nil {
		return result, errors.Errorf("cannot get available image metadata: %w", err)
	}

	if result.ControllerConfig, err = api.controllerConfigService.ControllerConfig(ctx); err != nil {
		return result, errors.Errorf("cannot get controller configuration: %w", err)
	}

	isController, err := api.machineService.IsMachineController(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return result, apiservererrors.ServerError(jujuerrors.NotFoundf("machine %q", machineName))
	} else if err != nil {
		return result, errors.Errorf("checking if machine %q is a controller: %w", machineName, err)
	}

	// Every machine can host units, we just need to check if it is a controller
	// to determine if it can host models.
	jobs := []model.MachineJob{model.JobHostUnits}
	if isController {
		jobs = append(jobs, model.JobManageModel)
	}
	result.Jobs = jobs

	result.Tags, err = api.machineTags(ctx, unitNames, machineName, isController, modelConfig)
	if err != nil {
		return result, errors.Capture(err)
	}

	return result, nil
}

// machineVolumeParams is responsible for getting the information and
// constructing the machine volume and attachment parameters required during
// provisioning.
func (api *ProvisionerAPI) machineVolumeParams(
	ctx context.Context,
	machineName coremachine.Name,
	machineUUID coremachine.UUID,
) ([]params.VolumeParams, []params.VolumeAttachmentParams, error) {
	volumeParams, attachmentParams, err :=
		api.storageProvisioningService.GetMachineProvisioningVolumeParams(
			ctx, machineUUID,
		)
	switch {
	case errors.Is(err, machineerrors.MachineNotFound):
		return nil, nil, errors.Errorf("machine does not exist").Add(
			coreerrors.NotFound,
		)
	case err != nil:
		return nil, nil, errors.Errorf("getting machine volume params: %w", err)
	}

	capturedVolumeIDs := make(
		map[domainstorageprovisioning.VolumeUUID]struct{}, len(volumeParams),
	)
	retValVParams := make([]params.VolumeParams, 0, len(volumeParams))
	for _, vp := range volumeParams {
		capturedVolumeIDs[vp.UUID] = struct{}{}
		vTag, err := names.ParseVolumeTag(names.VolumeTagKind + "-" + vp.ID)
		if err != nil {
			return nil, nil, errors.Errorf(
				"parsing volume id to a volume tag: %w", err,
			)
		}

		attr := make(map[string]any, len(vp.Attributes))
		for k, v := range vp.Attributes {
			attr[k] = v
		}

		retValVParams = append(retValVParams, params.VolumeParams{
			// We don't set attachment info
			Attributes: attr,
			Provider:   vp.Provider,
			SizeMiB:    vp.RequestedSizeMiB,
			Tags:       vp.Tags,
			VolumeTag:  vTag.String(),
		})
	}

	machineTag := names.NewMachineTag(machineName.String())
	retValVAParams := make([]params.VolumeAttachmentParams, 0, len(attachmentParams))
	for _, ap := range attachmentParams {
		if _, has := capturedVolumeIDs[ap.VolumeUUID]; has {
			// NOTE (tlm): This logic comes from 3.6 where if we supply a volume
			// param to the caller any associated attachment should not be
			// included. This is because the code assumes how the environ
			// works and that if the environ is making a volume there must be an
			// associate attachment to establish. The better design would be to
			// pass all the volume and attachment information back and let the
			// environ storage provider establish what it wants to do. We also
			// gain the ability then to pass specific attachment information to
			// the environ which we loose out on here.
			continue
		}

		vTag, err := names.ParseVolumeTag(names.VolumeTagKind + "-" + ap.VolumeID)
		if err != nil {
			return nil, nil, errors.Errorf(
				"parsing volume attachment volume id to a volume tag: %w", err,
			)
		}
		retValVAParams = append(retValVAParams, params.VolumeAttachmentParams{
			MachineTag: machineTag.String(),
			Provider:   ap.Provider,
			ReadOnly:   ap.ReadOnly,
			ProviderId: ap.VolumeProviderID,
			VolumeTag:  vTag.String(),
		})
	}
	return retValVParams, retValVAParams, nil
}

// machineTags returns machine-specific tags to set on the instance.
func (api *ProvisionerAPI) machineTags(
	ctx context.Context,
	unitNames []coreunit.Name,
	machineName coremachine.Name,
	isController bool,
	modelConfig *config.Config,
) (map[string]string, error) {
	// Names of all units deployed to the machine.
	//
	// TODO(axw) 2015-06-02 #1461358
	// We need a worker that periodically updates
	// instance tags with current deployment info.
	principalUnitNames := make([]string, 0, len(unitNames))
	for _, unitName := range unitNames {
		_, isPrincipal, err := api.applicationService.GetUnitPrincipal(ctx, unitName)
		if err != nil {
			return nil, errors.Errorf("getting unit principal for unit %q: %w", unitName, err)
		}
		if isPrincipal {
			principalUnitNames = append(principalUnitNames, unitName.String())
		}
	}
	sort.Strings(principalUnitNames)

	machineTags := instancecfg.InstanceTags(api.modelUUID.String(), api.controllerUUID, modelConfig, isController)
	if len(unitNames) > 0 {
		machineTags[tags.JujuUnitsDeployed] = strings.Join(principalUnitNames, " ")
	}

	machineID := fmt.Sprintf("%s-%s", api.modelName, names.NewMachineTag(machineName.String()).String())
	machineTags[tags.JujuMachine] = machineID

	return machineTags, nil
}

// machineSpaces returns the list of spaces that the machine must be in.
// Note that we will send a topology for the *union* of space constraints
// and bindings.
//
// We need to do this because some providers need to *choose* an instance
// fulfilling them all (MAAS/AWS) whereas others *create* an instance to
// fulfill them (OpenStack will create the NICs it needs).
//
// This means there is a difference between add-machine, which will only
// include the spaces based on constraints, and deploy/add-unit,
// which will include spaces for any endpoint bindings.
//
// It is the responsibility of the provider to negotiate this information
// appropriately.
func (api *ProvisionerAPI) machineSpaces(
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

	return transform.Slice(includeSpaces.SortedValues(), func(s string) network.SpaceName { return network.SpaceName(s) }), nil
}

func (api *ProvisionerAPI) machineSpaceTopology(
	ctx context.Context,
	machineID string,
	cons constraints.Value,
	spaceNames []network.SpaceName,
	cloudType string,
) (params.ProvisioningNetworkTopology, error) {
	var topology params.ProvisioningNetworkTopology

	// If there are no space names, or if there is only one space
	// name and that's the alpha space unless it was explicitly set as a
	// constraint, we don't bother setting a topology that constrains
	// provisioning.
	consHasOnlyAlpha := len(cons.IncludeSpaces()) == 1 && cons.IncludeSpaces()[0] == network.AlphaSpaceName.String()
	if len(spaceNames) < 1 ||
		((len(spaceNames) == 1 && spaceNames[0] == network.AlphaSpaceName) && !consHasOnlyAlpha) {
		return topology, nil
	}

	topology.SubnetAZs = make(map[string][]string)
	topology.SpaceSubnets = make(map[string][]string)

	for _, spaceName := range spaceNames {
		subnetsAndZones, err := api.subnetsAndZonesForSpace(ctx, machineID, spaceName, cloudType)
		if err != nil {
			if errors.Is(err, networkerrors.SpaceNotFound) {
				return topology, jujuerrors.NotFoundf("space with name %q", spaceName)
			}
			return topology, errors.Capture(err)
		}

		// Record each subnet provider ID as being in the space,
		// and add the zone mappings to our map
		subnetIDs := make([]string, 0, len(subnetsAndZones))
		for sID, zones := range subnetsAndZones {
			// We do not expect unique provider subnets to be in more than one
			// space, so no subnet should be processed more than once.
			// Log a warning if this happens.
			if _, ok := topology.SpaceSubnets[sID]; ok {
				api.logger.Warningf(ctx, "subnet with provider ID %q found is present in multiple spaces", sID)
			}
			topology.SubnetAZs[sID] = zones
			subnetIDs = append(subnetIDs, sID)
		}
		topology.SpaceSubnets[spaceName.String()] = subnetIDs
	}

	return topology, nil
}

func (api *ProvisionerAPI) subnetsAndZonesForSpace(
	ctx context.Context,
	machineID string,
	spaceName network.SpaceName,
	cloudType string,
) (map[string][]string, error) {
	space, err := api.networkService.SpaceByName(ctx, spaceName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	subnets := space.Subnets

	if len(subnets) == 0 {
		return nil, errors.Errorf("cannot use space %q as deployment target: no subnets", spaceName)
	}

	subnetsToZones := make(map[string][]string, len(subnets))
	for _, subnet := range subnets {
		warningPrefix := fmt.Sprintf("not using subnet %q in space %q for machine %q provisioning: ",
			subnet.CIDR, spaceName, machineID,
		)

		providerID := subnet.ProviderId
		if providerID == "" {
			api.logger.Warningf(ctx, warningPrefix+"no ProviderId set")
			continue
		}

		zones := subnet.AvailabilityZones
		if len(zones) == 0 {
			// For most providers we expect availability zones, however:
			// - Azure uses Availability Sets.
			// - OpenStack networks have R/W availability zone *hints*,
			//   and AZs based on the actual scheduling of the resource.
			// For these cases we allow empty map entries.
			// TODO (manadart 2022-11-10): Bring this condition under testing
			// when we cut machine handling over to Dqlite.
			if cloudType != "azure" && cloudType != "openstack" {
				api.logger.Warningf(ctx, warningPrefix+"no availability zone(s) set")
				continue
			}
		}

		subnetsToZones[string(providerID)] = zones
	}
	return subnetsToZones, nil
}

func (api *ProvisionerAPI) machineEndpointBindings(ctx context.Context, unitNames []coreunit.Name) (map[string]map[string]network.SpaceUUID, error) {
	endpointBindings := make(map[string]map[string]network.SpaceUUID)
	for _, unitName := range unitNames {
		_, isPrincipal, err := api.applicationService.GetUnitPrincipal(ctx, unitName)
		if err != nil {
			return nil, errors.Errorf("checking principal for unit %q: %w", unitName, err)
		}
		if !isPrincipal {
			continue
		}

		appName := unitName.Application()
		if _, ok := endpointBindings[appName]; ok {
			// Already processed, skip it.
			continue
		}
		bindings, err := api.applicationService.GetApplicationEndpointBindings(ctx, appName)
		if err != nil {
			return nil, errors.Errorf("getting endpoint bindings for application %q: %w", appName, err)
		}
		endpointBindings[appName] = bindings
	}
	return endpointBindings, nil
}

func (api *ProvisionerAPI) translateEndpointBindingsToSpaces(spaceInfos network.SpaceInfos, endpointBindings map[string]map[string]network.SpaceUUID) (map[string]string, []network.SpaceName, error) {
	combinedBindings := make(map[string]string)
	var boundSpaceNames []network.SpaceName
	for _, bindings := range endpointBindings {
		if len(bindings) == 0 {
			continue
		}

		for endpoint, spaceID := range bindings {
			space := spaceInfos.GetByID(spaceID)
			boundSpaceNames = append(boundSpaceNames, space.Name)
			if space != nil {
				bound := string(space.ProviderId)
				if bound == "" {
					bound = space.Name.String()
				}
				combinedBindings[endpoint] = bound
			} else {
				// Technically, this can't happen in practice, as we're
				// validating the bindings during application deployment.
				return nil, nil, errors.Errorf("unknown space %q with no provider ID specified for endpoint %q", spaceID, endpoint)
			}
		}
	}
	return combinedBindings, boundSpaceNames, nil
}

// availableImageMetadata returns all image metadata available to this machine
// or an error fetching them.
func (api *ProvisionerAPI) availableImageMetadata(
	ctx context.Context,
	machineName coremachine.Name,
	imageStream string,
) ([]params.CloudImageMetadata, error) {
	imageConstraint, err := api.constructImageConstraint(ctx, machineName, imageStream)
	if err != nil {
		return nil, errors.Errorf("could not construct image constraint: %w", err)
	}

	data, err := api.findImageMetadata(ctx, imageConstraint, imageStream)
	if err != nil {
		return nil, errors.Capture(err)
	}
	sort.Slice(data, func(i, j int) bool {
		return data[i].Priority < data[j].Priority
	})
	api.logger.Debugf(ctx, "available image metadata for provisioning: %v", data)
	return data, nil
}

// constructImageConstraint returns model-specific criteria used to look for image metadata.
func (api *ProvisionerAPI) constructImageConstraint(
	ctx context.Context,
	machineName coremachine.Name,
	imageStream string,
) (*imagemetadata.ImageConstraint, error) {
	machineBase, err := api.machineService.GetMachineBase(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return nil, apiservererrors.ServerError(jujuerrors.NotFoundf("machine %q", machineName))
	} else if err != nil {
		return nil, errors.Errorf("getting machine base: %w", err)
	}

	base, err := corebase.ParseBase(machineBase.OS, machineBase.Channel.String())
	if err != nil {
		return nil, errors.Capture(err)
	}
	vers := base.Channel.Track
	lookup := simplestreams.LookupParams{
		Releases: []string{vers},
		Stream:   imageStream,
	}

	// NOTE(nvinuesa): We should rethink this, so we can get the constraints
	// and the base from the same service call.
	cons, err := api.machineService.GetMachineConstraints(ctx, machineName)
	if err != nil {
		return nil, errors.Errorf("cannot get machine constraints for machine %v: %w", machineName, err)
	}

	if cons.Arch != nil {
		lookup.Arches = []string{*cons.Arch}
	}

	spec, err := api.modelInfoService.GetRegionCloudSpec(ctx)
	if err != nil {
		return nil, errors.Errorf("cannot get region cloud spec for this model: %w", err)
	}
	lookup.CloudSpec = spec

	return imagemetadata.NewImageConstraint(lookup, cons.ImageID)
}

// findImageMetadata returns all image metadata or an error fetching them.
// It looks for cached or custom image metadata in the CloudImageMetadata service.
// If none are found, we fall back on original image search in simple streams.
func (api *ProvisionerAPI) findImageMetadata(
	ctx context.Context,
	imageConstraint *imagemetadata.ImageConstraint,
	imageStream string,
) ([]params.CloudImageMetadata, error) {
	// Look for image metadata in the service (cached or custom metadata).
	serviceMetadata, err := api.imageMetadataFromService(ctx, imageConstraint)
	if err != nil {
		// look into simple stream if for some reason metadata can't be got from the service
		// so do not exit on error.
		api.logger.Infof(ctx, "could not get image metadata from controller: %v", err)
	}
	api.logger.Debugf(ctx, "got from controller %d metadata", len(serviceMetadata))
	// No need to look in data sources if found through service.
	if len(serviceMetadata) != 0 {
		return serviceMetadata, nil
	}

	// If no metadata is found through the service, fall back to original simple stream search.
	// Currently, an image metadata worker picks up this metadata periodically (daily),
	// and stores it. So potentially, this data could be different
	// to what is cached.
	dsMetadata, err := api.imageMetadataFromDataSources(ctx, imageConstraint, imageStream)
	if err != nil {
		if !errors.Is(err, jujuerrors.NotFound) {
			return nil, errors.Capture(err)
		}
	}
	api.logger.Debugf(ctx, "got from data sources %d metadata", len(dsMetadata))

	return dsMetadata, nil
}

// imageMetadataFromService returns image metadata stored in the service
// that matches given criteria.
func (api *ProvisionerAPI) imageMetadataFromService(ctx context.Context, constraint *imagemetadata.ImageConstraint) ([]params.CloudImageMetadata, error) {
	filter := cloudimagemetadata.MetadataFilter{
		Versions: constraint.Releases,
		Arches:   constraint.Arches,
		Region:   constraint.Region,
		Stream:   constraint.Stream,
	}
	if constraint.ImageID != nil {
		filter.ImageID = *constraint.ImageID
	}
	stored, err := api.cloudImageMetadataService.FindMetadata(ctx, filter)
	if err != nil {
		return nil, errors.Capture(err)
	}

	toParams := func(m cloudimagemetadata.Metadata) params.CloudImageMetadata {
		return params.CloudImageMetadata{
			ImageId:         m.ImageID,
			Stream:          m.Stream,
			Region:          m.Region,
			Version:         m.Version,
			Arch:            m.Arch,
			VirtType:        m.VirtType,
			RootStorageType: m.RootStorageType,
			RootStorageSize: m.RootStorageSize,
			Source:          m.Source,
			Priority:        m.Priority,
		}
	}

	var all []params.CloudImageMetadata
	for _, ms := range stored {
		for _, m := range ms {
			all = append(all, toParams(m))
		}
	}
	return all, nil
}

// imageMetadataFromDataSources finds image metadata that match specified criteria in existing data sources.
func (api *ProvisionerAPI) imageMetadataFromDataSources(
	ctx context.Context,
	constraint *imagemetadata.ImageConstraint,
	defaultImageStream string,
) ([]params.CloudImageMetadata, error) {
	env, err := api.machineService.GetBootstrapEnviron(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	fetcher := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	sources, err := environs.ImageMetadataSources(env, fetcher)
	if err != nil {
		return nil, errors.Capture(err)
	}

	toModel := func(m *imagemetadata.ImageMetadata, source string, priority int) cloudimagemetadata.Metadata {
		result := cloudimagemetadata.Metadata{
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Region:          m.RegionName,
				Arch:            m.Arch,
				VirtType:        m.VirtType,
				RootStorageType: m.Storage,
				Source:          source,
				Stream:          m.Stream,
				Version:         m.Version,
			},
			Priority: priority,
			ImageID:  m.Id,
		}
		// TODO (anastasiamac 2016-08-24) This is a band-aid solution.
		// Once correct value is read from simplestreams, this needs to go.
		// Bug# 1616295
		if result.Stream == "" {
			result.Stream = constraint.Stream
		}
		if result.Stream == "" {
			result.Stream = defaultImageStream
		}
		return result
	}

	var metadata []cloudimagemetadata.Metadata
	for _, source := range sources {
		api.logger.Debugf(ctx, "looking in data source %v", source.Description())
		found, info, err := imagemetadata.Fetch(ctx, fetcher, []simplestreams.DataSource{source}, constraint)
		if errors.Is(err, jujuerrors.NotFound) || errors.Is(err, jujuerrors.Unauthorized) {
			// Do not stop looking in other data sources if there is an issue here.
			api.logger.Warningf(ctx, "encountered %v while getting published images metadata from %v", err, source.Description())
			continue
		} else if err != nil {
			// When we get an actual protocol/unexpected error, we need to stop.
			return nil, errors.Errorf("failed getting published images metadata from %s: %w", source.Description(), err)
		}

		for _, m := range found {
			metadata = append(metadata, toModel(m, info.Source, source.Priority()))
		}
	}
	if len(metadata) > 0 {
		if err := api.cloudImageMetadataService.SaveMetadata(ctx, metadata); err != nil {
			// No need to react here, just take note
			api.logger.Warningf(ctx, "failed to save published image metadata: %v", err)
		}
	}

	// Since we've fallen through to data sources search and have saved all needed images in the service,
	// let's try to get them from the service to avoid duplication of conversion logic here.
	all, err := api.imageMetadataFromService(ctx, constraint)
	if err != nil && !errors.Is(err, cloudimagemetadataerrors.NotFound) {
		return nil, errors.Errorf("could not read metadata from the service after saving it there from data sources: %w", err)
	}

	if len(all) == 0 {
		return nil, jujuerrors.NotFoundf("image metadata for version %v, arch %v", constraint.Releases, constraint.Arches)
	}

	return all, nil
}
