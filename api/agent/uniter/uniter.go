// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"github.com/juju/charm/v11"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	apiwatcher "github.com/juju/juju/api/watcher"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

const uniterFacade = "Uniter"

// State provides access to the Uniter API facade.
type State struct {
	*common.ModelWatcher
	*common.APIAddresser
	*common.UpgradeSeriesAPI
	*common.UnitStateAPI
	*StorageAccessor

	LeadershipSettings *LeadershipSettingsAccessor
	facade             base.FacadeCaller
	// unitTag contains the authenticated unit's tag.
	unitTag names.UnitTag
}

// NewState creates a new client-side Uniter facade.
func NewState(
	caller base.APICaller,
	authTag names.UnitTag,
) *State {
	facadeCaller := base.NewFacadeCaller(
		caller,
		uniterFacade,
	)
	state := &State{
		ModelWatcher:     common.NewModelWatcher(facadeCaller),
		APIAddresser:     common.NewAPIAddresser(facadeCaller),
		UpgradeSeriesAPI: common.NewUpgradeSeriesAPI(facadeCaller, authTag),
		UnitStateAPI:     common.NewUniterStateAPI(facadeCaller, authTag),
		StorageAccessor:  NewStorageAccessor(facadeCaller),
		facade:           facadeCaller,
		unitTag:          authTag,
	}

	newWatcher := func(result params.NotifyWatchResult) watcher.NotifyWatcher {
		return apiwatcher.NewNotifyWatcher(caller, result)
	}
	state.LeadershipSettings = NewLeadershipSettingsAccessor(
		facadeCaller.FacadeCall,
		newWatcher,
	)
	return state
}

// NewFromConnection returns a version of the Connection that provides
// functionality required by the uniter worker if possible else a non-nil error.
func NewFromConnection(c api.Connection) (*State, error) {
	authTag := c.AuthTag()
	unitTag, ok := authTag.(names.UnitTag)
	if !ok {
		return nil, errors.Errorf("expected UnitTag, got %T %v", authTag, authTag)
	}
	return NewState(c, unitTag), nil
}

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
func (st *State) life(tag names.Tag) (life.Value, error) {
	return common.OneLife(st.facade, tag)
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

func (st *State) setRelationStatus(id int, status relation.Status) error {
	args := params.RelationStatusArgs{
		Args: []params.RelationStatusArg{{
			UnitTag:    st.unitTag.String(),
			RelationId: id,
			Status:     params.RelationStatusValue(status),
		}},
	}
	var results params.ErrorResults
	if err := st.facade.FacadeCall("SetRelationStatus", args, &results); err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// getOneAction retrieves a single Action from the controller.
func (st *State) getOneAction(tag *names.ActionTag) (params.ActionResult, error) {
	nothing := params.ActionResult{}

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	}

	var results params.ActionResults
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

// ActionStatus provides the status of a single action.
func (st *State) ActionStatus(tag names.ActionTag) (string, error) {
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	}

	var results params.StringResults
	err := st.facade.FacadeCall("ActionStatus", args, &results)
	if err != nil {
		return "", err
	}

	if len(results.Results) > 1 {
		return "", fmt.Errorf("expected only 1 action query result, got %d", len(results.Results))
	}

	// handle server errors
	result := results.Results[0]
	if err := result.Error; err != nil {
		return "", err
	}

	return result.Result, nil
}

// Unit provides access to methods of a state.Unit through the facade.
func (st *State) Unit(tag names.UnitTag) (*Unit, error) {
	unit := &Unit{
		tag: tag,
		st:  st,
	}
	err := unit.Refresh()
	if err != nil {
		return nil, err
	}
	return unit, nil
}

// Application returns an application state by tag.
func (st *State) Application(tag names.ApplicationTag) (*Application, error) {
	life, err := st.life(tag)
	if err != nil {
		return nil, err
	}
	return &Application{
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
		id:        result.Id,
		tag:       relationTag,
		life:      result.Life,
		suspended: result.Suspended,
		st:        st,
		otherApp:  result.OtherApplication,
	}, nil
}

// Action returns the Action with the given tag.
func (st *State) Action(tag names.ActionTag) (*Action, error) {
	result, err := st.getOneAction(&tag)
	if err != nil {
		return nil, err
	}
	a := &Action{
		id:     tag.Id(),
		name:   result.Action.Name,
		params: result.Action.Parameters,
	}
	if result.Action.Parallel != nil {
		a.parallel = *result.Action.Parallel
	}
	if result.Action.ExecutionGroup != nil {
		a.executionGroup = *result.Action.ExecutionGroup
	}
	return a, nil
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
		id:        result.Id,
		tag:       relationTag,
		life:      result.Life,
		suspended: result.Suspended,
		st:        st,
		otherApp:  result.OtherApplication,
	}, nil
}

// Model returns the model entity.
func (st *State) Model() (*model.Model, error) {
	var result params.ModelResult
	err := st.facade.FacadeCall("CurrentModel", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, err
	}
	modelType := model.ModelType(result.Type)
	if modelType == "" {
		modelType = model.IAAS
	}
	return &model.Model{
		Name:      result.Name,
		UUID:      result.UUID,
		ModelType: modelType,
	}, nil
}

func processOpenPortRangesByEndpointResults(results params.OpenPortRangesByEndpointResults, tag names.Tag) (map[names.UnitTag]network.GroupedPortRanges, error) {
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		err := apiservererrors.RestoreError(result.Error)
		return nil, errors.Annotatef(err, "unable to fetch opened ports for %s", tag)
	}

	portRangeMap := make(map[names.UnitTag]network.GroupedPortRanges)
	for unitTagStr, unitPortRanges := range result.UnitPortRanges {
		unitTag, err := names.ParseUnitTag(unitTagStr)
		if err != nil {
			return nil, errors.Trace(err)
		}
		portRangeMap[unitTag] = make(network.GroupedPortRanges)
		for _, group := range unitPortRanges {
			portRangeMap[unitTag][group.Endpoint] = transform.Slice(group.PortRanges, func(pr params.PortRange) network.PortRange {
				return pr.NetworkPortRange()
			})
		}
	}
	return portRangeMap, nil
}

// OpenedMachinePortRangesByEndpoint returns all port ranges currently open on the given
// machine, grouped by unit tag and application endpoint.
func (st *State) OpenedMachinePortRangesByEndpoint(machineTag names.MachineTag) (map[names.UnitTag]network.GroupedPortRanges, error) {
	var results params.OpenPortRangesByEndpointResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: machineTag.String()}},
	}
	err := st.facade.FacadeCall("OpenedMachinePortRangesByEndpoint", args, &results)
	if err != nil {
		return nil, err
	}
	return processOpenPortRangesByEndpointResults(results, machineTag)
}

// OpenedPortRangesByEndpoint returns all port ranges currently opened grouped by unit tag and application endpoint.
func (st *State) OpenedPortRangesByEndpoint() (map[names.UnitTag]network.GroupedPortRanges, error) {
	if st.BestAPIVersion() < 18 {
		// OpenedPortRangesByEndpoint() was introduced in UniterAPIV18.
		return nil, errors.NotImplementedf("OpenedPortRangesByEndpoint() (need V18+)")
	}
	var results params.OpenPortRangesByEndpointResults
	if err := st.facade.FacadeCall("OpenedPortRangesByEndpoint", nil, &results); err != nil {
		return nil, errors.Trace(err)
	}
	return processOpenPortRangesByEndpointResults(results, st.unitTag)
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

// SLALevel returns the SLA level set on the model.
func (st *State) SLALevel() (string, error) {
	var result params.StringResult
	err := st.facade.FacadeCall("SLALevel", nil, &result)
	if err != nil {
		return "", errors.Trace(err)
	}
	if err := result.Error; err != nil {
		return "", errors.Trace(err)
	}
	return result.Result, nil
}

// CloudAPIVersion returns the API version of the cloud, if known.
func (st *State) CloudAPIVersion() (string, error) {
	var result params.StringResult
	err := st.facade.FacadeCall("CloudAPIVersion", nil, &result)
	if err != nil {
		return "", errors.Trace(err)
	}
	if err := result.Error; err != nil {
		return "", errors.Trace(err)
	}
	return result.Result, nil
}

// GoalState returns a GoalState struct with the charm's
// peers and related units information.
func (st *State) GoalState() (application.GoalState, error) {
	var result params.GoalStateResults

	gs := application.GoalState{}

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: st.unitTag.String()},
		},
	}

	err := st.facade.FacadeCall("GoalStates", args, &result)
	if err != nil {
		return gs, err
	}
	if len(result.Results) != 1 {
		return gs, errors.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return gs, err
	}
	gs = goalStateFromParams(result.Results[0].Result)
	return gs, nil
}

func goalStateFromParams(paramsGoalState *params.GoalState) application.GoalState {
	goalState := application.GoalState{}

	copyUnits := func(units params.UnitsGoalState) application.UnitsGoalState {
		copiedUnits := application.UnitsGoalState{}
		for name, gs := range units {
			copiedUnits[name] = application.GoalStateStatus{
				Status: gs.Status,
				Since:  gs.Since,
			}
		}
		return copiedUnits
	}

	goalState.Units = copyUnits(paramsGoalState.Units)

	if paramsGoalState.Relations != nil {
		goalState.Relations = make(map[string]application.UnitsGoalState)
		for relation, relationUnits := range paramsGoalState.Relations {
			goalState.Relations[relation] = copyUnits(relationUnits)
		}
	}

	return goalState
}

// GetPodSpec gets the pod spec of the specified application.
func (st *State) GetPodSpec(appName string) (string, error) {
	if !names.IsValidApplication(appName) {
		return "", errors.NotValidf("application name %q", appName)
	}
	tag := names.NewApplicationTag(appName)
	var result params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: tag.String(),
		}},
	}
	if err := st.facade.FacadeCall("GetPodSpec", args, &result); err != nil {
		return "", errors.Trace(err)
	}
	if len(result.Results) != 1 {
		return "", fmt.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		if params.IsCodeNotFound(result.Results[0].Error) {
			return "", errors.NotFoundf("podspec for application %s", appName)
		}
		return "", err
	}
	return result.Results[0].Result, nil
}

// GetRawK8sSpec gets the raw k8s spec of the specified application.
func (st *State) GetRawK8sSpec(appName string) (string, error) {
	if !names.IsValidApplication(appName) {
		return "", errors.NotValidf("application name %q", appName)
	}
	tag := names.NewApplicationTag(appName)
	var result params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: tag.String(),
		}},
	}
	if err := st.facade.FacadeCall("GetRawK8sSpec", args, &result); err != nil {
		return "", errors.Trace(err)
	}
	if len(result.Results) != 1 {
		return "", fmt.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		if params.IsCodeNotFound(result.Results[0].Error) {
			return "", errors.NotFoundf("raw k8s spec for application %s", appName)
		}
		return "", err
	}
	return result.Results[0].Result, nil
}

// CloudSpec returns the cloud spec for the model that calling unit or
// application resides in.
// If the application has not been authorised to access its cloud spec,
// then an authorisation error will be returned.
func (st *State) CloudSpec() (*params.CloudSpec, error) {
	var result params.CloudSpecResult

	err := st.facade.FacadeCall("CloudSpec", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, err
	}
	return result.Result, nil
}

// UnitWorkloadVersion returns the version of the workload reported by
// the specified unit.
func (st *State) UnitWorkloadVersion(tag names.UnitTag) (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	err := st.facade.FacadeCall("WorkloadVersion", args, &results)
	if err != nil {
		return "", err
	}
	if len(results.Results) != 1 {
		return "", fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}

// SetUnitWorkloadVersion sets the specified unit's workload version to
// the provided value.
func (st *State) SetUnitWorkloadVersion(tag names.UnitTag, version string) error {
	var result params.ErrorResults
	args := params.EntityWorkloadVersions{
		Entities: []params.EntityWorkloadVersion{
			{Tag: tag.String(), WorkloadVersion: version},
		},
	}
	err := st.facade.FacadeCall("SetWorkloadVersion", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}
