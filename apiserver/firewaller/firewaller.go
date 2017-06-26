// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"net"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.firewaller")

// FirewallerAPIV3 provides access to the Firewaller v3 API facade.
type FirewallerAPIV3 struct {
	*common.LifeGetter
	*common.ModelWatcher
	*common.AgentEntityWatcher
	*common.UnitsWatcher
	*common.ModelMachinesWatcher
	*common.InstanceIdGetter
	cloudspec.CloudSpecAPI

	st                State
	resources         facade.Resources
	authorizer        facade.Authorizer
	accessUnit        common.GetAuthFunc
	accessApplication common.GetAuthFunc
	accessMachine     common.GetAuthFunc
	accessEnviron     common.GetAuthFunc
}

// FirewallerAPIV4 provides access to the Firewaller v4 API facade.
type FirewallerAPIV4 struct {
	*FirewallerAPIV3
}

// NewStateFirewallerAPIv3 creates a new server-side FirewallerAPIV3 facade.
func NewStateFirewallerAPIV3(context facade.Context) (*FirewallerAPIV3, error) {
	st := context.State()
	environConfigGetter := stateenvirons.EnvironConfigGetter{st}
	cloudSpecAPI := cloudspec.NewCloudSpec(environConfigGetter.CloudSpec, common.AuthFuncForTag(st.ModelTag()))
	return NewFirewallerAPI(stateShim{st}, context.Resources(), context.Auth(), cloudSpecAPI)
}

// NewStateFirewallerAPIv4 creates a new server-side FirewallerAPIV4 facade.
func NewStateFirewallerAPIV4(context facade.Context) (*FirewallerAPIV4, error) {
	facadev3, err := NewStateFirewallerAPIV3(context)
	if err != nil {
		return nil, err
	}
	return &FirewallerAPIV4{facadev3}, nil
}

// NewFirewallerAPI creates a new server-side FirewallerAPIV3 facade.
func NewFirewallerAPI(
	st State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	cloudSpecAPI cloudspec.CloudSpecAPI,
) (*FirewallerAPIV3, error) {
	if !authorizer.AuthController() {
		// Firewaller must run as environment manager.
		return nil, common.ErrPerm
	}
	// Set up the various authorization checkers.
	accessEnviron := common.AuthFuncForTagKind(names.ModelTagKind)
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
	// with unrestriced access.
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

	return &FirewallerAPIV3{
		LifeGetter:           lifeGetter,
		ModelWatcher:         modelWatcher,
		AgentEntityWatcher:   entityWatcher,
		UnitsWatcher:         unitsWatcher,
		ModelMachinesWatcher: machinesWatcher,
		InstanceIdGetter:     instanceIdGetter,
		CloudSpecAPI:         cloudSpecAPI,
		st:                   st,
		resources:            resources,
		authorizer:           authorizer,
		accessUnit:           accessUnit,
		accessApplication:    accessApplication,
		accessMachine:        accessMachine,
		accessEnviron:        accessEnviron,
	}, nil
}

// WatchOpenedPorts returns a new StringsWatcher for each given
// environment tag.
func (f *FirewallerAPIV3) WatchOpenedPorts(args params.Entities) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canWatch, err := f.accessEnviron()
	if err != nil {
		return params.StringsWatchResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if !canWatch(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		watcherId, initial, err := f.watchOneEnvironOpenedPorts(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		result.Results[i].StringsWatcherId = watcherId
		result.Results[i].Changes = initial
	}
	return result, nil
}

func (f *FirewallerAPIV3) watchOneEnvironOpenedPorts(tag names.Tag) (string, []string, error) {
	// NOTE: tag is ignored, as there is only one environment in the
	// state DB. Once this changes, change the code below accordingly.
	watch := f.st.WatchOpenedPorts()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return f.resources.Register(watch), changes, nil
	}
	return "", nil, watcher.EnsureErr(watch)
}

// GetMachinePorts returns the port ranges opened on a machine for the specified
// subnet as a map mapping port ranges to the tags of the units that opened
// them.
func (f *FirewallerAPIV3) GetMachinePorts(args params.MachinePortsParams) (params.MachinePortsResults, error) {
	result := params.MachinePortsResults{
		Results: make([]params.MachinePortsResult, len(args.Params)),
	}
	canAccess, err := f.accessMachine()
	if err != nil {
		return params.MachinePortsResults{}, err
	}
	for i, param := range args.Params {
		machineTag, err := names.ParseMachineTag(param.MachineTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		var subnetTag names.SubnetTag
		if param.SubnetTag != "" {
			subnetTag, err = names.ParseSubnetTag(param.SubnetTag)
			if err != nil {
				result.Results[i].Error = common.ServerError(err)
				continue
			}
		}
		machine, err := f.getMachine(canAccess, machineTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		ports, err := machine.OpenedPorts(subnetTag.Id())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if ports != nil {
			portRangeMap := ports.AllPortRanges()
			var portRanges []network.PortRange
			for portRange := range portRangeMap {
				portRanges = append(portRanges, portRange)
			}
			network.SortPortRanges(portRanges)

			for _, portRange := range portRanges {
				unitTag := names.NewUnitTag(portRangeMap[portRange]).String()
				result.Results[i].Ports = append(result.Results[i].Ports,
					params.MachinePortRange{
						UnitTag:   unitTag,
						PortRange: params.FromNetworkPortRange(portRange),
					})
			}
		}
	}
	return result, nil
}

// GetMachineActiveSubnets returns the tags of the all subnets that each machine
// (in args) has open ports on.
func (f *FirewallerAPIV3) GetMachineActiveSubnets(args params.Entities) (params.StringsResults, error) {
	result := params.StringsResults{
		Results: make([]params.StringsResult, len(args.Entities)),
	}
	canAccess, err := f.accessMachine()
	if err != nil {
		return params.StringsResults{}, err
	}
	for i, entity := range args.Entities {
		machineTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		machine, err := f.getMachine(canAccess, machineTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		ports, err := machine.AllPorts()
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		for _, port := range ports {
			subnetID := port.SubnetID()
			if subnetID != "" && !names.IsValidSubnet(subnetID) {
				// The error message below will look like e.g. `ports for
				// machine "0", subnet "bad" not valid`.
				err = errors.NotValidf("%s", ports)
				result.Results[i].Error = common.ServerError(err)
				continue
			} else if subnetID != "" && names.IsValidSubnet(subnetID) {
				subnetTag := names.NewSubnetTag(subnetID).String()
				result.Results[i].Result = append(result.Results[i].Result, subnetTag)
				continue
			}
			// TODO(dimitern): Empty subnet CIDRs for ports are still OK until
			// we can enforce it across all providers.
			result.Results[i].Result = append(result.Results[i].Result, "")
		}
	}
	return result, nil
}

// GetExposed returns the exposed flag value for each given application.
func (f *FirewallerAPIV3) GetExposed(args params.Entities) (params.BoolResults, error) {
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	canAccess, err := f.accessApplication()
	if err != nil {
		return params.BoolResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		application, err := f.getApplication(canAccess, tag)
		if err == nil {
			result.Results[i].Result = application.IsExposed()
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// GetAssignedMachine returns the assigned machine tag (if any) for
// each given unit.
func (f *FirewallerAPIV3) GetAssignedMachine(args params.Entities) (params.StringResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
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
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (f *FirewallerAPIV3) getEntity(canAccess common.AuthFunc, tag names.Tag) (state.Entity, error) {
	if !canAccess(tag) {
		return nil, common.ErrPerm
	}
	return f.st.FindEntity(tag)
}

func (f *FirewallerAPIV3) getUnit(canAccess common.AuthFunc, tag names.UnitTag) (*state.Unit, error) {
	entity, err := f.getEntity(canAccess, tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// unit.
	return entity.(*state.Unit), nil
}

func (f *FirewallerAPIV3) getApplication(canAccess common.AuthFunc, tag names.ApplicationTag) (*state.Application, error) {
	entity, err := f.getEntity(canAccess, tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// application.
	return entity.(*state.Application), nil
}

func (f *FirewallerAPIV3) getMachine(canAccess common.AuthFunc, tag names.MachineTag) (*state.Machine, error) {
	entity, err := f.getEntity(canAccess, tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// machine.
	return entity.(*state.Machine), nil
}

// WatchEgressAddressesForRelations creates a watcher that notifies when addresses, from which
// connections will originate for the relation, change.
// Each event contains the entire set of addresses which are required for ingress for the relation.
func (f *FirewallerAPIV4) WatchEgressAddressesForRelations(relations params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		make([]params.StringsWatchResult, len(relations.Entities)),
	}

	one := func(tag string) (id string, changes []string, _ error) {
		logger.Debugf("Watching egress addresses for %+v from model %v", tag, f.st.ModelUUID())

		relationTag, err := names.ParseRelationTag(tag)
		if err != nil {
			return "", nil, errors.Trace(err)
		}

		// Load the relation details for the current token.
		localEndpoint, err := f.localApplication(relationTag)
		if err != nil {
			return "", nil, errors.Trace(err)
		}

		w, err := NewIngressAddressWatcher(f.st, localEndpoint.relation, localEndpoint.application)
		if err != nil {
			return "", nil, errors.Trace(err)
		}

		// TODO(wallyworld) - we will need to watch subnets too, but only
		// when we support using cloud local addresses
		//filter := func(id interface{}) bool {
		//	include, err := includeAsIngressSubnet(id.(string))
		//	if err != nil {
		//		logger.Warningf("invalid CIDR %q", id)
		//	}
		//	return include
		//}
		//w := api.st.WatchSubnets(filter)

		changes, ok := <-w.Changes()
		if !ok {
			return "", nil, common.ServerError(watcher.EnsureErr(w))
		}
		return f.resources.Register(w), changes, nil
	}

	for i, e := range relations.Entities {
		watcherId, changes, err := one(e.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].StringsWatcherId = watcherId
		results.Results[i].Changes = changes
	}
	return results, nil
}

type localEndpointInfo struct {
	relation    Relation
	application string
	name        string
	role        charm.RelationRole
}

func (f *FirewallerAPIV3) localApplication(relationTag names.RelationTag) (*localEndpointInfo, error) {
	rel, err := f.st.KeyRelation(relationTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Gather info about the local (this model) application of the relation.
	// We'll use the info to figure out what addresses/subnets to include.
	localEndpoint := localEndpointInfo{relation: rel}
	for _, ep := range rel.Endpoints() {
		// Try looking up the info for the local application.
		_, err = f.st.Application(ep.ApplicationName)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		} else if err == nil {
			localEndpoint.application = ep.ApplicationName
			localEndpoint.name = ep.Name
			localEndpoint.role = ep.Role
			break
		}
	}
	// Until networking support becomes more sophisticated, for now
	// we only care about opening access to applications with an endpoint
	// having the "provider" role. The assumption is that such endpoints listen
	// to incoming connections and thus require ingress. An exception to this
	// would be applications which accept connections onto an endpoint which
	// has a "requirer" role.
	// We are operating in the model hosting the "consuming" application, so check
	// that its endpoint has the "requirer" role, meaning that we need to notify
	// the offering model of subnets from this model required for ingress.
	if localEndpoint.role != charm.RoleRequirer {
		return nil, errors.NotSupportedf(
			"egress network for application %v without requires endpoint", localEndpoint.application)
	}
	return &localEndpoint, nil
}

// TODO(wallyworld) - this is unused until we query subnets again
func includeAsEgressSubnet(cidr string) (bool, error) {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, errors.Trace(err)
	}
	if ip.IsLoopback() || ip.IsMulticast() {
		return false, nil
	}
	// TODO(wallyworld) - We only support IPv4 addresses as not all providers support IPv6.
	if ip.To4() == nil {
		return false, nil
	}
	return true, nil
}
