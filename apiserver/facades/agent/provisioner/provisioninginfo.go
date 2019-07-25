// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/os/series"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage"
)

// ProvisioningInfo returns the provisioning information for each given machine entity.
func (p *ProvisionerAPI) ProvisioningInfo(args params.Entities) (params.ProvisioningInfoResults, error) {
	result := params.ProvisioningInfoResults{
		Results: make([]params.ProvisioningInfoResult, len(args.Entities)),
	}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		return result, errors.Trace(err)
	}
	env, err := environs.GetEnviron(p.configGetter, environs.New)
	if err != nil {
		return result, errors.Annotate(err, "could not get environ")
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		machine, err := p.getMachine(canAccess, tag)
		if err == nil {
			result.Results[i].Result, err = p.getProvisioningInfo(machine, env)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (p *ProvisionerAPI) getProvisioningInfo(m *state.Machine, env environs.Environ) (*params.ProvisioningInfo, error) {
	cons, err := m.Constraints()
	if err != nil {
		return nil, errors.Trace(err)
	}

	volumes, volumeAttachments, err := p.machineVolumeParams(m, env)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var jobs []multiwatcher.MachineJob
	for _, job := range m.Jobs() {
		jobs = append(jobs, job.ToParams())
	}

	tags, err := p.machineTags(m, jobs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	subnetsToZones, err := p.machineSubnetsAndZones(m)
	if err != nil {
		return nil, errors.Annotate(err, "cannot match subnets to zones")
	}

	pNames, err := p.machineLXDProfileNames(m, env)
	if err != nil {
		return nil, errors.Annotate(err, "cannot write lxd profiles")
	}

	endpointBindings, err := p.machineEndpointBindings(m)
	if err != nil {
		return nil, errors.Annotate(err, "cannot determine machine endpoint bindings")
	}

	imageMetadata, err := p.availableImageMetadata(m, env)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get available image metadata")
	}

	controllerCfg, err := p.st.ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get controller configuration")
	}

	return &params.ProvisioningInfo{
		Constraints:       cons,
		Series:            m.Series(),
		Placement:         m.Placement(),
		Jobs:              jobs,
		Volumes:           volumes,
		VolumeAttachments: volumeAttachments,
		Tags:              tags,
		SubnetsToZones:    subnetsToZones,
		EndpointBindings:  endpointBindings,
		ImageMetadata:     imageMetadata,
		ControllerConfig:  controllerCfg,
		CloudInitUserData: env.Config().CloudInitUserData(),
		CharmLXDProfiles:  pNames,
	}, nil
}

// machineVolumeParams retrieves VolumeParams for the volumes that should be
// provisioned with, and attached to, the machine. The client should ignore
// parameters that it does not know how to handle.
func (p *ProvisionerAPI) machineVolumeParams(
	m *state.Machine,
	env environs.Environ,
) ([]params.VolumeParams, []params.VolumeAttachmentParams, error) {
	sb, err := state.NewStorageBackend(p.st)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	volumeAttachments, err := m.VolumeAttachments()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if len(volumeAttachments) == 0 {
		return nil, nil, nil
	}
	modelConfig, err := p.m.ModelConfig()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	controllerCfg, err := p.st.ControllerConfig()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	allVolumeParams := make([]params.VolumeParams, 0, len(volumeAttachments))
	var allVolumeAttachmentParams []params.VolumeAttachmentParams
	for _, volumeAttachment := range volumeAttachments {
		volumeTag := volumeAttachment.Volume()
		volume, err := sb.Volume(volumeTag)
		if err != nil {
			return nil, nil, errors.Annotatef(err, "getting volume %q", volumeTag.Id())
		}
		storageInstance, err := storagecommon.MaybeAssignedStorageInstance(
			volume.StorageInstance, sb.StorageInstance,
		)
		if err != nil {
			return nil, nil, errors.Annotatef(err, "getting volume %q storage instance", volumeTag.Id())
		}
		volumeParams, err := storagecommon.VolumeParams(
			volume, storageInstance, modelConfig.UUID(), controllerCfg.ControllerUUID(),
			modelConfig, p.storagePoolManager, p.storageProviderRegistry,
		)
		if err != nil {
			return nil, nil, errors.Annotatef(err, "getting volume %q parameters", volumeTag.Id())
		}
		if _, err := env.StorageProvider(storage.ProviderType(volumeParams.Provider)); errors.IsNotFound(err) {
			// This storage type is not managed by the environ
			// provider, so ignore it. It'll be managed by one
			// of the storage provisioners.
			continue
		} else if err != nil {
			return nil, nil, errors.Annotate(err, "getting storage provider")
		}

		var volumeProvisioned bool
		volumeInfo, err := volume.Info()
		if err == nil {
			volumeProvisioned = true
		} else if !errors.IsNotProvisioned(err) {
			return nil, nil, errors.Annotate(err, "getting volume info")
		}
		stateVolumeAttachmentParams, volumeDetached := volumeAttachment.Params()
		if !volumeDetached {
			// Volume is already attached to the machine, so
			// there's nothing more to do for it.
			continue
		}
		volumeAttachmentParams := params.VolumeAttachmentParams{
			volumeTag.String(),
			m.Tag().String(),
			volumeInfo.VolumeId,
			"", // we're creating the machine, so it has no instance ID.
			volumeParams.Provider,
			stateVolumeAttachmentParams.ReadOnly,
		}
		if volumeProvisioned {
			// Volume is already provisioned, so we just need to attach it.
			allVolumeAttachmentParams = append(
				allVolumeAttachmentParams, volumeAttachmentParams,
			)
		} else {
			// Not provisioned yet, so ask the cloud provisioner do it.
			volumeParams.Attachment = &volumeAttachmentParams
			allVolumeParams = append(allVolumeParams, volumeParams)
		}
	}
	return allVolumeParams, allVolumeAttachmentParams, nil
}

// machineTags returns machine-specific tags to set on the instance.
func (p *ProvisionerAPI) machineTags(m *state.Machine, jobs []multiwatcher.MachineJob) (map[string]string, error) {
	// Names of all units deployed to the machine.
	//
	// TODO(axw) 2015-06-02 #1461358
	// We need a worker that periodically updates
	// instance tags with current deployment info.
	units, err := m.Units()
	if err != nil {
		return nil, errors.Trace(err)
	}
	unitNames := make([]string, 0, len(units))
	for _, unit := range units {
		if !unit.IsPrincipal() {
			continue
		}
		unitNames = append(unitNames, unit.Name())
	}
	sort.Strings(unitNames)

	cfg, err := p.m.ModelConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerCfg, err := p.st.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	machineTags := instancecfg.InstanceTags(cfg.UUID(), controllerCfg.ControllerUUID(), cfg, jobs)
	if len(unitNames) > 0 {
		machineTags[tags.JujuUnitsDeployed] = strings.Join(unitNames, " ")
	}
	machineId := fmt.Sprintf("%s-%s", cfg.Name(), m.Tag().String())
	machineTags[tags.JujuMachine] = machineId
	return machineTags, nil
}

// machineSubnetsAndZones returns a map of subnet provider-specific id
// to list of availability zone names for that subnet. The result can
// be empty if there are no spaces constraints specified for the
// machine, or there's an error fetching them.
func (p *ProvisionerAPI) machineSubnetsAndZones(m *state.Machine) (map[string][]string, error) {
	mcons, err := m.Constraints()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get machine constraints")
	}
	includeSpaces := mcons.IncludeSpaces()
	if len(includeSpaces) < 1 {
		// Nothing to do.
		return nil, nil
	}
	// TODO(dimitern): For the network model MVP we only use the first
	// included space and ignore the rest.
	//
	// LKK Card: https://canonical.leankit.com/Boards/View/101652562/117352306
	// LP Bug: http://pad.lv/1498232
	spaceName := includeSpaces[0]
	if len(includeSpaces) > 1 {
		logger.Debugf(
			"using space %q from constraints for machine %q (ignoring remaining: %v)",
			spaceName, m.Id(), includeSpaces[1:],
		)
	}
	space, err := p.st.Space(spaceName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnets, err := space.Subnets()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(subnets) == 0 {
		return nil, errors.Errorf("cannot use space %q as deployment target: no subnets", spaceName)
	}
	subnetsToZones := make(map[string][]string, len(subnets))
	for _, subnet := range subnets {
		warningPrefix := fmt.Sprintf(
			"not using subnet %q in space %q for machine %q provisioning: ",
			subnet.CIDR(), spaceName, m.Id(),
		)
		providerId := subnet.ProviderId()
		if providerId == "" {
			logger.Warningf(warningPrefix + "no ProviderId set")
			continue
		}
		zones := subnet.AvailabilityZones()
		if len(zones) == 0 {
			logger.Warningf(warningPrefix + "no availability zone(s) set")
			continue
		}
		subnetsToZones[string(providerId)] = zones
	}
	return subnetsToZones, nil
}

// machineLXDProfileNames give the environ info to write lxd profiles needed for
// the given machine and returns the names of profiles. Unlike
// containerLXDProfilesInfo which returns the info necessary to write lxd profiles
// via the lxd broker.
func (p *ProvisionerAPI) machineLXDProfileNames(m *state.Machine, env environs.Environ) ([]string, error) {
	profileEnv, ok := env.(environs.LXDProfiler)
	if !ok {
		logger.Tracef("LXDProfiler not implemented by environ")
		return nil, nil
	}
	units, err := m.Units()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var names []string
	for _, unit := range units {
		app, err := unit.Application()
		if err != nil {
			return nil, errors.Trace(err)
		}
		ch, _, err := app.Charm()
		if err != nil {
			return nil, errors.Trace(err)
		}
		profile := ch.LXDProfile()
		if profile == nil || (profile != nil && profile.Empty()) {
			continue
		}
		pName := lxdprofile.Name(p.m.Name(), app.Name(), ch.Revision())
		// Lock here, we get a new env for every call to ProvisioningInfo().
		p.mu.Lock()
		if err := profileEnv.MaybeWriteLXDProfile(pName, profile); err != nil {
			p.mu.Unlock()
			return nil, errors.Trace(err)
		}
		p.mu.Unlock()
		names = append(names, pName)
	}
	return names, nil
}

func (p *ProvisionerAPI) machineEndpointBindings(m *state.Machine) (map[string]string, error) {
	units, err := m.Units()
	if err != nil {
		return nil, errors.Trace(err)
	}

	spacesNamesToProviderIds, err := p.allSpaceNamesToProviderIds()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var combinedBindings map[string]string
	processedServicesSet := set.NewStrings()
	for _, unit := range units {
		if !unit.IsPrincipal() {
			continue
		}
		service, err := unit.Application()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if processedServicesSet.Contains(service.Name()) {
			// Already processed, skip it.
			continue
		}
		bindings, err := service.EndpointBindings()
		if err != nil {
			return nil, errors.Trace(err)
		}
		processedServicesSet.Add(service.Name())

		if len(bindings) == 0 {
			continue
		}
		if combinedBindings == nil {
			combinedBindings = make(map[string]string)
		}

		for endpoint, spaceName := range bindings {
			if spaceName == "" {
				// Skip unspecified bindings, as they won't affect the instance
				// selected for provisioning.
				continue
			}

			spaceProviderId, nameKnown := spacesNamesToProviderIds[spaceName]
			if nameKnown {
				combinedBindings[endpoint] = spaceProviderId
			} else {
				// Technically, this can't happen in practice, as we're
				// validating the bindings during service deployment.
				return nil, errors.Errorf("unknown space %q with no provider ID specified for endpoint %q", spaceName, endpoint)
			}
		}
	}
	return combinedBindings, nil
}

func (p *ProvisionerAPI) allSpaceNamesToProviderIds() (map[string]string, error) {
	allSpaces, err := p.st.AllSpaces()
	if err != nil {
		return nil, errors.Annotate(err, "getting all spaces")
	}

	namesToProviderIds := make(map[string]string, len(allSpaces))
	for _, space := range allSpaces {
		name := space.Name()

		// For providers without native support for spaces, use the name instead
		// as provider ID.
		providerId := string(space.ProviderId())
		if len(providerId) == 0 {
			providerId = name
		}

		namesToProviderIds[name] = providerId
	}

	return namesToProviderIds, nil
}

// availableImageMetadata returns all image metadata available to this machine
// or an error fetching them.
func (p *ProvisionerAPI) availableImageMetadata(m *state.Machine, env environs.Environ) ([]params.CloudImageMetadata, error) {
	imageConstraint, err := p.constructImageConstraint(m, env)
	if err != nil {
		return nil, errors.Annotate(err, "could not construct image constraint")
	}

	// Look for image metadata in state.
	data, err := p.findImageMetadata(imageConstraint, env)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sort.Sort(metadataList(data))
	logger.Debugf("available image metadata for provisioning: %v", data)
	return data, nil
}

// constructImageConstraint returns model-specific criteria used to look for image metadata.
func (p *ProvisionerAPI) constructImageConstraint(m *state.Machine, env environs.Environ) (*imagemetadata.ImageConstraint, error) {
	lookup := simplestreams.LookupParams{
		Series: []string{m.Series()},
		Stream: env.Config().ImageStream(),
	}

	mcons, err := m.Constraints()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get machine constraints for machine %v", m.MachineTag().Id())
	}

	if mcons.Arch != nil {
		lookup.Arches = []string{*mcons.Arch}
	}

	if hasRegion, ok := env.(simplestreams.HasRegion); ok {
		// We can determine current region; we want only
		// metadata specific to this region.
		spec, err := hasRegion.Region()
		if err != nil {
			// can't really find images if we cannot determine cloud region
			// TODO (anastasiamac 2015-12-03) or can we?
			return nil, errors.Annotate(err, "getting provider region information (cloud spec)")
		}
		lookup.CloudSpec = spec
	}

	return imagemetadata.NewImageConstraint(lookup), nil
}

// findImageMetadata returns all image metadata or an error fetching them.
// It looks for image metadata in state.
// If none are found, we fall back on original image search in simple streams.
func (p *ProvisionerAPI) findImageMetadata(imageConstraint *imagemetadata.ImageConstraint, env environs.Environ) ([]params.CloudImageMetadata, error) {
	// Look for image metadata in state.
	stateMetadata, err := p.imageMetadataFromState(imageConstraint)
	if err != nil && !errors.IsNotFound(err) {
		// look into simple stream if for some reason can't get from controller,
		// so do not exit on error.
		logger.Infof("could not get image metadata from controller: %v", err)
	}
	logger.Debugf("got from controller %d metadata", len(stateMetadata))
	// No need to look in data sources if found in state.
	if len(stateMetadata) != 0 {
		return stateMetadata, nil
	}

	// If no metadata is found in state, fall back to original simple stream search.
	// Currently, an image metadata worker picks up this metadata periodically (daily),
	// and stores it in state. So potentially, this collection could be different
	// to what is in state.
	dsMetadata, err := p.imageMetadataFromDataSources(env, imageConstraint)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
	}
	logger.Debugf("got from data sources %d metadata", len(dsMetadata))

	return dsMetadata, nil
}

// imageMetadataFromState returns image metadata stored in state
// that matches given criteria.
func (p *ProvisionerAPI) imageMetadataFromState(constraint *imagemetadata.ImageConstraint) ([]params.CloudImageMetadata, error) {
	filter := cloudimagemetadata.MetadataFilter{
		Series: constraint.Series,
		Arches: constraint.Arches,
		Region: constraint.Region,
		Stream: constraint.Stream,
	}
	stored, err := p.st.CloudImageMetadataStorage.FindMetadata(filter)
	if err != nil {
		return nil, errors.Trace(err)
	}

	toParams := func(m cloudimagemetadata.Metadata) params.CloudImageMetadata {
		return params.CloudImageMetadata{
			ImageId:         m.ImageId,
			Stream:          m.Stream,
			Region:          m.Region,
			Version:         m.Version,
			Series:          m.Series,
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
func (p *ProvisionerAPI) imageMetadataFromDataSources(env environs.Environ, constraint *imagemetadata.ImageConstraint) ([]params.CloudImageMetadata, error) {
	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cfg := env.Config()
	toModel := func(m *imagemetadata.ImageMetadata, mSeries string, source string, priority int) cloudimagemetadata.Metadata {
		result := cloudimagemetadata.Metadata{
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Region:          m.RegionName,
				Arch:            m.Arch,
				VirtType:        m.VirtType,
				RootStorageType: m.Storage,
				Source:          source,
				Series:          mSeries,
				Stream:          m.Stream,
				Version:         m.Version,
			},
			Priority: priority,
			ImageId:  m.Id,
		}
		// TODO (anastasiamac 2016-08-24) This is a band-aid solution.
		// Once correct value is read from simplestreams, this needs to go.
		// Bug# 1616295
		if result.Stream == "" {
			result.Stream = constraint.Stream
		}
		if result.Stream == "" {
			result.Stream = cfg.ImageStream()
		}
		return result
	}

	var metadataState []cloudimagemetadata.Metadata
	for _, source := range sources {
		logger.Debugf("looking in data source %v", source.Description())
		found, info, err := imagemetadata.Fetch([]simplestreams.DataSource{source}, constraint)
		if err != nil {
			// Do not stop looking in other data sources if there is an issue here.
			logger.Warningf("encountered %v while getting published images metadata from %v", err, source.Description())
			continue
		}
		for _, m := range found {
			mSeries, err := series.VersionSeries(m.Version)
			if err != nil {
				logger.Warningf("could not determine series for image id %s: %v", m.Id, err)
				continue
			}
			metadataState = append(metadataState, toModel(m, mSeries, info.Source, source.Priority()))
		}
	}
	if len(metadataState) > 0 {
		if err := p.st.CloudImageMetadataStorage.SaveMetadata(metadataState); err != nil {
			// No need to react here, just take note
			logger.Warningf("failed to save published image metadata: %v", err)
		}
	}

	// Since we've fallen through to data sources search and have saved all needed images into controller,
	// let's try to get them from controller to avoid duplication of conversion logic here.
	all, err := p.imageMetadataFromState(constraint)
	if err != nil {
		return nil, errors.Annotate(err, "could not read metadata from controller after saving it there from data sources")
	}

	if len(all) == 0 {
		return nil, errors.NotFoundf("image metadata for series %v, arch %v", constraint.Series, constraint.Arches)
	}

	return all, nil
}

// metadataList is a convenience type enabling to sort
// a collection of CloudImageMetadata in order of priority.
type metadataList []params.CloudImageMetadata

// Implements sort.Interface
func (m metadataList) Len() int {
	return len(m)
}

// Implements sort.Interface and sorts image metadata by priority.
func (m metadataList) Less(i, j int) bool {
	return m[i].Priority < m[j].Priority
}

// Implements sort.Interface
func (m metadataList) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}
