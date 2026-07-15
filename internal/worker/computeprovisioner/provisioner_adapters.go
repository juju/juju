// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package computeprovisioner

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5/catacomb"

	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/agentbinary"
	agentbinaryservice "github.com/juju/juju/domain/agentbinary/service"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	provisioning "github.com/juju/juju/domain/provisioner"
	"github.com/juju/juju/environs/config"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

// ControllerConfigService provides access to controller configuration.
type ControllerConfigService interface {
	ControllerConfig(ctx context.Context) (controller.Config, error)
}

// ModelConfigService provides access to model configuration.
type ModelConfigService interface {
	ModelConfig(ctx context.Context) (*config.Config, error)
	Watch(ctx context.Context) (watcher.StringsWatcher, error)
}

// ControllerNodeService provides access to controller node information.
type ControllerNodeService interface {
	GetAllAPIAddressesForAgents(ctx context.Context) ([]string, error)
}

// MachineDomainService provides access to machine domain operations needed
// by the provisioner adapters.
type MachineDomainService interface {
	GetMachineLife(ctx context.Context, machineName coremachine.Name) (life.Value, error)
	AllMachineNames(ctx context.Context) ([]coremachine.Name, error)
	GetInstanceID(ctx context.Context, machineUUID coremachine.UUID) (instance.Id, error)
	WatchModelMachines(ctx context.Context) (watcher.StringsWatcher, error)
	WatchMachineContainerLife(ctx context.Context, parentMachineName coremachine.Name) (watcher.StringsWatcher, error)
	GetMachinePrincipalApplications(ctx context.Context, machineName coremachine.Name) ([]string, error)
	AvailabilityZone(ctx context.Context, machineUUID coremachine.UUID) (string, error)
	ShouldKeepInstance(ctx context.Context, machineName coremachine.Name) (bool, error)
	GetSupportedContainersTypes(ctx context.Context, machineUUID coremachine.UUID) ([]instance.ContainerType, error)
}

// StatusDomainService provides access to status domain operations.
type StatusDomainService interface {
	GetInstanceStatus(ctx context.Context, machineName coremachine.Name) (corestatus.StatusInfo, error)
	SetInstanceStatus(ctx context.Context, machineName coremachine.Name, statusInfo corestatus.StatusInfo) error
	GetMachineStatus(ctx context.Context, machineName coremachine.Name) (corestatus.StatusInfo, error)
	SetMachineStatus(ctx context.Context, machineName coremachine.Name, statusInfo corestatus.StatusInfo) error
}

// ProvisionerDomainService provides access to provisioning domain operations.
type ProvisionerDomainService interface {
	GetPreludeProvisioningInfo(ctx context.Context) (provisioning.SharedProvisioningInfo, error)
	GetProvisioningInfo(ctx context.Context, machineName coremachine.Name, isControllerModel bool, shared provisioning.SharedProvisioningInfo) (provisioning.ProvisioningInfo, error)
}

// AgentBinaryDomainService provides access to agent binary operations.
type AgentBinaryDomainService interface {
	GetEnvironAgentBinariesFinder() agentbinaryservice.EnvironAgentBinariesFinderFunc
	ListAgentBinaries(ctx context.Context) ([]agentbinary.Metadata, error)
}

// ApplicationDomainService provides access to application domain operations.
type ApplicationDomainService interface {
	GetMachinesForApplication(ctx context.Context, appName string) ([]coremachine.Name, error)
}

// RemovalDomainService provides access to removal domain operations.
type RemovalDomainService interface {
	MarkMachineAsDead(ctx context.Context, machineUUID coremachine.UUID) error
	MarkInstanceAsDead(ctx context.Context, machineUUID coremachine.UUID) error
}

// AgentPasswordDomainService provides access to agent password operations.
type AgentPasswordDomainService interface {
	SetMachinePassword(ctx context.Context, machineName coremachine.Name, password string) error
}

// ModelInfoService provides access to model metadata.
type ModelInfoService interface {
	IsControllerModel(ctx context.Context) (bool, error)
}

// controllerAPIAdapter implements ControllerAPI using domain services.
type controllerAPIAdapter struct {
	ctrlConfigSvc  ControllerConfigService
	modelConfigSvc ModelConfigService
	ctrlNodeSvc    ControllerNodeService
	modelUUID      string
}

// ControllerConfig implements ControllerAPI.
func (a *controllerAPIAdapter) ControllerConfig(ctx context.Context) (controller.Config, error) {
	return a.ctrlConfigSvc.ControllerConfig(ctx)
}

// CACert implements ControllerAPI.
func (a *controllerAPIAdapter) CACert(ctx context.Context) (string, error) {
	cfg, err := a.ctrlConfigSvc.ControllerConfig(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	caCert, ok := cfg.CACert()
	if !ok {
		return "", errors.NotFoundf("CA cert")
	}
	return caCert, nil
}

// ModelUUID implements ControllerAPI.
func (a *controllerAPIAdapter) ModelUUID(ctx context.Context) (string, error) {
	return a.modelUUID, nil
}

// ModelConfig implements ControllerAPI.
func (a *controllerAPIAdapter) ModelConfig(ctx context.Context) (*config.Config, error) {
	return a.modelConfigSvc.ModelConfig(ctx)
}

// WatchForModelConfigChanges implements ControllerAPI.
func (a *controllerAPIAdapter) WatchForModelConfigChanges(ctx context.Context) (watcher.NotifyWatcher, error) {
	w, err := a.modelConfigSvc.Watch(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return eventsource.NewStringsNotifyWatcher(w)
}

// APIAddresses implements ControllerAPI.
func (a *controllerAPIAdapter) APIAddresses(ctx context.Context) ([]string, error) {
	return a.ctrlNodeSvc.GetAllAPIAddressesForAgents(ctx)
}

// machinesAPIAdapter implements MachinesAPI using domain services.
type machinesAPIAdapter struct {
	machineSvc       MachineDomainService
	statusSvc        StatusDomainService
	provisionerSvc   ProvisionerDomainService
	ctrlConfigSvc    ControllerConfigService
	modelConfigSvc   ModelConfigService
	modelInfoSvc     ModelInfoService
	agentPasswordSvc AgentPasswordDomainService
	removalSvc       RemovalDomainService
	machineService   MachineService
	modelUUID        string
}

// Machines implements MachinesAPI.
func (a *machinesAPIAdapter) Machines(ctx context.Context, tags ...names.MachineTag) ([]apiprovisioner.MachineResult, error) {
	results := make([]apiprovisioner.MachineResult, len(tags))
	for i, tag := range tags {
		machineName := coremachine.Name(tag.Id())
		machineUUID, err := a.machineService.GetMachineUUID(ctx, machineName)
		if err != nil {
			results[i].Err = convertError(err)
			continue
		}
		machineLife, err := a.machineSvc.GetMachineLife(ctx, machineName)
		if err != nil {
			results[i].Err = convertError(err)
			continue
		}
		results[i].Machine = &machineAdapter{
			tag:              tag,
			machineUUID:      machineUUID,
			machineName:      machineName,
			life:             machineLife,
			machineSvc:       a.machineSvc,
			statusSvc:        a.statusSvc,
			modelConfigSvc:   a.modelConfigSvc,
			agentPasswordSvc: a.agentPasswordSvc,
			removalSvc:       a.removalSvc,
			machineService:   a.machineService,
		}
	}
	return results, nil
}

// MachinesWithTransientErrors implements MachinesAPI.
func (a *machinesAPIAdapter) MachinesWithTransientErrors(ctx context.Context) ([]apiprovisioner.MachineStatusResult, error) {
	machineNames, err := a.machineSvc.AllMachineNames(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []apiprovisioner.MachineStatusResult
	for _, machineName := range machineNames {
		machineUUID, err := a.machineService.GetMachineUUID(ctx, machineName)
		if err != nil {
			continue
		}

		// Skip provisioned machines.
		_, err = a.machineSvc.GetInstanceID(ctx, machineUUID)
		if err == nil {
			continue
		}
		if !errors.Is(err, machineerrors.NotProvisioned) {
			continue
		}

		// Check instance status for transient errors.
		statusInfo, err := a.statusSvc.GetInstanceStatus(ctx, machineName)
		if err != nil {
			continue
		}
		if statusInfo.Status != corestatus.Error && statusInfo.Status != corestatus.ProvisioningError {
			continue
		}
		if transient, ok := statusInfo.Data["transient"].(bool); !ok || !transient {
			continue
		}

		machineLife, err := a.machineSvc.GetMachineLife(ctx, machineName)
		if err != nil {
			continue
		}

		tag := names.NewMachineTag(string(machineName))
		results = append(results, apiprovisioner.MachineStatusResult{
			Machine: &machineAdapter{
				tag:              tag,
				machineUUID:      machineUUID,
				machineName:      machineName,
				life:             machineLife,
				machineSvc:       a.machineSvc,
				statusSvc:        a.statusSvc,
				modelConfigSvc:   a.modelConfigSvc,
				agentPasswordSvc: a.agentPasswordSvc,
				removalSvc:       a.removalSvc,
				machineService:   a.machineService,
			},
			Status: params.StatusResult{
				Id:     string(machineName),
				Life:   machineLife,
				Status: string(statusInfo.Status),
				Info:   statusInfo.Message,
				Data:   statusInfo.Data,
			},
		})
	}
	return results, nil
}

// WatchMachineErrorRetry implements MachinesAPI.
func (a *machinesAPIAdapter) WatchMachineErrorRetry(ctx context.Context) (watcher.NotifyWatcher, error) {
	return newMachineErrorRetryWatcher()
}

// WatchModelMachines implements MachinesAPI.
func (a *machinesAPIAdapter) WatchModelMachines(ctx context.Context) (watcher.StringsWatcher, error) {
	return a.machineSvc.WatchModelMachines(ctx)
}

// ProvisioningInfo implements MachinesAPI.
func (a *machinesAPIAdapter) ProvisioningInfo(ctx context.Context, machineTags []names.MachineTag) (params.ProvisioningInfoResults, error) {
	result := params.ProvisioningInfoResults{
		Results: make([]params.ProvisioningInfoResult, len(machineTags)),
	}

	isControllerModel, err := a.modelInfoSvc.IsControllerModel(ctx)
	if err != nil {
		return result, errors.Annotate(err, "getting model info")
	}

	ctrlConfig, err := a.ctrlConfigSvc.ControllerConfig(ctx)
	if err != nil {
		return result, errors.Annotate(err, "getting controller config")
	}

	shared, err := a.provisionerSvc.GetPreludeProvisioningInfo(ctx)
	if err != nil {
		return result, errors.Annotate(err, "getting shared provisioning info")
	}
	shared.ControllerConfig = ctrlConfig

	for i, tag := range machineTags {
		machineName := coremachine.Name(tag.Id())
		info, err := a.provisionerSvc.GetProvisioningInfo(ctx, machineName, isControllerModel, shared)
		if errors.Is(err, machineerrors.MachineNotFound) {
			result.Results[i].Error = convertError(errors.NotFoundf("machine %s", machineName))
			continue
		}
		if err != nil {
			result.Results[i].Error = convertError(err)
			continue
		}
		pInfo := provisioningInfoToParams(info)
		result.Results[i].Result = &pInfo
	}
	return result, nil
}

// toolsFinderAdapter implements ToolsFinder using domain services.
type toolsFinderAdapter struct {
	agentBinarySvc AgentBinaryDomainService
	ctrlNodeSvc    ControllerNodeService
	modelUUID      string
}

// FindTools implements ToolsFinder.
//
// It mirrors the server-side FindAgents + ToolsURLs logic from
// apiserver/common/tools.go: matched tools from agent-binary storage (or
// simplestreams fallback) are expanded into one entry per API address so the
// cloud-init template can cycle through all addresses on download failure.
func (a *toolsFinderAdapter) FindTools(
	ctx context.Context,
	v semversion.Number,
	osType string,
	arch string,
) (coretools.List, error) {
	var baseList coretools.List

	// Check local agent storage first (same as server-side
	// matchingStorageAgent).
	allMetadata, err := a.agentBinarySvc.ListAgentBinaries(ctx)
	if err == nil {
		var storageList coretools.List
		for _, m := range allMetadata {
			ver, parseErr := semversion.Parse(m.Version)
			if parseErr != nil {
				continue
			}
			binVer := semversion.Binary{
				Number:  ver,
				Arch:    m.Arch,
				Release: osType,
			}
			storageList = append(storageList, &coretools.Tools{
				Version: binVer,
				Size:    m.Size,
				SHA256:  m.SHA256,
			})
		}
		filter := coretools.Filter{
			Number: v,
			OSType: osType,
			Arch:   arch,
		}
		matched, matchErr := storageList.Match(filter)
		if matchErr == nil && len(matched) > 0 {
			baseList = matched
		}
	}

	// Fall back to simplestreams when agent-binary storage has no match.
	if len(baseList) == 0 {
		finder := a.agentBinarySvc.GetEnvironAgentBinariesFinder()
		filter := coretools.Filter{
			Number: v,
			OSType: osType,
			Arch:   arch,
		}
		streamList, streamErr := finder(ctx, v.Major, v.Minor, v, "", filter)
		if streamErr != nil {
			return nil, errors.Trace(streamErr)
		}
		baseList = streamList
	}

	if len(baseList) == 0 {
		return nil, coretools.ErrNoMatches
	}

	// Rewrite URLs to point at the API server, producing one entry per
	// (tool, address) pair — same as FindAgents -> ToolsURLs on the server
	// side. This ensures the cloud-init download script cycles through all
	// API addresses and can fall back from unreachable private IPs to public
	// ones.
	addrs, addrErr := a.ctrlNodeSvc.GetAllAPIAddressesForAgents(ctx)
	if addrErr != nil {
		return nil, errors.Annotate(addrErr, "getting API addresses for tools URLs")
	}
	if len(addrs) == 0 {
		return nil, errors.New("no suitable API server address to pick from")
	}

	var fullList coretools.List
	for _, baseTools := range baseList {
		for _, addr := range addrs {
			tools := *baseTools // copy — don't mutate the shared pointer
			serverRoot := fmt.Sprintf("https://%s/model/%s", addr, a.modelUUID)
			tools.URL = fmt.Sprintf("%s/tools/%s", serverRoot, tools.Version.String())
			fullList = append(fullList, &tools)
		}
	}
	return fullList, nil
}

// distributionGroupFinderAdapter implements DistributionGroupFinder using
// domain services.
type distributionGroupFinderAdapter struct {
	machineSvc     MachineDomainService
	appSvc         ApplicationDomainService
	machineService MachineService
}

// DistributionGroupByMachineId implements DistributionGroupFinder.
func (a *distributionGroupFinderAdapter) DistributionGroupByMachineId(ctx context.Context, tags ...names.MachineTag) ([]apiprovisioner.DistributionGroupResult, error) {
	results := make([]apiprovisioner.DistributionGroupResult, len(tags))
	for i, tag := range tags {
		machineName := coremachine.Name(tag.Id())
		machineIds, err := a.commonApplicationMachineIds(ctx, machineName)
		if err != nil {
			results[i].Err = convertError(err)
			continue
		}
		results[i].MachineIds = machineIds
	}
	return results, nil
}

// commonApplicationMachineIds returns machine IDs that share at least one
// principal application with the given machine.
func (a *distributionGroupFinderAdapter) commonApplicationMachineIds(ctx context.Context, machineName coremachine.Name) ([]string, error) {
	applications, err := a.machineSvc.GetMachinePrincipalApplications(ctx, machineName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	machineIdSet := make(map[string]struct{})
	for _, app := range applications {
		machines, err := a.appSvc.GetMachinesForApplication(ctx, app)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, m := range machines {
			if m != machineName {
				machineIdSet[string(m)] = struct{}{}
			}
		}
	}

	result := make([]string, 0, len(machineIdSet))
	for id := range machineIdSet {
		result = append(result, id)
	}
	sort.Strings(result)
	return result, nil
}

// machineAdapter implements MachineProvisioner using domain services.
type machineAdapter struct {
	tag              names.MachineTag
	machineUUID      coremachine.UUID
	machineName      coremachine.Name
	life             life.Value
	machineSvc       MachineDomainService
	statusSvc        StatusDomainService
	modelConfigSvc   ModelConfigService
	agentPasswordSvc AgentPasswordDomainService
	removalSvc       RemovalDomainService
	machineService   MachineService
}

// Tag implements MachineProvisioner.
func (m *machineAdapter) Tag() names.Tag {
	return m.tag
}

// ModelAgentVersion implements MachineProvisioner.
func (m *machineAdapter) ModelAgentVersion(ctx context.Context) (*semversion.Number, error) {
	mc, err := m.modelConfigSvc.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if v, ok := mc.AgentVersion(); ok {
		return &v, nil
	}
	return nil, errors.New("model has no agent version configured")
}

// MachineTag implements MachineProvisioner.
func (m *machineAdapter) MachineTag() names.MachineTag {
	return m.tag
}

// Id implements MachineProvisioner.
func (m *machineAdapter) Id() string {
	return m.tag.Id()
}

// String implements MachineProvisioner.
func (m *machineAdapter) String() string {
	return m.Id()
}

// Life implements MachineProvisioner.
func (m *machineAdapter) Life() life.Value {
	return m.life
}

// Refresh implements MachineProvisioner.
func (m *machineAdapter) Refresh(ctx context.Context) error {
	machineLife, err := m.machineSvc.GetMachineLife(ctx, m.machineName)
	if err != nil {
		return errors.Trace(err)
	}
	m.life = machineLife
	return nil
}

// SetInstanceStatus implements MachineProvisioner.
func (m *machineAdapter) SetInstanceStatus(ctx context.Context, status corestatus.Status, message string, data map[string]any) error {
	statusInfo := corestatus.StatusInfo{
		Status:  status,
		Message: message,
		Data:    data,
	}
	if err := m.statusSvc.SetInstanceStatus(ctx, m.machineName, statusInfo); err != nil {
		return err
	}
	if status != corestatus.ProvisioningError && status != corestatus.Error {
		return nil
	}
	statusInfo.Status = corestatus.Error
	return m.statusSvc.SetMachineStatus(ctx, m.machineName, statusInfo)
}

// InstanceStatus implements MachineProvisioner.
func (m *machineAdapter) InstanceStatus(ctx context.Context) (corestatus.Status, string, error) {
	statusInfo, err := m.statusSvc.GetInstanceStatus(ctx, m.machineName)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	return statusInfo.Status, statusInfo.Message, nil
}

// SetStatus implements MachineProvisioner.
func (m *machineAdapter) SetStatus(ctx context.Context, status corestatus.Status, info string, data map[string]any) error {
	return m.statusSvc.SetMachineStatus(ctx, m.machineName, corestatus.StatusInfo{
		Status:  status,
		Message: info,
		Data:    data,
	})
}

// Status implements MachineProvisioner.
func (m *machineAdapter) Status(ctx context.Context) (corestatus.Status, string, error) {
	statusInfo, err := m.statusSvc.GetMachineStatus(ctx, m.machineName)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	return statusInfo.Status, statusInfo.Message, nil
}

// EnsureDead implements MachineProvisioner.
func (m *machineAdapter) EnsureDead(ctx context.Context) error {
	return m.removalSvc.MarkMachineAsDead(ctx, m.machineUUID)
}

// MarkForRemoval implements MachineProvisioner.
func (m *machineAdapter) MarkForRemoval(ctx context.Context) error {
	return m.removalSvc.MarkInstanceAsDead(ctx, m.machineUUID)
}

// AvailabilityZone implements MachineProvisioner.
func (m *machineAdapter) AvailabilityZone(ctx context.Context) (string, error) {
	return m.machineSvc.AvailabilityZone(ctx, m.machineUUID)
}

// DistributionGroup implements MachineProvisioner.
func (m *machineAdapter) DistributionGroup(ctx context.Context) ([]instance.Id, error) {
	return nil, nil
}

// SetInstanceInfo implements MachineProvisioner.
func (m *machineAdapter) SetInstanceInfo(
	ctx context.Context,
	id instance.Id,
	displayName string,
	nonce string,
	characteristics *instance.HardwareCharacteristics,
	networkConfig []params.NetworkConfig,
	volumes []params.Volume,
	volumeAttachments map[string]params.VolumeAttachmentInfo,
) error {
	return m.machineService.SetMachineCloudInstance(ctx, m.machineUUID, id, displayName, nonce, characteristics)
}

// InstanceId implements MachineProvisioner.
func (m *machineAdapter) InstanceId(ctx context.Context) (instance.Id, error) {
	id, err := m.machineSvc.GetInstanceID(ctx, m.machineUUID)
	if errors.Is(err, machineerrors.NotProvisioned) {
		return "", &params.Error{
			Code:    params.CodeNotProvisioned,
			Message: err.Error(),
		}
	}
	return id, err
}

// KeepInstance implements MachineProvisioner.
func (m *machineAdapter) KeepInstance(ctx context.Context) (bool, error) {
	return m.machineSvc.ShouldKeepInstance(ctx, m.machineName)
}

// SetPassword implements MachineProvisioner.
func (m *machineAdapter) SetPassword(ctx context.Context, password string) error {
	return m.agentPasswordSvc.SetMachinePassword(ctx, m.machineName, password)
}

// WatchContainers implements MachineProvisioner.
func (m *machineAdapter) WatchContainers(ctx context.Context, ctype instance.ContainerType) (watcher.StringsWatcher, error) {
	return m.machineSvc.WatchMachineContainerLife(ctx, m.machineName)
}

// SetSupportedContainers implements MachineProvisioner.
func (m *machineAdapter) SetSupportedContainers(ctx context.Context, containerTypes ...instance.ContainerType) error {
	return nil
}

// SupportsNoContainers implements MachineProvisioner.
func (m *machineAdapter) SupportsNoContainers(ctx context.Context) error {
	return nil
}

// SupportedContainers implements MachineProvisioner.
func (m *machineAdapter) SupportedContainers(ctx context.Context) ([]instance.ContainerType, bool, error) {
	types, err := m.machineSvc.GetSupportedContainersTypes(ctx, m.machineUUID)
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	return types, true, nil
}

// machineErrorRetryWatcher is a polling watcher that fires every minute to
// signal that machines with transient errors should be retried. It matches
// the server-side implementation in
// apiserver/facades/agent/provisioner/machineerror.go.
type machineErrorRetryWatcher struct {
	catacomb catacomb.Catacomb
	out      chan struct{}
}

func newMachineErrorRetryWatcher() (watcher.NotifyWatcher, error) {
	w := &machineErrorRetryWatcher{
		out: make(chan struct{}),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "machine-error-retry",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func (w *machineErrorRetryWatcher) loop() error {
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-time.After(time.Minute):
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.out <- struct{}{}:
			}
		}
	}
}

func (w *machineErrorRetryWatcher) Kill() {
	w.catacomb.Kill(nil)
}

func (w *machineErrorRetryWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *machineErrorRetryWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *machineErrorRetryWatcher) Err() error {
	return w.catacomb.Err()
}

func (w *machineErrorRetryWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

// provisioningInfoToParams converts domain provisioning info to API params.
// This mirrors the server-side implementation in
// apiserver/facades/agent/provisioner/provisioninginfo.go.
func provisioningInfoToParams(info provisioning.ProvisioningInfo) params.ProvisioningInfo {
	result := params.ProvisioningInfo{
		Base: params.Base{
			Name:    info.Base.OS,
			Channel: info.Base.Channel.String(),
		},
		Constraints:       info.Constraints,
		Placement:         unptr(info.PlacementDirective),
		Jobs:              info.Jobs,
		Tags:              info.Tags,
		EndpointBindings:  info.EndpointBindings,
		CloudInitUserData: info.CloudInitUserData,
		ControllerConfig:  info.ControllerConfig,
	}

	if len(info.Volumes) > 0 {
		result.Volumes = make([]params.VolumeParams, len(info.Volumes))
		for i, v := range info.Volumes {
			result.Volumes[i] = volumeParamsToParams(v)
		}
	}

	if len(info.VolumeAttachments) > 0 {
		result.VolumeAttachments = make([]params.VolumeAttachmentParams, len(info.VolumeAttachments))
		for i, va := range info.VolumeAttachments {
			result.VolumeAttachments[i] = volumeAttachmentParamsToParams(va)
		}
	}

	if info.RootDisk != nil {
		rd := volumeParamsToParams(*info.RootDisk)
		result.RootDisk = &rd
	}

	if len(info.ImageMetadata) > 0 {
		result.ImageMetadata = make([]params.CloudImageMetadata, len(info.ImageMetadata))
		for i, m := range info.ImageMetadata {
			result.ImageMetadata[i] = params.CloudImageMetadata{
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
	}

	if info.SpaceSubnets != nil || info.SubnetAZs != nil {
		result.ProvisioningNetworkTopology = params.ProvisioningNetworkTopology{
			SubnetAZs:    info.SubnetAZs,
			SpaceSubnets: info.SpaceSubnets,
		}
	}

	return result
}

func volumeParamsToParams(v provisioning.VolumeParams) params.VolumeParams {
	p := params.VolumeParams{
		SizeMiB:    v.SizeMiB,
		Provider:   v.Provider,
		Attributes: v.Attributes,
		Tags:       v.Tags,
	}
	if v.VolumeID != "" {
		p.VolumeTag = names.NewVolumeTag(v.VolumeID).String()
	}
	if v.Attachment != nil {
		att := volumeAttachmentParamsToParams(*v.Attachment)
		p.Attachment = &att
	}
	return p
}

func volumeAttachmentParamsToParams(va provisioning.VolumeAttachmentParams) params.VolumeAttachmentParams {
	p := params.VolumeAttachmentParams{
		Provider:   va.Provider,
		ReadOnly:   va.ReadOnly,
		ProviderId: va.ProviderID,
	}
	if va.VolumeID != "" {
		p.VolumeTag = names.NewVolumeTag(va.VolumeID).String()
	}
	if va.MachineID != "" {
		p.MachineTag = names.NewMachineTag(va.MachineID).String()
	}
	return p
}

func unptr[T any](ptr *T) T {
	var zero T
	if ptr == nil {
		return zero
	}
	return *ptr
}

// convertError converts a Go error into a *params.Error for API results.
func convertError(err error) *params.Error {
	if err == nil {
		return nil
	}
	paramsErr := &params.Error{
		Message: err.Error(),
	}
	switch {
	case errors.Is(err, machineerrors.MachineNotFound):
		paramsErr.Code = params.CodeNotFound
	case errors.Is(err, errors.NotFound):
		paramsErr.Code = params.CodeNotFound
	case errors.Is(err, errors.NotProvisioned):
		paramsErr.Code = params.CodeNotProvisioned
	case errors.Is(err, errors.Unauthorized):
		paramsErr.Code = params.CodeUnauthorized
	case errors.Is(err, errors.NotSupported):
		paramsErr.Code = params.CodeNotSupported
	case errors.Is(err, errors.NotImplemented):
		paramsErr.Code = params.CodeNotImplemented
	case errors.Is(err, errors.AlreadyExists):
		paramsErr.Code = params.CodeAlreadyExists
	case errors.Is(err, errors.NotValid):
		paramsErr.Code = params.CodeNotValid
	}
	return paramsErr
}
