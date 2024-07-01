// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"fmt"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/types"
	apiwatcher "github.com/juju/juju/api/watcher"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

const uniterFacade = "Uniter"

// Client provides access to the Uniter API facade.
type Client struct {
	*common.ModelWatcher
	*common.APIAddresser
	*common.UpgradeSeriesAPI
	*common.UnitStateAPI
	*StorageAccessor

	leadershipSettings *LeadershipSettings
	facade             base.FacadeCaller
	// unitTag contains the authenticated unit's tag.
	unitTag names.UnitTag
}

// NewClient creates a new client-side Uniter facade.
func NewClient(
	caller base.APICaller,
	authTag names.UnitTag,
	options ...Option,
) *Client {
	facadeCaller := base.NewFacadeCaller(
		caller,
		uniterFacade,
		options...,
	)
	client := &Client{
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
	client.leadershipSettings = NewLeadershipSettings(
		facadeCaller.FacadeCall,
		newWatcher,
	)
	return client
}

// NewFromConnection returns a version of the Connection that provides
// functionality required by the uniter worker if possible else a non-nil error.
func NewFromConnection(c api.Connection) (*Client, error) {
	authTag := c.AuthTag()
	unitTag, ok := authTag.(names.UnitTag)
	if !ok {
		return nil, errors.Errorf("expected UnitTag, got %T %v", authTag, authTag)
	}
	return NewClient(c, unitTag), nil
}

// BestAPIVersion returns the API version that we were able to
// determine is supported by both the client and the API Server.
func (client *Client) BestAPIVersion() int {
	return client.facade.BestAPIVersion()
}

// life requests the lifecycle of the given entity from the server.
func (client *Client) life(ctx context.Context, tag names.Tag) (life.Value, error) {
	return common.OneLife(ctx, client.facade, tag)
}

// relation requests relation information from the server.
func (client *Client) relation(ctx context.Context, relationTag, unitTag names.Tag) (params.RelationResult, error) {
	nothing := params.RelationResult{}
	var result params.RelationResults
	args := params.RelationUnits{
		RelationUnits: []params.RelationUnit{
			{Relation: relationTag.String(), Unit: unitTag.String()},
		},
	}
	err := client.facade.FacadeCall(ctx, "Relation", args, &result)
	if err != nil {
		return nothing, errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(result.Results) != 1 {
		return nothing, fmt.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return nothing, err
	}
	return result.Results[0], nil
}

func (client *Client) setRelationStatus(ctx context.Context, id int, status relation.Status) error {
	args := params.RelationStatusArgs{
		Args: []params.RelationStatusArg{{
			UnitTag:    client.unitTag.String(),
			RelationId: id,
			Status:     params.RelationStatusValue(status),
		}},
	}
	var results params.ErrorResults
	if err := client.facade.FacadeCall(ctx, "SetRelationStatus", args, &results); err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
	}
	return results.OneError()
}

// getOneAction retrieves a single Action from the controller.
func (client *Client) getOneAction(ctx context.Context, tag *names.ActionTag) (params.ActionResult, error) {
	nothing := params.ActionResult{}

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	}

	var results params.ActionResults
	err := client.facade.FacadeCall(ctx, "Actions", args, &results)
	if err != nil {
		return nothing, errors.Trace(apiservererrors.RestoreError(err))
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

// LeadershipSettingsAccessor is an interface that allows us not to have
// to use the concrete `api/uniter/LeadershipSettings` type, thus
// simplifying testing.
type LeadershipSettingsAccessor interface {
	Read(applicationName string) (map[string]string, error)
	Merge(applicationName, unitName string, settings map[string]string) error
}

// LeadershipSettings returns the client's leadership settings api.
func (client *Client) LeadershipSettings() LeadershipSettingsAccessor {
	return client.leadershipSettings
}

// ActionStatus provides the status of a single action.
func (client *Client) ActionStatus(ctx context.Context, tag names.ActionTag) (string, error) {
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	}

	var results params.StringResults
	err := client.facade.FacadeCall(ctx, "ActionStatus", args, &results)
	if err != nil {
		return "", errors.Trace(apiservererrors.RestoreError(err))
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
func (client *Client) Unit(ctx context.Context, tag names.UnitTag) (*Unit, error) {
	unit := &Unit{
		tag:    tag,
		client: client,
	}
	err := unit.Refresh(ctx)
	if err != nil {
		return nil, err
	}
	return unit, nil
}

// Application returns an application state by tag.
func (client *Client) Application(ctx context.Context, tag names.ApplicationTag) (*Application, error) {
	life, err := client.life(ctx, tag)
	if err != nil {
		return nil, err
	}
	return &Application{
		tag:    tag,
		life:   life,
		client: client,
	}, nil
}

// ProviderType returns a provider type used by the current juju model.
//
// TODO(dimitern): We might be able to drop this, once we have machine
// addresses implemented fully. See also LP bug 1221798.
func (client *Client) ProviderType(ctx context.Context) (string, error) {
	var result params.StringResult
	err := client.facade.FacadeCall(ctx, "ProviderType", nil, &result)
	if err != nil {
		return "", errors.Trace(apiservererrors.RestoreError(err))
	}
	if err := result.Error; err != nil {
		return "", err
	}
	return result.Result, nil
}

// Charm returns the charm with the given URL.
func (client *Client) Charm(curl string) (*Charm, error) {
	if curl == "" {
		return nil, fmt.Errorf("charm url cannot be empty")
	}
	return &Charm{
		client: client,
		curl:   curl,
	}, nil
}

// Relation returns the existing relation with the given tag.
func (client *Client) Relation(ctx context.Context, relationTag names.RelationTag) (*Relation, error) {
	result, err := client.relation(ctx, relationTag, client.unitTag)
	if err != nil {
		return nil, err
	}
	return &Relation{
		id:        result.Id,
		tag:       relationTag,
		life:      result.Life,
		suspended: result.Suspended,
		client:    client,
		otherApp:  result.OtherApplication,
	}, nil
}

// Action returns the Action with the given tag.
func (client *Client) Action(ctx context.Context, tag names.ActionTag) (*Action, error) {
	result, err := client.getOneAction(ctx, &tag)
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
func (client *Client) ActionBegin(ctx context.Context, tag names.ActionTag) error {
	var outcome params.ErrorResults

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: tag.String()},
		},
	}

	err := client.facade.FacadeCall(ctx, "BeginActions", args, &outcome)
	if err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
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
func (client *Client) ActionFinish(ctx context.Context, tag names.ActionTag, status string, results map[string]interface{}, message string) error {
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

	err := client.facade.FacadeCall(ctx, "FinishActions", args, &outcome)
	if err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
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
func (client *Client) RelationById(ctx context.Context, id int) (*Relation, error) {
	var results params.RelationResults
	args := params.RelationIds{
		RelationIds: []int{id},
	}

	err := client.facade.FacadeCall(ctx, "RelationById", args, &results)
	if err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
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
		client:    client,
		otherApp:  result.OtherApplication,
	}, nil
}

// Model returns the model entity.
func (client *Client) Model(ctx context.Context) (*types.Model, error) {
	var result params.ModelResult
	err := client.facade.FacadeCall(ctx, "CurrentModel", nil, &result)
	if err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}
	if err := result.Error; err != nil {
		return nil, err
	}
	modelType := types.ModelType(result.Type)
	if modelType == "" {
		modelType = types.IAAS
	}
	return &types.Model{
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
			return nil, errors.Trace(apiservererrors.RestoreError(err))
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
func (client *Client) OpenedMachinePortRangesByEndpoint(ctx context.Context, machineTag names.MachineTag) (map[names.UnitTag]network.GroupedPortRanges, error) {
	var results params.OpenPortRangesByEndpointResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: machineTag.String()}},
	}
	err := client.facade.FacadeCall(ctx, "OpenedMachinePortRangesByEndpoint", args, &results)
	if err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}
	return processOpenPortRangesByEndpointResults(results, machineTag)
}

// OpenedPortRangesByEndpoint returns all port ranges currently opened grouped by unit tag and application endpoint.
func (client *Client) OpenedPortRangesByEndpoint(ctx context.Context) (map[names.UnitTag]network.GroupedPortRanges, error) {
	if client.BestAPIVersion() < 18 {
		// OpenedPortRangesByEndpoint() was introduced in UniterAPIV18.
		return nil, errors.NotImplementedf("OpenedPortRangesByEndpoint() (need V18+)")
	}
	var results params.OpenPortRangesByEndpointResults
	if err := client.facade.FacadeCall(ctx, "OpenedPortRangesByEndpoint", nil, &results); err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}
	return processOpenPortRangesByEndpointResults(results, client.unitTag)
}

// WatchRelationUnits returns a watcher that notifies of changes to the
// counterpart units in the relation for the given unit.
func (client *Client) WatchRelationUnits(
	ctx context.Context,
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
	err := client.facade.FacadeCall(ctx, "WatchRelationUnits", args, &results)
	if err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewRelationUnitsWatcher(client.facade.RawAPICaller(), result)
	return w, nil
}

// CloudAPIVersion returns the API version of the cloud, if known.
func (client *Client) CloudAPIVersion(ctx context.Context) (string, error) {
	var result params.StringResult
	err := client.facade.FacadeCall(ctx, "CloudAPIVersion", nil, &result)
	if err != nil {
		return "", errors.Trace(apiservererrors.RestoreError(err))
	}
	if err := result.Error; err != nil {
		return "", errors.Trace(err)
	}
	return result.Result, nil
}

// GoalState returns a GoalState struct with the charm's
// peers and related units information.
func (client *Client) GoalState(ctx context.Context) (application.GoalState, error) {
	var result params.GoalStateResults

	gs := application.GoalState{}

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: client.unitTag.String()},
		},
	}

	err := client.facade.FacadeCall(ctx, "GoalStates", args, &result)
	if err != nil {
		return gs, errors.Trace(apiservererrors.RestoreError(err))
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

// CloudSpec returns the cloud spec for the model that calling unit or
// application resides in.
// If the application has not been authorised to access its cloud spec,
// then an authorisation error will be returned.
func (client *Client) CloudSpec(ctx context.Context) (*params.CloudSpec, error) {
	var result params.CloudSpecResult

	err := client.facade.FacadeCall(ctx, "CloudSpec", nil, &result)
	if err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}
	if err := result.Error; err != nil {
		return nil, err
	}
	return result.Result, nil
}

// UnitWorkloadVersion returns the version of the workload reported by
// the specified unit.
func (client *Client) UnitWorkloadVersion(ctx context.Context, tag names.UnitTag) (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	err := client.facade.FacadeCall(ctx, "WorkloadVersion", args, &results)
	if err != nil {
		return "", errors.Trace(apiservererrors.RestoreError(err))
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
func (client *Client) SetUnitWorkloadVersion(ctx context.Context, tag names.UnitTag, version string) error {
	var result params.ErrorResults
	args := params.EntityWorkloadVersions{
		Entities: []params.EntityWorkloadVersion{
			{Tag: tag.String(), WorkloadVersion: version},
		},
	}
	err := client.facade.FacadeCall(ctx, "SetWorkloadVersion", args, &result)
	if err != nil {
		return errors.Trace(apiservererrors.RestoreError(err))
	}
	return result.OneError()
}
