// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"reflect"

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

var logger = loggo.GetLogger("juju.apiserver.controller.caasoperatorprovisioner")

type API struct {
	*common.PasswordChanger

	auth      facade.Authorizer
	resources facade.Resources

	state CAASOperatorProvisionerState
}

// NewStateCAASOperatorProvisionerAPI provides the signature required for facade registration.
func NewStateCAASOperatorProvisionerAPI(ctx facade.Context) (*API, error) {

	authorizer := ctx.Auth()
	resources := ctx.Resources()
	return NewCAASOperatorProvisionerAPI(resources, authorizer, stateShim{ctx.State()})
}

// NewCAASOperatorProvisionerAPI returns a new CAAS operator provisioner API facade.
func NewCAASOperatorProvisionerAPI(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st CAASOperatorProvisionerState,
) (*API, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	return &API{
		PasswordChanger: common.NewPasswordChanger(st, common.AuthAlways()),
		auth:            authorizer,
		resources:       resources,
		state:           st,
	}, nil
}

// WatchApplications starts a StringsWatcher to watch CAAS applications
// deployed to this model.
func (a *API) WatchApplications() (params.StringsWatchResult, error) {
	watch := a.state.WatchApplications()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: a.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return params.StringsWatchResult{}, watcher.EnsureErr(watch)
}

// UpdateApplicationsUnits updates the Juju data model to reflect the given
// units of the specified application.
func (a *API) UpdateApplicationsUnits(args params.UpdateApplicationUnitArgs) (params.ErrorResults, error) {
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
func (a *API) updateUnitsFromCloud(app Application, units []params.ApplicationUnitParams) error {
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
	aliveCloudUnits := make(map[string]params.ApplicationUnitParams)

	existingCloudUnitsById := make(map[string]params.ApplicationUnitParams)

	// Record all unit provider ids known to exist in the cloud.
	// We initially assume the corresponding Juju unit is alive
	// and will remove dying/dead ones below.
	for _, u := range units {
		aliveCloudUnits[u.Id] = u
	}

	// Loop over any existing state units and record those which do not yet have
	// provider ids, and those which have been removed or updated.
	for _, u := range existingStateUnits {
		unitAlive := u.Life() == state.Alive
		if u.ProviderId() == "" && unitAlive {
			logger.Debugf("unit %q is unallocated", u.Name())
			unassociatedUnits = append(unassociatedUnits, u)
		} else {
			if !unitAlive {
				delete(aliveCloudUnits, u.ProviderId())
				continue
			}
			if _, ok := aliveCloudUnits[u.ProviderId()]; ok {
				logger.Debugf("unit %q (%v) has changed in the cloud", u.Name(), u.ProviderId())
				stateUnitsById[u.ProviderId()] = u
			} else {
				logger.Debugf("unit %q (%v) has been removed from the cloud", u.Name(), u.ProviderId())
				removedUnits = append(removedUnits, u)
			}
		}
	}
	for _, u := range aliveCloudUnits {
		if _, ok := stateUnitsById[u.Id]; ok {
			existingCloudUnitsById[u.Id] = u
		} else {
			addedCloudUnits = append(addedCloudUnits, u)
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
			ProviderId: unitParams.Id,
			// TODO(caas)
			//Address:    unitParams.Address,
			//Ports:      unitParams.Ports,
			Status: &status.StatusInfo{
				Status:  status.Status(unitParams.Status),
				Message: unitParams.Info,
				Data:    unitParams.Data,
			},
		}
	}

	shouldUpdate := func(u Unit, params params.ApplicationUnitParams) (bool, error) {
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
		u := stateUnitsById[id]
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
