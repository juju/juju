// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"
	"sort"
	"strconv"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/common/firewall"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// FirewallerAPI provides access to the Firewaller API facade.
type FirewallerAPI struct {
	*common.LifeGetter
	*common.ModelWatcher
	*common.AgentEntityWatcher
	*common.UnitsWatcher
	*common.ModelMachinesWatcher
	*common.InstanceIdGetter
	ControllerConfigAPI
	cloudspec.CloudSpecer

	st                State
	resources         facade.Resources
	authorizer        facade.Authorizer
	accessUnit        common.GetAuthFunc
	accessApplication common.GetAuthFunc
	accessMachine     common.GetAuthFunc
	accessModel       common.GetAuthFunc
	logger            loggo.Logger

	// Fetched on demand and memoized
	spaceInfos          network.SpaceInfos
	appEndpointBindings map[string]map[string]string
}

// NewStateFirewallerAPI creates a new server-side FirewallerAPIV7 facade.
func NewStateFirewallerAPI(
	st State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	cloudSpecAPI cloudspec.CloudSpecer,
	controllerConfigAPI ControllerConfigAPI,
	logger loggo.Logger,
) (*FirewallerAPI, error) {
	if !authorizer.AuthController() {
		// Firewaller must run as a controller.
		return nil, apiservererrors.ErrPerm
	}
	// Set up the various authorization checkers.
	accessModel := common.AuthFuncForTagKind(names.ModelTagKind)
	accessUnit := common.AuthFuncForTagKind(names.UnitTagKind)
	accessApplication := common.AuthFuncForTagKind(names.ApplicationTagKind)
	accessMachine := common.AuthFuncForTagKind(names.MachineTagKind)
	accessRelation := common.AuthFuncForTagKind(names.RelationTagKind)
	accessUnitApplicationOrMachineOrRelation := common.AuthAny(accessUnit, accessApplication, accessMachine, accessRelation)

	// Life() is supported for units, applications or machines.
	lifeGetter := common.NewLifeGetter(
		st,
		accessUnitApplicationOrMachineOrRelation,
	)
	// ModelConfig() and WatchForModelConfigChanges() are allowed
	// with unrestricted access.
	modelWatcher := common.NewModelWatcher(
		st,
		resources,
		authorizer,
	)
	// Watch() is supported for applications only.
	entityWatcher := common.NewAgentEntityWatcher(
		st,
		resources,
		accessApplication,
	)
	// WatchUnits() is supported for machines.
	unitsWatcher := common.NewUnitsWatcher(st,
		resources,
		accessMachine,
	)
	// WatchModelMachines() is allowed with unrestricted access.
	machinesWatcher := common.NewModelMachinesWatcher(
		st,
		resources,
		authorizer,
	)
	// InstanceId() is supported for machines.
	instanceIdGetter := common.NewInstanceIdGetter(
		st,
		accessMachine,
	)

	return &FirewallerAPI{
		LifeGetter:           lifeGetter,
		ModelWatcher:         modelWatcher,
		AgentEntityWatcher:   entityWatcher,
		UnitsWatcher:         unitsWatcher,
		ModelMachinesWatcher: machinesWatcher,
		InstanceIdGetter:     instanceIdGetter,
		CloudSpecer:          cloudSpecAPI,
		ControllerConfigAPI:  controllerConfigAPI,
		st:                   st,
		resources:            resources,
		authorizer:           authorizer,
		accessUnit:           accessUnit,
		accessApplication:    accessApplication,
		accessMachine:        accessMachine,
		accessModel:          accessModel,
		logger:               logger,
	}, nil
}

// WatchOpenedPorts returns a new StringsWatcher for each given
// model tag.
func (f *FirewallerAPI) WatchOpenedPorts(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canWatch, err := f.accessModel()
	if err != nil {
		return params.StringsWatchResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canWatch(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		watcherId, initial, err := f.watchOneModelOpenedPorts(tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].StringsWatcherId = watcherId
		result.Results[i].Changes = initial
	}
	return result, nil
}

func (f *FirewallerAPI) watchOneModelOpenedPorts(tag names.Tag) (string, []string, error) {
	// NOTE: tag is ignored, as there is only one model in the
	// state DB. Once this changes, change the code below accordingly.
	watch := f.st.WatchOpenedPorts()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return f.resources.Register(watch), changes, nil
	}
	return "", nil, watcher.EnsureErr(watch)
}

// ModelFirewallRules returns the firewall rules that this model is
// configured to open
func (f *FirewallerAPI) ModelFirewallRules(ctx context.Context) (params.IngressRulesResult, error) {
	cfg, err := f.st.ModelConfig(ctx)
	if err != nil {
		return params.IngressRulesResult{Error: apiservererrors.ServerError(err)}, nil
	}
	ctrlCfg, err := f.st.ControllerConfig()
	if err != nil {
		return params.IngressRulesResult{Error: apiservererrors.ServerError(err)}, nil
	}
	isController := f.st.IsController()

	var rules []params.IngressRule
	sshAllow := cfg.SSHAllow()
	if len(sshAllow) != 0 {
		portRange := params.FromNetworkPortRange(network.MustParsePortRange("22"))
		rules = append(rules, params.IngressRule{PortRange: portRange, SourceCIDRs: sshAllow})
	}
	if isController {
		portRange := params.FromNetworkPortRange(network.MustParsePortRange(strconv.Itoa(ctrlCfg.APIPort())))
		rules = append(rules, params.IngressRule{PortRange: portRange, SourceCIDRs: []string{"0.0.0.0/0", "::/0"}})
	}
	if isController && ctrlCfg.AutocertDNSName() != "" {
		portRange := params.FromNetworkPortRange(network.MustParsePortRange("80"))
		rules = append(rules, params.IngressRule{PortRange: portRange, SourceCIDRs: []string{"0.0.0.0/0", "::/0"}})
	}
	return params.IngressRulesResult{
		Rules: rules,
	}, nil
}

// WatchModelFirewallRules returns a NotifyWatcher that notifies of
// potential changes to a model's configured firewall rules
func (f *FirewallerAPI) WatchModelFirewallRules(ctx context.Context) (params.NotifyWatchResult, error) {
	watch, err := NewModelFirewallRulesWatcher(f.st)
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}, nil
	}

	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{NotifyWatcherId: f.resources.Register(watch)}, nil
	} else {
		err := watcher.EnsureErr(watch)
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}, nil
	}
}

// GetAssignedMachine returns the assigned machine tag (if any) for
// each given unit.
func (f *FirewallerAPI) GetAssignedMachine(ctx context.Context, args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := f.accessUnit()
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unit, err := f.getUnit(canAccess, tag)
		if err == nil {
			var machineId string
			machineId, err = unit.AssignedMachineId()
			if err == nil {
				result.Results[i].Result = names.NewMachineTag(machineId).String()
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// getSpaceInfos returns the cached SpaceInfos or retrieves them from state
// and memoizes it for future invocations.
func (f *FirewallerAPI) getSpaceInfos() (network.SpaceInfos, error) {
	if f.spaceInfos != nil {
		return f.spaceInfos, nil
	}

	si, err := f.st.SpaceInfos()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return si, nil
}

// getApplicationBindings returns the cached endpoint bindings for all model
// applications grouped by app name. If the application endpoints have not yet
// been retrieved they will be retrieved and memoized for future calls.
func (f *FirewallerAPI) getApplicationBindings() (map[string]map[string]string, error) {
	if f.appEndpointBindings == nil {
		bindings, err := f.st.AllEndpointBindings()
		if err != nil {
			return nil, errors.Trace(err)
		}
		f.appEndpointBindings = bindings
	}

	return f.appEndpointBindings, nil
}

func (f *FirewallerAPI) getEntity(canAccess common.AuthFunc, tag names.Tag) (state.Entity, error) {
	if !canAccess(tag) {
		return nil, apiservererrors.ErrPerm
	}
	return f.st.FindEntity(tag)
}

func (f *FirewallerAPI) getUnit(canAccess common.AuthFunc, tag names.UnitTag) (*state.Unit, error) {
	entity, err := f.getEntity(canAccess, tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// unit.
	return entity.(*state.Unit), nil
}

func (f *FirewallerAPI) getApplication(canAccess common.AuthFunc, tag names.ApplicationTag) (*state.Application, error) {
	entity, err := f.getEntity(canAccess, tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// application.
	return entity.(*state.Application), nil
}

func (f *FirewallerAPI) getMachine(canAccess common.AuthFunc, tag names.MachineTag) (firewall.Machine, error) {
	if !canAccess(tag) {
		return nil, apiservererrors.ErrPerm
	}
	return f.st.Machine(tag.Id())
}

// WatchEgressAddressesForRelations creates a watcher that notifies when addresses, from which
// connections will originate for the relation, change.
// Each event contains the entire set of addresses which are required for ingress for the relation.
func (f *FirewallerAPI) WatchEgressAddressesForRelations(ctx context.Context, relations params.Entities) (params.StringsWatchResults, error) {
	return firewall.WatchEgressAddressesForRelations(f.resources, f.st, relations)
}

// WatchIngressAddressesForRelations creates a watcher that returns the ingress networks
// that have been recorded against the specified relations.
func (f *FirewallerAPI) WatchIngressAddressesForRelations(ctx context.Context, relations params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		make([]params.StringsWatchResult, len(relations.Entities)),
	}

	one := func(tag string) (id string, changes []string, _ error) {
		f.logger.Debugf("Watching ingress addresses for %+v from model %v", tag, f.st.ModelUUID())

		relationTag, err := names.ParseRelationTag(tag)
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		rel, err := f.st.KeyRelation(relationTag.Id())
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		w := rel.WatchRelationIngressNetworks()
		changes, ok := <-w.Changes()
		if !ok {
			return "", nil, apiservererrors.ServerError(watcher.EnsureErr(w))
		}
		return f.resources.Register(w), changes, nil
	}

	for i, e := range relations.Entities {
		watcherId, changes, err := one(e.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].StringsWatcherId = watcherId
		results.Results[i].Changes = changes
	}
	return results, nil
}

// MacaroonForRelations returns the macaroon for the specified relations.
func (f *FirewallerAPI) MacaroonForRelations(ctx context.Context, args params.Entities) (params.MacaroonResults, error) {
	var result params.MacaroonResults
	result.Results = make([]params.MacaroonResult, len(args.Entities))
	for i, entity := range args.Entities {
		relationTag, err := names.ParseRelationTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		mac, err := f.st.GetMacaroon(relationTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result = mac
	}
	return result, nil
}

// SetRelationsStatus sets the status for the specified relations.
func (f *FirewallerAPI) SetRelationsStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	var result params.ErrorResults
	result.Results = make([]params.ErrorResult, len(args.Entities))
	for i, entity := range args.Entities {
		relationTag, err := names.ParseRelationTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		rel, err := f.st.KeyRelation(relationTag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = rel.SetStatus(status.StatusInfo{
			Status:  status.Status(entity.Status),
			Message: entity.Info,
		}, nil)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// AreManuallyProvisioned returns whether each given entity is
// manually provisioned or not. Only machine tags are accepted.
func (f *FirewallerAPI) AreManuallyProvisioned(ctx context.Context, args params.Entities) (params.BoolResults, error) {
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	canAccess, err := f.accessMachine()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		machineTag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		machine, err := f.getMachine(canAccess, machineTag)
		if err == nil {
			result.Results[i].Result, err = machine.IsManual()
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// OpenedMachinePortRanges returns a list of the opened port ranges for the
// specified machines where each result is broken down by unit. The list of
// opened ports for each unit is further grouped by endpoint name and includes
// the subnet CIDRs that belong to the space that each endpoint is bound to.
func (f *FirewallerAPI) OpenedMachinePortRanges(ctx context.Context, args params.Entities) (params.OpenMachinePortRangesResults, error) {
	result := params.OpenMachinePortRangesResults{
		Results: make([]params.OpenMachinePortRangesResult, len(args.Entities)),
	}
	canAccess, err := f.accessMachine()
	if err != nil {
		return result, err
	}

	for i, arg := range args.Entities {
		machineTag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		machine, err := f.getMachine(canAccess, machineTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		unitPortRanges, err := f.openedPortRangesForOneMachine(machine)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue

		}
		result.Results[i].UnitPortRanges = unitPortRanges
	}
	return result, nil
}

func (f *FirewallerAPI) openedPortRangesForOneMachine(machine firewall.Machine) (map[string][]params.OpenUnitPortRanges, error) {
	machPortRanges, err := machine.OpenedPortRanges()
	if err != nil {
		return nil, errors.Trace(err)
	}

	portRangesByUnit := machPortRanges.ByUnit()
	if len(portRangesByUnit) == 0 { // no ports open
		return nil, nil
	}

	// Look up space to subnet mappings
	spaceInfos, err := f.getSpaceInfos()
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnetCIDRsBySpaceID := spaceInfos.SubnetCIDRsBySpaceID()

	// Fetch application endpoint bindings
	allApps := set.NewStrings()
	for unitName := range portRangesByUnit {
		appName, err := names.UnitApplication(unitName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		allApps.Add(appName)
	}
	allAppBindings, err := f.getApplicationBindings()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Map the port ranges for each unit to one or more subnet CIDRs
	// depending on the endpoints they apply to.
	res := make(map[string][]params.OpenUnitPortRanges)
	for unitName, unitPortRanges := range portRangesByUnit {
		// Already checked for validity; error can be ignored
		appName, _ := names.UnitApplication(unitName)
		appBindings := allAppBindings[appName]

		unitTag := names.NewUnitTag(unitName).String()
		res[unitTag] = mapUnitPortsAndResolveSubnetCIDRs(unitPortRanges.ByEndpoint(), appBindings, subnetCIDRsBySpaceID)
	}

	return res, nil
}

// mapUnitPortsAndResolveSubnetCIDRs maps the provided list of opened port
// ranges by endpoint to a params.OpenUnitPortRanges result list. Each entry in
// the result list also contains the subnet CIDRs that correspond to each
// endpoint.
//
// To resolve the subnet CIDRs, the function consults the application endpoint
// bindings for the unit in conjunction with the provided subnetCIDRs by
// spaceID map. Using this information, each endpoint from the incoming port
// range grouping is resolved to a space ID and the space ID is in turn
// resolved into a list of subnet CIDRs (the wildcard endpoint is treated as
// *all known* endpoints for this conversion step).
func mapUnitPortsAndResolveSubnetCIDRs(portRangesByEndpoint network.GroupedPortRanges, endpointBindings map[string]string, subnetCIDRsBySpaceID map[string][]string) []params.OpenUnitPortRanges {
	var entries []params.OpenUnitPortRanges

	for endpointName, portRanges := range portRangesByEndpoint {
		entry := params.OpenUnitPortRanges{
			Endpoint:   endpointName,
			PortRanges: make([]params.PortRange, len(portRanges)),
		}

		// These port ranges target an explicit endpoint; just iterate
		// the subnets that correspond to the space it is bound to and
		// append their CIDRs.
		if endpointName != "" {
			entry.SubnetCIDRs = subnetCIDRsBySpaceID[endpointBindings[endpointName]]
			sort.Strings(entry.SubnetCIDRs)
		} else {
			// The wildcard endpoint expands to all known endpoints.
			for boundEndpoint, spaceID := range endpointBindings {
				if boundEndpoint == "" { // ignore default endpoint entry in the set of app bindings
					continue
				}
				entry.SubnetCIDRs = append(entry.SubnetCIDRs, subnetCIDRsBySpaceID[spaceID]...)
			}

			// Ensure that any duplicate CIDRs are removed.
			entry.SubnetCIDRs = set.NewStrings(entry.SubnetCIDRs...).SortedValues()
		}

		// Finally, map the port ranges to params.PortRange and
		network.SortPortRanges(portRanges)
		for i, pr := range portRanges {
			entry.PortRanges[i] = params.FromNetworkPortRange(pr)
		}

		entries = append(entries, entry)
	}

	// Ensure results are sorted by endpoint name to be consistent.
	sort.Slice(entries, func(a, b int) bool {
		return entries[a].Endpoint < entries[b].Endpoint
	})

	return entries
}

// GetExposeInfo returns the expose flag and per-endpoint expose settings
// for the specified applications.
func (f *FirewallerAPI) GetExposeInfo(ctx context.Context, args params.Entities) (params.ExposeInfoResults, error) {
	canAccess, err := f.accessApplication()
	if err != nil {
		return params.ExposeInfoResults{}, err
	}

	result := params.ExposeInfoResults{
		Results: make([]params.ExposeInfoResult, len(args.Entities)),
	}

	for i, entity := range args.Entities {
		tag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		application, err := f.getApplication(canAccess, tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if !application.IsExposed() {
			continue
		}

		result.Results[i].Exposed = true
		if exposedEndpoints := application.ExposedEndpoints(); len(exposedEndpoints) != 0 {
			mappedEndpoints := make(map[string]params.ExposedEndpoint)
			for endpoint, exposeDetails := range exposedEndpoints {
				mappedEndpoints[endpoint] = params.ExposedEndpoint{
					ExposeToSpaces: exposeDetails.ExposeToSpaceIDs,
					ExposeToCIDRs:  exposeDetails.ExposeToCIDRs,
				}
			}
			result.Results[i].ExposedEndpoints = mappedEndpoints
		}
	}
	return result, nil
}

// SpaceInfos returns a comprehensive representation of either all spaces or
// a filtered subset of the known spaces and their associated subnet details.
func (f *FirewallerAPI) SpaceInfos(ctx context.Context, args params.SpaceInfosParams) (params.SpaceInfos, error) {
	if !f.authorizer.AuthController() {
		return params.SpaceInfos{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}

	allSpaceInfos, err := f.getSpaceInfos()
	if err != nil {
		return params.SpaceInfos{}, apiservererrors.ServerError(err)
	}

	// Apply filtering if required
	if len(args.FilterBySpaceIDs) != 0 {
		var (
			filteredList network.SpaceInfos
			selectList   = set.NewStrings(args.FilterBySpaceIDs...)
		)
		for _, si := range allSpaceInfos {
			if selectList.Contains(si.ID) {
				filteredList = append(filteredList, si)
			}
		}

		allSpaceInfos = filteredList
	}

	return params.FromNetworkSpaceInfos(allSpaceInfos), nil
}

// WatchSubnets returns a new StringsWatcher that watches the specified
// subnet tags or all tags if no entities are specified.
func (f *FirewallerAPI) WatchSubnets(ctx context.Context, args params.Entities) (params.StringsWatchResult, error) {
	if !f.authorizer.AuthController() {
		return params.StringsWatchResult{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}

	var (
		filterFn  func(id interface{}) bool
		filterSet set.Strings
		result    = params.StringsWatchResult{}
	)

	if len(args.Entities) != 0 {
		filterSet = set.NewStrings()
		for _, arg := range args.Entities {
			subnetTag, err := names.ParseSubnetTag(arg.Tag)
			if err != nil {
				return params.StringsWatchResult{}, apiservererrors.ServerError(err)
			}

			filterSet.Add(subnetTag.Id())
		}

		filterFn = func(id interface{}) bool {
			return filterSet.Contains(id.(string))
		}
	}

	watcherId, initial, err := f.watchModelSubnets(filterFn)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	result.StringsWatcherId = watcherId
	result.Changes = initial
	return result, nil
}

func (f *FirewallerAPI) watchModelSubnets(filterFn func(interface{}) bool) (string, []string, error) {
	watch := f.st.WatchSubnets(filterFn)

	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return f.resources.Register(watch), changes, nil
	}
	return "", nil, watcher.EnsureErr(watch)
}

func setEquals(a, b set.Strings) bool {
	if a.Size() != b.Size() {
		return false
	}
	return a.Intersection(b).Size() == a.Size()
}
