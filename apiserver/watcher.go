// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/errors"
	"github.com/kr/pretty"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelrelations"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/state"
)

type watcherCommon struct {
	id         string
	resources  facade.Resources
	dispose    func()
	controller *cache.Controller
}

func newWatcherCommon(context facade.Context) watcherCommon {
	return watcherCommon{
		context.ID(),
		context.Resources(),
		context.Dispose,
		context.Controller(),
	}
}

// Stop stops the watcher.
func (w *watcherCommon) Stop() error {
	w.dispose()
	return w.resources.Stop(w.id)
}

// SrvAllWatcherV1 defines the API methods on a state.Multiwatcher.
type SrvAllWatcherV1 struct {
	*SrvAllWatcher
}

// SrvAllWatcher defines the API methods on a state.Multiwatcher.
// which watches any changes to the state. Each client has its own
// current set of watchers, stored in resources. It is used by both
// the AllWatcher and AllModelWatcher facades.
type SrvAllWatcher struct {
	watcherCommon
	watcher multiwatcher.Watcher

	deltaTranslater DeltaTranslater
}

// NewAllWatcherV1 is a wrapper that creates a V1 AllWatcher API.
func NewAllWatcherV1(context facade.Context) (facade.Facade, error) {
	api, err := newAllWatcher(context, newAllWatcherDeltaTranslaterV1())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &SrvAllWatcherV1{api}, nil
}

// Next will return the current state of everything on the first call
// and subsequent calls will
func (aw *SrvAllWatcherV1) Next() (params.AllWatcherNextResults, error) {
	deltas, err := aw.watcher.Next()
	return params.AllWatcherNextResults{
		Deltas: translate(aw.deltaTranslater, deltas),
	}, err
}

type allWatcherDeltaTranslaterV1 struct {
	DeltaTranslater
}

func newAllWatcherDeltaTranslaterV1() DeltaTranslater {
	return &allWatcherDeltaTranslaterV1{
		newAllWatcherDeltaTranslater(),
	}
}

func (aw allWatcherDeltaTranslaterV1) TranslateAction(info multiwatcher.EntityInfo) params.EntityInfo {
	orig, ok := info.(*multiwatcher.ActionInfo)
	if !ok {
		logger.Criticalf("consistency error: %s", pretty.Sprint(info))
		return nil
	}
	return &params.ActionInfo{
		ModelUUID:  orig.ModelUUID,
		Id:         orig.ID,
		Receiver:   orig.Receiver,
		Name:       orig.Name,
		Parameters: orig.Parameters,
		Status:     orig.Status,
		Message:    orig.Message,
		Results:    orig.Results,
		Enqueued:   orig.Enqueued,
		Started:    orig.Started,
		Completed:  orig.Completed,
	}
}

func newAllWatcher(context facade.Context, deltaTranslater DeltaTranslater) (*SrvAllWatcher, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()

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
	watcher, ok := resources.Get(id).(multiwatcher.Watcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &SrvAllWatcher{
		watcherCommon:   newWatcherCommon(context),
		watcher:         watcher,
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
func (aw *SrvAllWatcher) Next() (params.AllWatcherNextResults, error) {
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
		Series:                   orig.Series,
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
		OfferUUID: orig.OfferUUID,
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
		Series:         orig.Series,
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

func isAgent(auth facade.Authorizer) bool {
	return auth.AuthMachineAgent() || auth.AuthUnitAgent() || auth.AuthApplicationAgent() || auth.AuthModelAgent()
}

func isAgentOrUser(auth facade.Authorizer) bool {
	return isAgent(auth) || auth.AuthClient()
}

func newNotifyWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()

	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgentOrUser(auth) {
		return nil, apiservererrors.ErrPerm
	}

	watcher, ok := resources.Get(id).(cache.NotifyWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}

	return &srvNotifyWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       watcher,
	}, nil
}

// srvNotifyWatcher defines the API access to methods on a NotifyWatcher.
// Each client has its own current set of watchers, stored in resources.
type srvNotifyWatcher struct {
	watcherCommon
	watcher cache.NotifyWatcher
}

// state watchers have an Err method, but cache watchers do not.
type hasErr interface {
	Err() error
}

// Next returns when a change has occurred to the
// entity being watched since the most recent call to Next
// or the Watch call that created the NotifyWatcher.
func (w *srvNotifyWatcher) Next() error {
	if _, ok := <-w.watcher.Changes(); ok {
		return nil
	}

	var err error
	if e, ok := w.watcher.(hasErr); ok {
		err = e.Err()
	}
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return err
}

// srvStringsWatcher defines the API for methods on a state.StringsWatcher.
// Each client has its own current set of watchers, stored in resources.
// srvStringsWatcher notifies about changes for all entities of a given kind,
// sending the changes as a list of strings.
type srvStringsWatcher struct {
	watcherCommon
	watcher cache.StringsWatcher
}

func newStringsWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()

	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgentOrUser(auth) {
		return nil, apiservererrors.ErrPerm
	}
	watcher, ok := resources.Get(id).(cache.StringsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvStringsWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       watcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvStringsWatcher.
func (w *srvStringsWatcher) Next() (params.StringsWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		return params.StringsWatchResult{
			Changes: changes,
		}, nil
	}
	var err error
	if e, ok := w.watcher.(hasErr); ok {
		err = e.Err()
	}
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.StringsWatchResult{}, err
}

// srvRelationUnitsWatcher defines the API wrapping a state.RelationUnitsWatcher.
// It notifies about units entering and leaving the scope of a RelationUnit,
// and changes to the settings of those units known to have entered.
type srvRelationUnitsWatcher struct {
	watcherCommon
	watcher common.RelationUnitsWatcher
}

func newRelationUnitsWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()

	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	watcher, ok := resources.Get(id).(common.RelationUnitsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvRelationUnitsWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       watcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvRelationUnitsWatcher.
func (w *srvRelationUnitsWatcher) Next() (params.RelationUnitsWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		return params.RelationUnitsWatchResult{
			Changes: changes,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.RelationUnitsWatchResult{}, err
}

// srvRemoteRelationWatcher defines the API wrapping a
// state.RelationUnitsWatcher but serving the events it emits as
// fully-expanded params.RemoteRelationChangeEvents so they can be
// used across model/controller boundaries.
type srvRemoteRelationWatcher struct {
	watcherCommon
	backend crossmodel.Backend
	watcher *crossmodel.WrappedUnitsWatcher
}

func newRemoteRelationWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()

	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	watcher, ok := resources.Get(id).(*crossmodel.WrappedUnitsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvRemoteRelationWatcher{
		watcherCommon: newWatcherCommon(context),
		backend:       crossmodel.GetBackend(context.State()),
		watcher:       watcher,
	}, nil
}

func (w *srvRemoteRelationWatcher) Next() (params.RemoteRelationWatchResult, error) {
	if change, ok := <-w.watcher.Changes(); ok {
		// Expand the change into a cross-model event.
		expanded, err := crossmodel.ExpandChange(
			w.backend,
			w.watcher.RelationToken,
			w.watcher.ApplicationToken,
			change,
		)
		if err != nil {
			return params.RemoteRelationWatchResult{
				Error: apiservererrors.ServerError(err),
			}, nil
		}
		return params.RemoteRelationWatchResult{
			Changes: expanded,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.RemoteRelationWatchResult{}, err
}

// srvRelationStatusWatcher defines the API wrapping a state.RelationStatusWatcher.
type srvRelationStatusWatcher struct {
	watcherCommon
	st      *state.State
	watcher state.StringsWatcher
}

func newRelationStatusWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()

	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	watcher, ok := resources.Get(id).(state.StringsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvRelationStatusWatcher{
		watcherCommon: newWatcherCommon(context),
		st:            context.State(),
		watcher:       watcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvRelationStatusWatcher.
func (w *srvRelationStatusWatcher) Next() (params.RelationLifeSuspendedStatusWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		changesParams := make([]params.RelationLifeSuspendedStatusChange, len(changes))
		for i, key := range changes {
			change, err := crossmodel.GetRelationLifeSuspendedStatusChange(crossmodel.GetBackend(w.st), key)
			if err != nil {
				return params.RelationLifeSuspendedStatusWatchResult{
					Error: apiservererrors.ServerError(err),
				}, nil
			}
			changesParams[i] = *change
		}
		return params.RelationLifeSuspendedStatusWatchResult{
			Changes: changesParams,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.RelationLifeSuspendedStatusWatchResult{}, err
}

// srvOfferStatusWatcher defines the API wrapping a crossmodelrelations.OfferStatusWatcher.
type srvOfferStatusWatcher struct {
	watcherCommon
	st      *state.State
	model   *cache.Model
	watcher crossmodelrelations.OfferWatcher
}

func newOfferStatusWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()

	st := context.State()
	model, err := context.CachedModel(st.ModelUUID())
	if err != nil {
		return watcherCommon{}, err
	}

	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	watcher, ok := resources.Get(id).(crossmodelrelations.OfferWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvOfferStatusWatcher{
		watcherCommon: newWatcherCommon(context),
		st:            st,
		model:         model,
		watcher:       watcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvOfferStatusWatcher.
func (w *srvOfferStatusWatcher) Next() (params.OfferStatusWatchResult, error) {
	if _, ok := <-w.watcher.Changes(); ok {
		shim := crossmodel.CacheShim{w.model}
		change, err := crossmodel.GetOfferStatusChange(
			shim, crossmodel.GetBackend(w.st),
			w.watcher.OfferUUID(), w.watcher.OfferName())
		if err != nil {
			return params.OfferStatusWatchResult{
				Error: apiservererrors.ServerError(err),
			}, nil
		}
		return params.OfferStatusWatchResult{
			Changes: []params.OfferStatusChange{*change},
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.OfferStatusWatchResult{}, err
}

// srvMachineStorageIdsWatcher defines the API wrapping a state.StringsWatcher
// watching machine/storage attachments. This watcher notifies about storage
// entities (volumes/filesystems) being attached to and detached from machines.
//
// TODO(axw) state needs a new watcher, this is a bt of a hack. State watchers
// could do with some deduplication of logic, and I don't want to add to that
// spaghetti right now.
type srvMachineStorageIdsWatcher struct {
	watcherCommon
	watcher state.StringsWatcher
	parser  func([]string) ([]params.MachineStorageId, error)
}

func newVolumeAttachmentsWatcher(context facade.Context) (facade.Facade, error) {
	return newMachineStorageIdsWatcher(
		context,
		storagecommon.ParseVolumeAttachmentIds,
	)
}

func newVolumeAttachmentPlansWatcher(context facade.Context) (facade.Facade, error) {
	return newMachineStorageIdsWatcher(
		context,
		storagecommon.ParseVolumeAttachmentIds,
	)
}

func newFilesystemAttachmentsWatcher(context facade.Context) (facade.Facade, error) {
	return newMachineStorageIdsWatcher(
		context,
		storagecommon.ParseFilesystemAttachmentIds,
	)
}

func newMachineStorageIdsWatcher(
	context facade.Context,
	parser func([]string) ([]params.MachineStorageId, error),
) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()
	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	watcher, ok := resources.Get(id).(state.StringsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvMachineStorageIdsWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       watcher,
		parser:        parser,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvMachineStorageIdsWatcher.
func (w *srvMachineStorageIdsWatcher) Next() (params.MachineStorageIdsWatchResult, error) {
	if stringChanges, ok := <-w.watcher.Changes(); ok {
		changes, err := w.parser(stringChanges)
		if err != nil {
			return params.MachineStorageIdsWatchResult{}, err
		}
		return params.MachineStorageIdsWatchResult{
			Changes: changes,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.MachineStorageIdsWatchResult{}, err
}

// EntitiesWatcher defines an interface based on the StringsWatcher
// but also providing a method for the mapping of the received
// strings to the tags of the according entities.
type EntitiesWatcher interface {
	state.StringsWatcher

	// MapChanges maps the received strings to their according tag strings.
	// The EntityFinder interface representing state or a mock has to be
	// upcasted into the needed sub-interface of state for the real mapping.
	MapChanges(in []string) ([]string, error)
}

// srvEntitiesWatcher defines the API for methods on a state.StringsWatcher.
// Each client has its own current set of watchers, stored in resources.
// srvEntitiesWatcher notifies about changes for all entities of a given kind,
// sending the changes as a list of strings, which could be transformed
// from state entity ids to their corresponding entity tags.
type srvEntitiesWatcher struct {
	watcherCommon
	watcher EntitiesWatcher
}

func newEntitiesWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()

	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	watcher, ok := resources.Get(id).(EntitiesWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvEntitiesWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       watcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvEntitiesWatcher.
func (w *srvEntitiesWatcher) Next() (params.EntitiesWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		mapped, err := w.watcher.MapChanges(changes)
		if err != nil {
			return params.EntitiesWatchResult{}, errors.Annotate(err, "cannot map changes")
		}
		return params.EntitiesWatchResult{
			Changes: mapped,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.EntitiesWatchResult{}, err
}

var getMigrationBackend = func(st *state.State) migrationBackend {
	return st
}

var getControllerBackend = func(pool *state.StatePool) controllerBackend {
	return pool.SystemState()
}

// migrationBackend defines model State functionality required by the
// migration watchers.
type migrationBackend interface {
	LatestMigration() (state.ModelMigration, error)
}

// migrationBackend defines controller State functionality required by the
// migration watchers.
type controllerBackend interface {
	APIHostPortsForClients() ([]network.SpaceHostPorts, error)
	ControllerConfig() (controller.Config, error)
}

func newMigrationStatusWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()
	st := context.State()
	pool := context.StatePool()

	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	w, ok := resources.Get(id).(state.NotifyWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvMigrationStatusWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       w,
		st:            getMigrationBackend(st),
		ctrlSt:        getControllerBackend(pool),
	}, nil
}

type srvMigrationStatusWatcher struct {
	watcherCommon
	watcher state.NotifyWatcher
	st      migrationBackend
	ctrlSt  controllerBackend
}

// Next returns when the status for a model migration for the
// associated model changes. The current details for the active
// migration are returned.
func (w *srvMigrationStatusWatcher) Next() (params.MigrationStatus, error) {
	empty := params.MigrationStatus{}

	if _, ok := <-w.watcher.Changes(); !ok {
		err := w.watcher.Err()
		if err == nil {
			err = apiservererrors.ErrStoppedWatcher
		}
		return empty, err
	}

	mig, err := w.st.LatestMigration()
	if errors.IsNotFound(err) {
		return params.MigrationStatus{
			Phase: migration.NONE.String(),
		}, nil
	} else if err != nil {
		return empty, errors.Annotate(err, "migration lookup")
	}

	phase, err := mig.Phase()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving migration phase")
	}

	sourceAddrs, err := w.getLocalHostPorts()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving source addresses")
	}

	sourceCACert, err := getControllerCACert(w.ctrlSt)
	if err != nil {
		return empty, errors.Annotate(err, "retrieving source CA cert")
	}

	target, err := mig.TargetInfo()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving target info")
	}

	return params.MigrationStatus{
		MigrationId:    mig.Id(),
		Attempt:        mig.Attempt(),
		Phase:          phase.String(),
		SourceAPIAddrs: sourceAddrs,
		SourceCACert:   sourceCACert,
		TargetAPIAddrs: target.Addrs,
		TargetCACert:   target.CACert,
	}, nil
}

func (w *srvMigrationStatusWatcher) getLocalHostPorts() ([]string, error) {
	hostports, err := w.ctrlSt.APIHostPortsForClients()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var out []string
	for _, section := range hostports {
		for _, hostport := range section {
			out = append(out, hostport.String())
		}
	}
	return out, nil
}

// This is a shim to avoid the need to use a working State into the
// unit tests. It is tested as part of the client side API tests.
var getControllerCACert = func(st controllerBackend) (string, error) {
	cfg, err := st.ControllerConfig()
	if err != nil {
		return "", errors.Trace(err)
	}

	cacert, ok := cfg.CACert()
	if !ok {
		return "", errors.New("missing CA cert for controller model")
	}
	return cacert, nil
}

// newModelSummaryWatcher exists solely to be registered with regRaw.
// Standard registration doesn't handle watcher types (it checks for
// and empty ID in the context).
func newModelSummaryWatcher(context facade.Context) (facade.Facade, error) {
	return NewModelSummaryWatcher(context)
}

// NewModelSummaryWatcher returns a new API server endpoint for interacting with
// a watcher created by the WatchModelSummaries and WatchAllModelSummaries API
// calls.
func NewModelSummaryWatcher(context facade.Context) (*SrvModelSummaryWatcher, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()

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
	watcher, ok := resources.Get(id).(cache.ModelSummaryWatcher)
	if !ok {
		return nil, errors.Annotatef(apiservererrors.ErrUnknownWatcher, "watcher id: %s", id)
	}
	return &SrvModelSummaryWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       watcher,
	}, nil
}

// SrvModelSummaryWatcher defines the API methods on a ModelSummaryWatcher.
type SrvModelSummaryWatcher struct {
	watcherCommon
	watcher cache.ModelSummaryWatcher
}

// Next will return the current state of everything on the first call
// and subsequent calls will return just those model summaries that have
// changed.
func (w *SrvModelSummaryWatcher) Next() (params.SummaryWatcherNextResults, error) {
	if summaries, ok := <-w.watcher.Changes(); ok {
		return params.SummaryWatcherNextResults{
			Models: w.translate(summaries),
		}, nil
	}
	return params.SummaryWatcherNextResults{}, apiservererrors.ErrStoppedWatcher
}

func (w *SrvModelSummaryWatcher) translate(summaries []cache.ModelSummary) []params.ModelAbstract {
	response := make([]params.ModelAbstract, 0, len(summaries))
	for _, summary := range summaries {
		if summary.Removed {
			response = append(response, params.ModelAbstract{
				UUID:    summary.UUID,
				Removed: true,
			})
			continue
		}

		result := params.ModelAbstract{
			UUID:       summary.UUID,
			Controller: summary.Controller,
			Name:       summary.Name,
			Admins:     summary.Admins,
			Cloud:      summary.Cloud,
			Region:     summary.Region,
			Credential: summary.Credential,
			Size: params.ModelSummarySize{
				Machines:     summary.MachineCount,
				Containers:   summary.ContainerCount,
				Applications: summary.ApplicationCount,
				Units:        summary.UnitCount,
				Relations:    summary.RelationCount,
			},
			Status:      summary.Status,
			Messages:    w.translateMessages(summary.Messages),
			Annotations: summary.Annotations,
		}
		response = append(response, result)
	}
	return response
}

func (w *SrvModelSummaryWatcher) translateMessages(messages []cache.ModelSummaryMessage) []params.ModelSummaryMessage {
	if messages == nil {
		return nil
	}
	result := make([]params.ModelSummaryMessage, len(messages))
	for i, m := range messages {
		result[i] = params.ModelSummaryMessage{
			Agent:   m.Agent,
			Message: m.Message,
		}
	}
	return result
}

// srvSecretRotationWatcher defines the API wrapping a SecretsRotationWatcher.
type srvSecretRotationWatcher struct {
	watcherCommon
	st      *state.State
	watcher state.SecretsRotationWatcher
}

func newSecretsRotationWatcher(context facade.Context) (facade.Facade, error) {
	id := context.ID()
	auth := context.Auth()
	resources := context.Resources()

	st := context.State()

	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	watcher, ok := resources.Get(id).(state.SecretsRotationWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvSecretRotationWatcher{
		watcherCommon: newWatcherCommon(context),
		st:            st,
		watcher:       watcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvSecretRotationWatcher.
func (w *srvSecretRotationWatcher) Next() (params.SecretRotationWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		return params.SecretRotationWatchResult{
			Changes: w.translateChanges(changes),
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.SecretRotationWatchResult{}, err
}

func (w *srvSecretRotationWatcher) translateChanges(changes []corewatcher.SecretRotationChange) []params.SecretRotationChange {
	if changes == nil {
		return nil
	}
	result := make([]params.SecretRotationChange, len(changes))
	for i, c := range changes {
		result[i] = params.SecretRotationChange{
			ID:             c.ID,
			URL:            c.URL.ID(),
			RotateInterval: c.RotateInterval,
			LastRotateTime: c.LastRotateTime,
		}
	}
	return result
}
