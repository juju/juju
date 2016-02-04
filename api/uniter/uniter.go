// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/watcher"
)

const uniterFacade = "Uniter"

// State provides access to the Uniter API facade.
type State struct {
	*common.ModelWatcher
	*common.APIAddresser
	*StorageAccessor

	LeadershipSettings *LeadershipSettingsAccessor
	facade             base.FacadeCaller
	// unitTag contains the authenticated unit's tag.
	unitTag names.UnitTag
}

// newStateForVersion creates a new client-side Uniter facade for the
// given version.
func newStateForVersion(
	caller base.APICaller,
	authTag names.UnitTag,
	version int,
) *State {
	facadeCaller := base.NewFacadeCallerForVersion(
		caller,
		uniterFacade,
		version,
	)
	state := &State{
		ModelWatcher:    common.NewModelWatcher(facadeCaller),
		APIAddresser:    common.NewAPIAddresser(facadeCaller),
		StorageAccessor: NewStorageAccessor(facadeCaller),
		facade:          facadeCaller,
		unitTag:         authTag,
	}

	newWatcher := func(result params.NotifyWatchResult) watcher.NotifyWatcher {
		return apiwatcher.NewNotifyWatcher(caller, result)
	}
	state.LeadershipSettings = NewLeadershipSettingsAccessor(
		facadeCaller.FacadeCall,
		newWatcher,
		ErrIfNotVersionFn(2, state.BestAPIVersion()),
	)
	return state
}

func newStateForVersionFn(version int) func(base.APICaller, names.UnitTag) *State {
	return func(caller base.APICaller, authTag names.UnitTag) *State {
		return newStateForVersion(caller, authTag, version)
	}
}

// newStateV3 creates a new client-side Uniter facade, version 3.
var newStateV3 = newStateForVersionFn(3)

// NewState creates a new client-side Uniter facade.
// Defined like this to allow patching during tests.
var NewState = newStateV3

// BestAPIVersion returns the API version that we were able to
// determine is supported by both the client and the API Server.
func (st *State) BestAPIVersion() int {
	return st.facade.BestAPIVersion()
}

// Facade returns the current facade.
func (st *State) Facade() base.FacadeCaller {
	return st.facade
}

// life requests the lifecycle of the given entity from the server.
func (st *State) life(tag names.Tag) (params.Life, error) {
	return common.Life(st.facade, tag)
}

// relation requests relation information from the server.
func (st *State) relation(relationTag, unitTag names.Tag) (params.RelationResult, error) {
	nothing := params.RelationResult{}
	var result params.RelationResults
	args := params.RelationUnits{
		RelationUnits: []params.RelationUnit{
			{Relation: relationTag.String(), Unit: unitTag.String()},
		},
	}
	err := st.facade.FacadeCall("Relation", args, &result)
	if err != nil {
		return nothing, err
	}
	if len(result.Results) != 1 {
		return nothing, fmt.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return nothing, err
	}
	return result.Results[0], nil
}

// getOneAction retrieves a single Action from the controller.
func (st *State) getOneAction(tag *names.ActionTag) (params.ActionsQueryResult, error) {
	nothing := params.ActionsQueryResult{}

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	}

	var results params.ActionsQueryResults
	err := st.facade.FacadeCall("Actions", args, &results)
	if err != nil {
		return nothing, err
	}

	if len(results.Results) > 1 {
		return nothing, fmt.Errorf("expected only 1 action query result, got %d", len(results.Results))
	}

	// handle server errors
	result := results.Results[0]
	if err := result.Error; err != nil {
		return nothing, err
	}

	return result, nil
}

// Unit provides access to methods of a state.Unit through the facade.
func (st *State) Unit(tag names.UnitTag) (*Unit, error) {
	life, err := st.life(tag)
	if err != nil {
		return nil, err
	}
	return &Unit{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}

// Service returns a service state by tag.
func (st *State) Service(tag names.ServiceTag) (*Service, error) {
	life, err := st.life(tag)
	if err != nil {
		return nil, err
	}
	return &Service{
		tag:  tag,
		life: life,
		st:   st,
	}, nil
}

// ProviderType returns a provider type used by the current juju model.
//
// TODO(dimitern): We might be able to drop this, once we have machine
// addresses implemented fully. See also LP bug 1221798.
func (st *State) ProviderType() (string, error) {
	var result params.StringResult
	err := st.facade.FacadeCall("ProviderType", nil, &result)
	if err != nil {
		return "", err
	}
	if err := result.Error; err != nil {
		return "", err
	}
	return result.Result, nil
}

// Charm returns the charm with the given URL.
func (st *State) Charm(curl *charm.URL) (*Charm, error) {
	if curl == nil {
		return nil, fmt.Errorf("charm url cannot be nil")
	}
	return &Charm{
		st:   st,
		curl: curl,
	}, nil
}

// Relation returns the existing relation with the given tag.
func (st *State) Relation(relationTag names.RelationTag) (*Relation, error) {
	result, err := st.relation(relationTag, st.unitTag)
	if err != nil {
		return nil, err
	}
	return &Relation{
		id:   result.Id,
		tag:  relationTag,
		life: result.Life,
		st:   st,
	}, nil
}

// Action returns the Action with the given tag.
func (st *State) Action(tag names.ActionTag) (*Action, error) {
	result, err := st.getOneAction(&tag)
	if err != nil {
		return nil, err
	}
	return &Action{
		name:   result.Action.Action.Name,
		params: result.Action.Action.Parameters,
	}, nil
}

// ActionBegin marks an action as running.
func (st *State) ActionBegin(tag names.ActionTag) error {
	var outcome params.ErrorResults

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	}

	err := st.facade.FacadeCall("BeginActions", args, &outcome)
	if err != nil {
		return err
	}
	if len(outcome.Results) != 1 {
		return fmt.Errorf("expected 1 result, got %d", len(outcome.Results))
	}
	result := outcome.Results[0]
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// ActionFinish captures the structured output of an action.
func (st *State) ActionFinish(tag names.ActionTag, status string, results map[string]interface{}, message string) error {
	var outcome params.ErrorResults

	args := params.ActionExecutionResults{
		Results: []params.ActionExecutionResult{
			{
				ActionTag: tag.String(),
				Status:    status,
				Results:   results,
				Message:   message,
			},
		},
	}

	err := st.facade.FacadeCall("FinishActions", args, &outcome)
	if err != nil {
		return err
	}
	if len(outcome.Results) != 1 {
		return fmt.Errorf("expected 1 result, got %d", len(outcome.Results))
	}
	result := outcome.Results[0]
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// RelationById returns the existing relation with the given id.
func (st *State) RelationById(id int) (*Relation, error) {
	var results params.RelationResults
	args := params.RelationIds{
		RelationIds: []int{id},
	}
	err := st.facade.FacadeCall("RelationById", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return nil, err
	}
	relationTag := names.NewRelationTag(result.Key)
	return &Relation{
		id:   result.Id,
		tag:  relationTag,
		life: result.Life,
		st:   st,
	}, nil
}

// Model returns the model entity.
func (st *State) Model() (*Model, error) {
	var result params.ModelResult
	err := st.facade.FacadeCall("CurrentModel", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, err
	}
	return &Model{
		name: result.Name,
		uuid: result.UUID,
	}, nil
}

// AllMachinePorts returns all port ranges currently open on the given
// machine, mapped to the tags of the unit that opened them and the
// relation that applies.
func (st *State) AllMachinePorts(machineTag names.MachineTag) (map[network.PortRange]params.RelationUnit, error) {
	if st.BestAPIVersion() < 1 {
		// AllMachinePorts() was introduced in UniterAPIV1.
		return nil, errors.NotImplementedf("AllMachinePorts() (need V1+)")
	}
	var results params.MachinePortsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: machineTag.String()}},
	}
	err := st.facade.FacadeCall("AllMachinePorts", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	portsMap := make(map[network.PortRange]params.RelationUnit)
	for _, ports := range result.Ports {
		portRange := ports.PortRange.NetworkPortRange()
		portsMap[portRange] = params.RelationUnit{
			Unit:     ports.UnitTag,
			Relation: ports.RelationTag,
		}
	}
	return portsMap, nil
}

// WatchRelationUnits returns a watcher that notifies of changes to the
// counterpart units in the relation for the given unit.
func (st *State) WatchRelationUnits(
	relationTag names.RelationTag,
	unitTag names.UnitTag,
) (watcher.RelationUnitsWatcher, error) {
	var results params.RelationUnitsWatchResults
	args := params.RelationUnits{
		RelationUnits: []params.RelationUnit{{
			Relation: relationTag.String(),
			Unit:     unitTag.String(),
		}},
	}
	err := st.facade.FacadeCall("WatchRelationUnits", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewRelationUnitsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// ErrIfNotVersionFn returns a function which can be used to check for
// the minimum supported version, and, if appropriate, generate an
// error.
func ErrIfNotVersionFn(minVersion int, bestApiVersion int) func(string) error {
	return func(fnName string) error {
		if minVersion <= bestApiVersion {
			return nil
		}
		return errors.NotImplementedf("%s(...) requires v%d+", fnName, minVersion)
	}
}
