// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/status"
)

var logger = loggo.GetLogger("juju.apiserver.controller.caasunitprovisioner")

type Facade struct {
	*common.LifeGetter
	resources facade.Resources
	state     CAASUnitProvisionerState
}

// NewStateFacade provides the signature required for facade registration.
func NewStateFacade(ctx facade.Context) (*Facade, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	return NewFacade(
		resources,
		authorizer,
		stateShim{ctx.State()},
	)
}

// NewFacade returns a new CAAS unit provisioner Facade facade.
func NewFacade(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st CAASUnitProvisionerState,
) (*Facade, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	return &Facade{
		LifeGetter: common.NewLifeGetter(
			st, common.AuthAny(
				common.AuthFuncForTagKind(names.ApplicationTagKind),
				common.AuthFuncForTagKind(names.UnitTagKind),
			),
		),
		resources: resources,
		state:     st,
	}, nil
}

// WatchApplications starts a StringsWatcher to watch CAAS applications
// deployed to this model.
func (f *Facade) WatchApplications() (params.StringsWatchResult, error) {
	watch := f.state.WatchApplications()
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: f.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return params.StringsWatchResult{}, watcher.EnsureErr(watch)
}

// WatchUnits starts a StringsWatcher to watch changes to the
// lifecycle states of units for the specified applications in
// this model.
func (f *Facade) WatchUnits(args params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		id, changes, err := f.watchUnits(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].StringsWatcherId = id
		results.Results[i].Changes = changes
	}
	return results, nil
}

func (f *Facade) watchUnits(tagString string) (string, []string, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	app, err := f.state.Application(tag.Id())
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	w := app.WatchUnits()
	if changes, ok := <-w.Changes(); ok {
		return f.resources.Register(w), changes, nil
	}
	return "", nil, watcher.EnsureErr(w)
}

// WatchPodSpec starts a NotifyWatcher to watch changes to the
// pod spec for specified units in this model.
func (f *Facade) WatchPodSpec(args params.Entities) (params.NotifyWatchResults, error) {
	model, err := f.state.Model()
	if err != nil {
		return params.NotifyWatchResults{}, errors.Trace(err)
	}
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		id, err := f.watchPodSpec(model, arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].NotifyWatcherId = id
	}
	return results, nil
}

func (f *Facade) watchPodSpec(model Model, tagString string) (string, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return "", errors.Trace(err)
	}
	w, err := model.WatchPodSpec(tag)
	if err != nil {
		return "", errors.Trace(err)
	}
	if _, ok := <-w.Changes(); ok {
		return f.resources.Register(w), nil
	}
	return "", watcher.EnsureErr(w)
}

// PodSpec returns the pod spec for specified units in this model.
func (f *Facade) PodSpec(args params.Entities) (params.StringResults, error) {
	model, err := f.state.Model()
	if err != nil {
		return params.StringResults{}, errors.Trace(err)
	}
	results := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		spec, err := f.podSpec(model, arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = spec
	}
	return results, nil
}

func (f *Facade) podSpec(model Model, tagString string) (string, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return "", errors.Trace(err)
	}
	return model.PodSpec(tag)
}

// ApplicationsConfig returns the config for the specified applications.
func (f *Facade) ApplicationsConfig(args params.Entities) (params.ApplicationGetConfigResults, error) {
	results := params.ApplicationGetConfigResults{
		Results: make([]params.ConfigResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		result, err := f.getApplicationConfig(arg.Tag)
		results.Results[i].Config = result
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

func (f *Facade) getApplicationConfig(tagString string) (map[string]interface{}, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return nil, errors.Trace(err)
	}
	app, err := f.state.Application(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return app.ApplicationConfig()
}

// UpdateApplicationsUnits updates the Juju data model to reflect the given
// units of the specified application.
func (a *Facade) UpdateApplicationsUnits(args params.UpdateApplicationUnitArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	if len(args.Args) == 0 {
		return result, nil
	}
	for i, appUpdate := range args.Args {
		appTag, err := names.ParseApplicationTag(appUpdate.ApplicationTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		app, err := a.state.Application(appTag.Id())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		err = a.updateUnitsFromCloud(app, appUpdate.Units)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

// updateStatus constructs the unit and agent status values based on the pod status.
func (a *Facade) updateStatus(params params.ApplicationUnitParams) (
	agentStatus *status.StatusInfo,
	unitStatus *status.StatusInfo,
	_ error,
) {
	switch status.Status(params.Status) {
	case status.Unknown:
		// The container runtime can spam us with unimportant
		// status updates, so ignore any irrelevant ones.
		return nil, nil, nil
	case status.Allocating:
		// The container runtime has decided to restart the pod.
		agentStatus = &status.StatusInfo{
			Status:  status.Allocating,
			Message: params.Info,
		}
		unitStatus = &status.StatusInfo{
			Status:  status.Waiting,
			Message: status.MessageWaitForContainer,
		}
	case status.Running:
		// A pod has finished starting so the workload is now active.
		agentStatus = &status.StatusInfo{
			Status: status.Idle,
		}
		unitStatus = &status.StatusInfo{
			Status:  status.Active,
			Message: params.Info,
		}
	case status.Error:
		agentStatus = &status.StatusInfo{
			Status:  status.Error,
			Message: params.Info,
			Data:    params.Data,
		}
	}
	return agentStatus, unitStatus, nil
}

// updateUnitsFromCloud takes a slice of unit information provided by an external
// source (typically a cloud update event) and merges that with the existing unit
// data model in state. The passed in units are the complete set for the cloud, so
// any existing units in state with provider ids which aren't in the set will be removed.
// This method is used when the cloud manages the units rather than Juju.
func (a *Facade) updateUnitsFromCloud(app Application, unitUpdates []params.ApplicationUnitParams) error {
	// Set up the initial data structures.
	existingStateUnits, err := app.AllUnits()
	if err != nil {
		return errors.Trace(err)
	}

	stateUnitsById := make(map[string]Unit)
	cloudUnitsById := make(map[string]params.ApplicationUnitParams)

	// Record all unit provider ids known to exist in the cloud.
	for _, u := range unitUpdates {
		cloudUnitsById[u.ProviderId] = u
	}

	stateUnitExistsInCloud := func(providerId string) bool {
		if providerId == "" {
			return false
		}
		_, ok := cloudUnitsById[providerId]
		return ok
	}

	unitInfo := &updateStateUnitParams{
		stateUnitsInCloud: make(map[string]Unit),
		deletedRemoved:    true,
	}
	var (
		// aliveStateIds holds the provider ids of alive units in state.
		aliveStateIds = set.NewStrings()

		// extraStateIds holds the provider ids of units in state which
		// no longer exist in the cloud.
		extraStateIds = set.NewStrings()
	)

	// Loop over any existing state units and record those which do not yet have
	// provider ids, and those which have been removed or updated.
	for _, u := range existingStateUnits {
		var providerId string
		info, err := u.ContainerInfo()
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		if err == nil {
			providerId = info.ProviderId()
		}

		unitAlive := u.Life() == state.Alive
		if !unitAlive {
			continue
		}

		if providerId == "" {
			logger.Debugf("unit %q is not associated with any pod", u.Name())
			unitInfo.unassociatedUnits = append(unitInfo.unassociatedUnits, u)
			continue
		}
		stateUnitsById[providerId] = u
		stateUnitInCloud := stateUnitExistsInCloud(providerId)
		aliveStateIds.Add(providerId)

		if stateUnitInCloud {
			logger.Debugf("unit %q (%v) has changed in the cloud", u.Name(), providerId)
			unitInfo.stateUnitsInCloud[u.UnitTag().String()] = u
		} else {
			extraStateIds.Add(providerId)
		}
	}

	// Do it in sorted order so it's deterministic for tests.
	var ids []string
	for id := range cloudUnitsById {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// Sort extra ids also to guarantee order.
	var extraIds []string
	for id := range extraStateIds {
		extraIds = append(extraIds, id)
	}
	sort.Strings(extraIds)
	extraIdIndex := 0

	for _, id := range ids {
		u := cloudUnitsById[id]
		if aliveStateIds.Contains(id) {
			u.UnitTag = stateUnitsById[id].UnitTag().String()
			unitInfo.existingCloudUnits = append(unitInfo.existingCloudUnits, u)
			continue
		}
		// If there are units in state which used to be be associated with a pod
		// but are now not, we give those state units a pod which is not
		// associated with any unit. The pod may have been added new due to a scale
		// out operation, or the pod's state unit was deleted but the cloud removed
		// a different pod in response and so we need to re-associated this still
		// alive pod with the orphaned unit.
		if !extraStateIds.IsEmpty() {
			extraId := extraIds[extraIdIndex]
			extraIdIndex += 1
			extraStateIds.Remove(extraId)
			u.ProviderId = id
			unit := stateUnitsById[extraId]
			u.UnitTag = unit.UnitTag().String()
			unitInfo.existingCloudUnits = append(unitInfo.existingCloudUnits, u)
			unitInfo.stateUnitsInCloud[u.UnitTag] = unit
			continue
		}

		// A new pod was added to the cloud but does not yet have a unit in state.
		unitInfo.addedCloudUnits = append(unitInfo.addedCloudUnits, u)
	}

	// If there are any extra provider ids left over after allocating all the cloud pods,
	// then consider those state units as terminated.
	for _, providerId := range extraStateIds.Values() {
		u := stateUnitsById[providerId]
		logger.Debugf("unit %q (%v) has been removed from the cloud", u.Name(), providerId)
		unitInfo.removedUnits = append(unitInfo.removedUnits, u)
	}

	return a.updateStateUnits(app, unitInfo)
}

type updateStateUnitParams struct {
	stateUnitsInCloud  map[string]Unit
	addedCloudUnits    []params.ApplicationUnitParams
	existingCloudUnits []params.ApplicationUnitParams
	removedUnits       []Unit
	unassociatedUnits  []Unit
	deletedRemoved     bool
}

func (a *Facade) updateStateUnits(app Application, unitInfo *updateStateUnitParams) error {

	if app.Life() != state.Alive {
		// We ignore any updates for dying applications.
		logger.Debugf("ignoring unit updates for dying application: %v", app.Name())
		return nil
	}

	logger.Tracef("added cloud units: %+v", unitInfo.addedCloudUnits)
	logger.Tracef("existing cloud units: %+v", unitInfo.existingCloudUnits)
	logger.Tracef("removed units: %+v", unitInfo.removedUnits)
	logger.Tracef("unassociated units: %+v", unitInfo.unassociatedUnits)

	// Now we have the added, removed, updated units all sorted,
	// generate the state update operations.
	var unitUpdate state.UpdateUnitsOperation

	for _, u := range unitInfo.removedUnits {
		if unitInfo.deletedRemoved {
			unitUpdate.Deletes = append(unitUpdate.Deletes, u.DestroyOperation())
			continue
		}
		// We'll set the status as Terminated. This will either be transient, as will
		// occur when a pod is restarted external to Juju, or permanent if the pod has
		// been deleted external to Juju. In the latter case, juju remove-unit will be
		// need to clean things up on the Juju side.
		unitStatus := &status.StatusInfo{
			Status:  status.Terminated,
			Message: "unit stopped by the cloud",
		}
		// And we'll reset the provider id - the pod may be restarted and we'll
		// record the new id next time.
		resetId := ""
		updateProps := state.UnitUpdateProperties{
			ProviderId: &resetId,
			UnitStatus: unitStatus,
		}
		unitUpdate.Updates = append(unitUpdate.Updates,
			u.UpdateOperation(updateProps))
	}

	for _, unitParams := range unitInfo.existingCloudUnits {
		u, ok := unitInfo.stateUnitsInCloud[unitParams.UnitTag]
		if !ok {
			logger.Warningf("unexpected unit parameters %+v not in state", unitParams)
			continue
		}
		agentStatus, unitStatus, err := a.updateStatus(unitParams)
		if err != nil {
			return errors.Trace(err)
		}
		params := unitParams
		updateProps := state.UnitUpdateProperties{
			ProviderId:  &params.ProviderId,
			Address:     &params.Address,
			Ports:       &params.Ports,
			AgentStatus: agentStatus,
			UnitStatus:  unitStatus,
		}

		unitUpdate.Updates = append(unitUpdate.Updates,
			u.UpdateOperation(updateProps))
	}

	// For newly added units in the cloud, either update state units which
	// exist but which do not yet have provider ids (recording the provider
	// id as well), or add a brand new unit.
	idx := 0
	for _, unitParams := range unitInfo.addedCloudUnits {
		agentStatus, unitStatus, err := a.updateStatus(unitParams)
		if err != nil {
			return errors.Trace(err)
		}
		params := unitParams
		updateProps := state.UnitUpdateProperties{
			ProviderId:  &params.ProviderId,
			Address:     &params.Address,
			Ports:       &params.Ports,
			AgentStatus: agentStatus,
			UnitStatus:  unitStatus,
		}

		if idx < len(unitInfo.unassociatedUnits) {
			u := unitInfo.unassociatedUnits[idx]
			unitUpdate.Updates = append(unitUpdate.Updates,
				u.UpdateOperation(updateProps))
			idx += 1
			continue
		}

		unitUpdate.Adds = append(unitUpdate.Adds,
			app.AddOperation(updateProps))
	}
	err := app.UpdateUnits(&unitUpdate)
	// We ignore any updates for dying applications.
	if state.IsNotAlive(err) {
		return nil
	}
	return err
}

// UpdateApplicationsService updates the Juju data model to reflect the given
// service details of the specified application.
func (a *Facade) UpdateApplicationsService(args params.UpdateApplicationServiceArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	if len(args.Args) == 0 {
		return result, nil
	}
	for i, appUpdate := range args.Args {
		appTag, err := names.ParseApplicationTag(appUpdate.ApplicationTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		app, err := a.state.Application(appTag.Id())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := app.UpdateCloudService(appUpdate.ProviderId, params.NetworkAddresses(appUpdate.Addresses...)); err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}
