// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import (
	"github.com/juju/errors"
	"github.com/kr/pretty"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

// srvAllWatcher defines the API methods on a state.Multiwatcher.
// which watches any changes to the state. Each client has its own
// current set of watchers, stored in resources. It is used by both
// the AllWatcher and AllModelWatcher facades.
type srvAllWatcher struct {
	watcherCommon
	watcher multiwatcher.Watcher

	deltaTranslater DeltaTranslater
}

func newAllWatcher(context facade.Context, deltaTranslater DeltaTranslater) (*srvAllWatcher, error) {
	var (
		id              = context.ID()
		auth            = context.Auth()
		watcherRegistry = context.WatcherRegistry()
	)

	if !auth.AuthClient() {
		// Note that we don't need to check specific permissions
		// here, as the AllWatcher can only do anything if the
		// watcher resource has already been created, so we can
		// rely on the permission check there to ensure that
		// this facade can't do anything it shouldn't be allowed
		// to.
		//
		// This is useful because the AllWatcher is reused for
		// both the WatchAll (requires model access rights) and
		// the WatchAllModels (requring controller superuser
		// rights) API calls.
		return nil, apiservererrors.ErrPerm
	}

	watcher, err := watcherRegistry.Get(id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	multiwatcher, ok := watcher.(multiwatcher.Watcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvAllWatcher{
		watcherCommon:   newWatcherCommon(context),
		watcher:         multiwatcher,
		deltaTranslater: deltaTranslater,
	}, nil
}

// NewAllWatcher returns a new API server endpoint for interacting
// with a watcher created by the WatchAll and WatchAllModels API calls.
func NewAllWatcher(context facade.Context) (facade.Facade, error) {
	return newAllWatcher(context, newAllWatcherDeltaTranslater())
}

// Next will return the current state of everything on the first call
// and subsequent calls will
func (aw *srvAllWatcher) Next() (params.AllWatcherNextResults, error) {
	deltas, err := aw.watcher.Next()
	return params.AllWatcherNextResults{
		Deltas: translate(aw.deltaTranslater, deltas),
	}, err
}

type allWatcherDeltaTranslater struct {
	DeltaTranslater
}

func newAllWatcherDeltaTranslater() DeltaTranslater {
	return &allWatcherDeltaTranslater{}
}

// DeltaTranslater defines methods for translating multiwatcher.EntityInfo to params.EntityInfo.
type DeltaTranslater interface {
	TranslateModel(multiwatcher.EntityInfo) params.EntityInfo
	TranslateApplication(multiwatcher.EntityInfo) params.EntityInfo
	TranslateRemoteApplication(multiwatcher.EntityInfo) params.EntityInfo
	TranslateMachine(multiwatcher.EntityInfo) params.EntityInfo
	TranslateUnit(multiwatcher.EntityInfo) params.EntityInfo
	TranslateCharm(multiwatcher.EntityInfo) params.EntityInfo
	TranslateRelation(multiwatcher.EntityInfo) params.EntityInfo
	TranslateBranch(multiwatcher.EntityInfo) params.EntityInfo
	TranslateAnnotation(multiwatcher.EntityInfo) params.EntityInfo
	TranslateBlock(multiwatcher.EntityInfo) params.EntityInfo
	TranslateAction(multiwatcher.EntityInfo) params.EntityInfo
	TranslateApplicationOffer(multiwatcher.EntityInfo) params.EntityInfo
}

func translate(dt DeltaTranslater, deltas []multiwatcher.Delta) []params.Delta {
	response := make([]params.Delta, 0, len(deltas))
	for _, delta := range deltas {
		id := delta.Entity.EntityID()
		var converted params.EntityInfo
		switch id.Kind {
		case multiwatcher.ModelKind:
			converted = dt.TranslateModel(delta.Entity)
		case multiwatcher.ApplicationKind:
			converted = dt.TranslateApplication(delta.Entity)
		case multiwatcher.RemoteApplicationKind:
			converted = dt.TranslateRemoteApplication(delta.Entity)
		case multiwatcher.MachineKind:
			converted = dt.TranslateMachine(delta.Entity)
		case multiwatcher.UnitKind:
			converted = dt.TranslateUnit(delta.Entity)
		case multiwatcher.CharmKind:
			converted = dt.TranslateCharm(delta.Entity)
		case multiwatcher.RelationKind:
			converted = dt.TranslateRelation(delta.Entity)
		case multiwatcher.BranchKind:
			converted = dt.TranslateBranch(delta.Entity)
		case multiwatcher.AnnotationKind: // THIS SEEMS WEIRD
			// FIXME: annotations should be part of the underlying entity.
			converted = dt.TranslateAnnotation(delta.Entity)
		case multiwatcher.BlockKind:
			converted = dt.TranslateBlock(delta.Entity)
		case multiwatcher.ActionKind:
			converted = dt.TranslateAction(delta.Entity)
		case multiwatcher.ApplicationOfferKind:
			converted = dt.TranslateApplicationOffer(delta.Entity)
		default:
			// converted stays nil
		}
		// It is possible that there are some multiwatcher elements that are
		// internal, and not exposed outside the controller.
		// Also this is a key place to start versioning the all watchers.
		if converted != nil {
			response = append(response, params.Delta{
				Removed: delta.Removed,
				Entity:  converted})
		}
	}
	return response
}

func (aw allWatcherDeltaTranslater) TranslateModel(info multiwatcher.EntityInfo) params.EntityInfo {
	orig, ok := info.(*multiwatcher.ModelInfo)
	if !ok {
		logger.Criticalf("consistency error: %s", pretty.Sprint(info))
		return nil
	}

	var version string
	if cfg, err := config.New(config.NoDefaults, orig.Config); err == nil {
		versionNumber, _ := cfg.AgentVersion()
		version = versionNumber.String()
	}

	return &params.ModelUpdate{
		ModelUUID:      orig.ModelUUID,
		Name:           orig.Name,
		Life:           orig.Life,
		Owner:          orig.Owner,
		ControllerUUID: orig.ControllerUUID,
		IsController:   orig.IsController,
		Config:         orig.Config,
		Status:         aw.translateStatus(orig.Status),
		Constraints:    orig.Constraints,
		SLA: params.ModelSLAInfo{
			Level: orig.SLA.Level,
			Owner: orig.SLA.Owner,
		},
		Type:        orig.Type.String(),
		Cloud:       orig.Cloud,
		CloudRegion: orig.CloudRegion,
		Version:     version,
	}
}

func (aw allWatcherDeltaTranslater) translateStatus(info multiwatcher.StatusInfo) params.StatusInfo {
	return params.StatusInfo{
		Err:     info.Err, // CHECK THIS
		Current: info.Current,
		Message: info.Message,
		Since:   info.Since,
		Version: info.Version,
		Data:    info.Data,
	}
}

func (aw allWatcherDeltaTranslater) TranslateApplication(info multiwatcher.EntityInfo) params.EntityInfo {
	orig, ok := info.(*multiwatcher.ApplicationInfo)
	if !ok {
		logger.Criticalf("consistency error: %s", pretty.Sprint(info))
		return nil
	}

	// If the application status is unset, then set it to unknown. It is
	// expected that downstream clients (model cache, pylibjuju, jslibjuju)
	// correctly interpret the unknown status from the unit status. If the unit
	// status is not found, then fall back to unknown.
	// If a charm author has set the application status, then show that instead.
	applicationStatus := multiwatcher.StatusInfo{Current: status.Unset}
	if orig.Status.Current != status.Unset {
		applicationStatus = orig.Status
	}

	return &params.ApplicationInfo{
		ModelUUID:       orig.ModelUUID,
		Name:            orig.Name,
		Exposed:         orig.Exposed,
		CharmURL:        orig.CharmURL,
		OwnerTag:        orig.OwnerTag,
		Life:            orig.Life,
		MinUnits:        orig.MinUnits,
		Constraints:     orig.Constraints,
		Config:          orig.Config,
		Subordinate:     orig.Subordinate,
		Status:          aw.translateStatus(applicationStatus),
		WorkloadVersion: orig.WorkloadVersion,
	}
}

func (aw allWatcherDeltaTranslater) TranslateMachine(info multiwatcher.EntityInfo) params.EntityInfo {
	orig, ok := info.(*multiwatcher.MachineInfo)
	if !ok {
		logger.Criticalf("consistency error: %s", pretty.Sprint(info))
		return nil
	}
	return &params.MachineInfo{
		ModelUUID:                orig.ModelUUID,
		Id:                       orig.ID,
		InstanceId:               orig.InstanceID,
		AgentStatus:              aw.translateStatus(orig.AgentStatus),
		InstanceStatus:           aw.translateStatus(orig.InstanceStatus),
		Life:                     orig.Life,
		Config:                   orig.Config,
		Base:                     orig.Base,
		ContainerType:            orig.ContainerType,
		IsManual:                 orig.IsManual,
		SupportedContainers:      orig.SupportedContainers,
		SupportedContainersKnown: orig.SupportedContainersKnown,
		HardwareCharacteristics:  orig.HardwareCharacteristics,
		CharmProfiles:            orig.CharmProfiles,
		Jobs:                     orig.Jobs,
		Addresses:                aw.translateAddresses(orig.Addresses),
		HasVote:                  orig.HasVote,
		WantsVote:                orig.WantsVote,
		Hostname:                 orig.Hostname,
	}
}

func (aw allWatcherDeltaTranslater) translateAddresses(addresses []network.ProviderAddress) []params.Address {
	if addresses == nil {
		return nil
	}
	result := make([]params.Address, 0, len(addresses))
	for _, address := range addresses {
		result = append(result, params.Address{
			Value:           address.Value,
			Type:            string(address.Type),
			Scope:           string(address.Scope),
			SpaceName:       string(address.SpaceName),
			ProviderSpaceID: string(address.ProviderSpaceID),
		})
	}
	return result
}

func (aw allWatcherDeltaTranslater) TranslateCharm(info multiwatcher.EntityInfo) params.EntityInfo {
	orig, ok := info.(*multiwatcher.CharmInfo)
	if !ok {
		logger.Criticalf("consistency error: %s", pretty.Sprint(info))
		return nil
	}
	return &params.CharmInfo{
		ModelUUID:     orig.ModelUUID,
		CharmURL:      orig.CharmURL,
		CharmVersion:  orig.CharmVersion,
		Life:          orig.Life,
		LXDProfile:    aw.translateProfile(orig.LXDProfile),
		DefaultConfig: orig.DefaultConfig,
	}
}

func (aw allWatcherDeltaTranslater) translateProfile(profile *multiwatcher.Profile) *params.Profile {
	if profile == nil {
		return nil
	}
	return &params.Profile{
		Config:      profile.Config,
		Description: profile.Description,
		Devices:     profile.Devices,
	}
}

func (aw allWatcherDeltaTranslater) TranslateRemoteApplication(info multiwatcher.EntityInfo) params.EntityInfo {
	orig, ok := info.(*multiwatcher.RemoteApplicationUpdate)
	if !ok {
		logger.Criticalf("consistency error: %s", pretty.Sprint(info))
		return nil
	}
	return &params.RemoteApplicationUpdate{
		ModelUUID: orig.ModelUUID,
		Name:      orig.Name,
		OfferURL:  orig.OfferURL,
		Life:      orig.Life,
		Status:    aw.translateStatus(orig.Status),
	}
}

func (aw allWatcherDeltaTranslater) TranslateApplicationOffer(info multiwatcher.EntityInfo) params.EntityInfo {
	orig, ok := info.(*multiwatcher.ApplicationOfferInfo)
	if !ok {
		logger.Criticalf("consistency error: %s", pretty.Sprint(info))
		return nil
	}
	return &params.ApplicationOfferInfo{
		ModelUUID:            orig.ModelUUID,
		OfferName:            orig.OfferName,
		OfferUUID:            orig.OfferUUID,
		ApplicationName:      orig.ApplicationName,
		CharmName:            orig.CharmName,
		TotalConnectedCount:  orig.TotalConnectedCount,
		ActiveConnectedCount: orig.ActiveConnectedCount,
	}
}

func (aw allWatcherDeltaTranslater) TranslateUnit(info multiwatcher.EntityInfo) params.EntityInfo {
	orig, ok := info.(*multiwatcher.UnitInfo)
	if !ok {
		logger.Criticalf("consistency error: %s", pretty.Sprint(info))
		return nil
	}

	translatedPortRanges := aw.translatePortRanges(orig.OpenPortRangesByEndpoint)

	return &params.UnitInfo{
		ModelUUID:      orig.ModelUUID,
		Name:           orig.Name,
		Application:    orig.Application,
		Base:           orig.Base,
		CharmURL:       orig.CharmURL,
		Life:           orig.Life,
		PublicAddress:  orig.PublicAddress,
		PrivateAddress: orig.PrivateAddress,
		MachineId:      orig.MachineID,
		Ports:          aw.mapRangesIntoPorts(translatedPortRanges),
		PortRanges:     translatedPortRanges,
		Principal:      orig.Principal,
		Subordinate:    orig.Subordinate,
		WorkloadStatus: aw.translateStatus(orig.WorkloadStatus),
		AgentStatus:    aw.translateStatus(orig.AgentStatus),
	}
}

func (aw allWatcherDeltaTranslater) mapRangesIntoPorts(portRanges []params.PortRange) []params.Port {
	if portRanges == nil {
		return nil
	}
	result := make([]params.Port, 0, len(portRanges))
	for _, pr := range portRanges {
		for portNum := pr.FromPort; portNum <= pr.ToPort; portNum++ {
			result = append(result, params.Port{
				Protocol: pr.Protocol,
				Number:   portNum,
			})
		}
	}
	return result
}

// translatePortRanges flattens a set of port ranges grouped by endpoint into
// a list of unique port ranges. This method ignores subnet IDs and is provided
// for backwards compatibility with pre 2.9 clients that assume that open-ports
// applies to all subnets.
func (aw allWatcherDeltaTranslater) translatePortRanges(portsByEndpoint network.GroupedPortRanges) []params.PortRange {
	if portsByEndpoint == nil {
		return nil
	}

	uniquePortRanges := portsByEndpoint.UniquePortRanges()
	network.SortPortRanges(uniquePortRanges)

	result := make([]params.PortRange, len(uniquePortRanges))
	for i, pr := range uniquePortRanges {
		result[i] = params.FromNetworkPortRange(pr)
	}

	return result
}

func (aw allWatcherDeltaTranslater) TranslateAction(info multiwatcher.EntityInfo) params.EntityInfo {
	orig, ok := info.(*multiwatcher.ActionInfo)
	if !ok {
		logger.Criticalf("consistency error: %s", pretty.Sprint(info))
		return nil
	}
	return &params.ActionInfo{
		ModelUUID: orig.ModelUUID,
		Id:        orig.ID,
		Receiver:  orig.Receiver,
		Name:      orig.Name,
		Status:    orig.Status,
		Message:   orig.Message,
		Enqueued:  orig.Enqueued,
		Started:   orig.Started,
		Completed: orig.Completed,
	}
}

func (aw allWatcherDeltaTranslater) TranslateRelation(info multiwatcher.EntityInfo) params.EntityInfo {
	orig, ok := info.(*multiwatcher.RelationInfo)
	if !ok {
		logger.Criticalf("consistency error: %s", pretty.Sprint(info))
		return nil
	}
	return &params.RelationInfo{
		ModelUUID: orig.ModelUUID,
		Key:       orig.Key,
		Id:        orig.ID,
		Endpoints: aw.translateEndpoints(orig.Endpoints),
	}
}

func (aw allWatcherDeltaTranslater) translateEndpoints(eps []multiwatcher.Endpoint) []params.Endpoint {
	if eps == nil {
		return nil
	}
	result := make([]params.Endpoint, 0, len(eps))
	for _, ep := range eps {
		result = append(result, params.Endpoint{
			ApplicationName: ep.ApplicationName,
			Relation: params.CharmRelation{
				Name:      ep.Relation.Name,
				Role:      ep.Relation.Role,
				Interface: ep.Relation.Interface,
				Optional:  ep.Relation.Optional,
				Limit:     ep.Relation.Limit,
				Scope:     ep.Relation.Scope,
			},
		})
	}
	return result
}

func (aw allWatcherDeltaTranslater) TranslateAnnotation(info multiwatcher.EntityInfo) params.EntityInfo {
	orig, ok := info.(*multiwatcher.AnnotationInfo)
	if !ok {
		logger.Criticalf("consistency error: %s", pretty.Sprint(info))
		return nil
	}
	return &params.AnnotationInfo{
		ModelUUID:   orig.ModelUUID,
		Tag:         orig.Tag,
		Annotations: orig.Annotations,
	}
}

func (aw allWatcherDeltaTranslater) TranslateBlock(info multiwatcher.EntityInfo) params.EntityInfo {
	orig, ok := info.(*multiwatcher.BlockInfo)
	if !ok {
		logger.Criticalf("consistency error: %s", pretty.Sprint(info))
		return nil
	}
	return &params.BlockInfo{
		ModelUUID: orig.ModelUUID,
		Id:        orig.ID,
		Type:      orig.Type,
		Message:   orig.Message,
		Tag:       orig.Tag,
	}
}

func (aw allWatcherDeltaTranslater) TranslateBranch(info multiwatcher.EntityInfo) params.EntityInfo {
	orig, ok := info.(*multiwatcher.BranchInfo)
	if !ok {
		logger.Criticalf("consistency error: %s", pretty.Sprint(info))
		return nil
	}
	return &params.BranchInfo{
		ModelUUID:     orig.ModelUUID,
		Id:            orig.ID,
		Name:          orig.Name,
		AssignedUnits: orig.AssignedUnits,
		Config:        aw.translateBranchConfig(orig.Config),
		Created:       orig.Created,
		CreatedBy:     orig.CreatedBy,
		Completed:     orig.Completed,
		CompletedBy:   orig.CompletedBy,
		GenerationId:  orig.GenerationID,
	}
}

func (aw allWatcherDeltaTranslater) translateBranchConfig(config map[string][]multiwatcher.ItemChange) map[string][]params.ItemChange {
	if config == nil {
		return nil
	}
	result := make(map[string][]params.ItemChange)
	for key, value := range config {
		if value == nil {
			result[key] = nil
		} else {
			changes := make([]params.ItemChange, 0, len(value))
			for _, change := range value {
				changes = append(changes, params.ItemChange{
					Type:     change.Type,
					Key:      change.Key,
					OldValue: change.OldValue,
					NewValue: change.NewValue,
				})
			}
			result[key] = changes
		}
	}
	return result
}
