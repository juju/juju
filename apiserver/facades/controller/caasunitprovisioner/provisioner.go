// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
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

// WatchContainerSpec starts a NotifyWatcher to watch changes to the
// container spec for specified units in this model.
func (f *Facade) WatchContainerSpec(args params.Entities) (params.NotifyWatchResults, error) {
	model, err := f.state.Model()
	if err != nil {
		return params.NotifyWatchResults{}, errors.Trace(err)
	}
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		id, err := f.watchContainerSpec(model, arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].NotifyWatcherId = id
	}
	return results, nil
}

func (f *Facade) watchContainerSpec(model Model, tagString string) (string, error) {
	tag, err := names.ParseUnitTag(tagString)
	if err != nil {
		return "", errors.Trace(err)
	}
	w, err := model.WatchContainerSpec(tag)
	if err != nil {
		return "", errors.Trace(err)
	}
	if _, ok := <-w.Changes(); ok {
		return f.resources.Register(w), nil
	}
	return "", watcher.EnsureErr(w)
}

// ContainerSpec returns the container spec for specified units in this model.
func (f *Facade) ContainerSpec(args params.Entities) (params.StringResults, error) {
	model, err := f.state.Model()
	if err != nil {
		return params.StringResults{}, errors.Trace(err)
	}
	results := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		spec, err := f.containerSpec(model, arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = spec
	}
	return results, nil
}

func (f *Facade) containerSpec(model Model, tagString string) (string, error) {
	tag, err := names.ParseUnitTag(tagString)
	if err != nil {
		return "", errors.Trace(err)
	}
	return model.ContainerSpec(tag)
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
		if err := a.updateUnitsFromCloud(app, appUpdate.Units); err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

// updateUnitsFromCloud takes a slice of unit information provided by an external
// source (typically a cloud update event) and merges that with the existing unit
// data model in state. The passed in units are the complete set for the cloud, so
// any existing units in state with provider ids which aren't in the set will be removed.
func (a *Facade) updateUnitsFromCloud(app Application, units []params.ApplicationUnitParams) error {
	// Set up the initial data structures.
	existingStateUnits, err := app.AllUnits()
	if err != nil {
		return errors.Trace(err)
	}

	var (
		// unassociatedUnits are those which do not yet
		// have a provider id assigned.
		unassociatedUnits []Unit

		// removedUnits are those that exist in state but are not
		// represented in the unit params passed in.
		removedUnits []Unit

		addedCloudUnits []params.ApplicationUnitParams
	)
	stateUnitsById := make(map[string]Unit)
	stateUnitsByTag := make(map[string]Unit)
	aliveCloudUnitsById := make(map[string]params.ApplicationUnitParams)
	aliveCloudUnitTags := make(set.Strings)

	existingCloudUnitsById := make(map[string]params.ApplicationUnitParams)

	// Record all unit provider ids known to exist in the cloud.
	// We initially assume the corresponding Juju unit is alive
	// and will remove dying/dead ones below.
	for _, u := range units {
		aliveCloudUnitsById[u.ProviderId] = u
		if u.UnitTag != "" {
			aliveCloudUnitTags.Add(u.UnitTag)
		}
	}

	stateUnitExistsInCloud := func(u Unit) bool {
		// Tags take precedence over provider ids.
		if aliveCloudUnitTags.Contains(u.UnitTag().String()) {
			return true
		}
		// If any units have been created with tags, we only
		// support the case where they all have tags
		// (ie managed by Juju).
		if len(aliveCloudUnitTags) > 0 {
			return false
		}
		_, ok := aliveCloudUnitsById[u.ProviderId()]
		return ok
	}
	cloudUnitExistsInState := func(u params.ApplicationUnitParams) bool {
		// Tags take precedence over provider ids.
		if _, ok := stateUnitsByTag[u.UnitTag]; ok {
			return true
		}
		// If the cloud unit has a tag it was created by Juju
		// and we don't check provider id.
		if u.UnitTag != "" {
			return false
		}
		_, ok := stateUnitsById[u.ProviderId]
		return ok
	}

	// Loop over any existing state units and record those which do not yet have
	// provider ids, and those which have been removed or updated.
	for _, u := range existingStateUnits {
		unitAlive := u.Life() == state.Alive
		stateUnitInCloud := stateUnitExistsInCloud(u)
		if stateUnitInCloud {
			stateUnitsById[u.ProviderId()] = u
			stateUnitsByTag[u.UnitTag().String()] = u
		}
		if u.ProviderId() == "" && unitAlive {
			logger.Debugf("unit %q is not associated with any pod", u.Name())
			unassociatedUnits = append(unassociatedUnits, u)
		} else {
			if !unitAlive {
				delete(aliveCloudUnitsById, u.ProviderId())
				aliveCloudUnitTags.Remove(u.UnitTag().String())
				continue
			}
			if stateUnitInCloud {
				logger.Debugf("unit %q (%v) has changed in the cloud", u.Name(), u.ProviderId())
			} else {
				logger.Debugf("unit %q (%v) has been removed from the cloud", u.Name(), u.ProviderId())
				removedUnits = append(removedUnits, u)
			}
		}
	}

	for _, u := range aliveCloudUnitsById {
		if cloudUnitExistsInState(u) {
			existingCloudUnitsById[u.ProviderId] = u
		} else {
			if u.UnitTag == "" {
				addedCloudUnits = append(addedCloudUnits, u)
			} else {
				logger.Warningf("ignoring orphaned cloud unit: %v", u.UnitTag)
			}
		}
	}

	// Now we have the added, removed, updated units all sorted,
	// generate the state update operations.
	var unitUpdate state.UpdateUnitsOperation

	for _, u := range removedUnits {
		unitUpdate.Deletes = append(unitUpdate.Deletes, u.DestroyOperation())
	}

	unitUpdateProperties := func(unitParams params.ApplicationUnitParams) state.UnitUpdateProperties {
		return state.UnitUpdateProperties{
			ProviderId: unitParams.ProviderId,
			Address:    unitParams.Address,
			Ports:      unitParams.Ports,
			Status: &status.StatusInfo{
				Status:  status.Status(unitParams.Status),
				Message: unitParams.Info,
				Data:    unitParams.Data,
			},
		}
	}

	shouldUpdate := func(u Unit, params params.ApplicationUnitParams) (bool, error) {
		if u.ProviderId() == "" {
			return true, nil
		}
		existingStatus, err := u.AgentStatus()
		if err != nil {
			return false, errors.Trace(err)
		}
		if string(existingStatus.Status) != params.Status ||
			existingStatus.Message != params.Info ||
			len(existingStatus.Data) != len(params.Data) ||
			reflect.DeepEqual(existingStatus.Data, params.Data) {
			return true, nil
		}
		return false, nil
	}

	// For existing units which have been updated, create the necessary update ops.
	for id, unitParams := range existingCloudUnitsById {
		u, ok := stateUnitsByTag[unitParams.UnitTag]
		if !ok {
			u, ok = stateUnitsById[id]
		}
		if !ok {
			logger.Warningf("missing unit parameters %+v", unitParams)
			continue
		}
		// Check to see if any update is needed.
		update, err := shouldUpdate(u, unitParams)
		if err != nil {
			return errors.Trace(err)
		}
		if !update {
			continue
		}
		unitUpdate.Updates = append(unitUpdate.Updates,
			u.UpdateOperation(unitUpdateProperties(unitParams)))
	}

	// For newly added units in the cloud, either update state units which
	// exist but which do not yet have provider ids (recording the provider
	// id as well), or add a brand new unit.
	idx := 0
	for _, unitParams := range addedCloudUnits {
		if idx < len(unassociatedUnits) {
			u := unassociatedUnits[idx]
			unitUpdate.Updates = append(unitUpdate.Updates,
				u.UpdateOperation(unitUpdateProperties(unitParams)))
			idx += 1
			continue
		}

		unitUpdate.Adds = append(unitUpdate.Adds,
			app.AddOperation(unitUpdateProperties(unitParams)))
	}
	return app.UpdateUnits(&unitUpdate)
}
