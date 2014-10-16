// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/tools"
)

// FullStatus gives the information needed for juju status over the api
func (c *Client) FullStatus(args params.StatusParams) (api.Status, error) {
	cfg, err := c.api.state.EnvironConfig()
	if err != nil {
		return api.Status{}, errors.Annotate(err, "could not get environ config")
	}
	var noStatus api.Status
	var context statusContext
	if context.services, context.units, context.latestCharms, err =
		fetchAllServicesAndUnits(c.api.state); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch services and units")
	} else if context.machines, err = fetchMachines(c.api.state, nil); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch machines")
	} else if context.relations, err = fetchRelations(c.api.state); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch relations")
	} else if context.networks, err = fetchNetworks(c.api.state); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch networks")
	}

	if len(args.Patterns) > 0 {
		predicate := BuildPredicateFor(args.Patterns)

		// Filter machines
		for status, machList := range context.machines {
			for idx, mach := range machList {
				if matches, err := predicate(mach); err != nil {
					return noStatus, errors.Annotate(err, "could not filter machines")
				} else if !matches {
					// TODO(katco-): Check for index errors.
					context.machines[status] = append(machList[:idx], machList[idx+1:]...)
				}
			}
		}

		// Filter units
		unitChainPredicate := unitChainPredicateFn(predicate, context.unitByName)
		for _, unitMap := range context.units {
			for name, unit := range unitMap {
				// Always start examining at the top-level. This
				// prevents a situation where we filter a subordinate
				// before we discover it's parent is a match.
				if !unit.IsPrincipal() {
					continue
				}
				if matches, err := unitChainPredicate(unit); err != nil {
					return noStatus, errors.Annotate(err, "could not filter units")
				} else if !matches {
					delete(unitMap, name)
				}
			}
		}
	}

	return api.Status{
		EnvironmentName: cfg.Name(),
		Machines:        context.processMachines(),
		Services:        context.processServices(),
		Networks:        context.processNetworks(),
		Relations:       context.processRelations(),
	}, nil
}

// unitChainPredicateFn builds a function which runs the given
// predicate over a unit and all of its subordinates. If one unit in
// the chain matches, the entire chain matches.
func unitChainPredicateFn(
	predicate Predicate,
	getUnit func(string) *state.Unit,
) func(*state.Unit) (bool, error) {
	considered := make(map[string]bool)
	var f func(unit *state.Unit) (bool, error)
	f = func(unit *state.Unit) (bool, error) {
		// Don't try and filter the same unit 2x.
		if matches, ok := considered[unit.Name()]; ok {
			logger.Debugf("%s has already been examined and found to be: %t", unit.Name(), matches)
			return matches, nil
		}

		// Check the current unit.
		matches, err := predicate(unit)
		if err != nil {
			return false, errors.Annotate(err, "could not filter units")
		}
		considered[unit.Name()] = matches

		// Now check all of this unit's subordinates.
		for _, subName := range unit.SubordinateNames() {
			// A master match supercedes any subordinate match.
			if matches {
				logger.Debugf("%s is a subordinate to a match.", subName)
				considered[subName] = true
				continue
			}

			subUnit := getUnit(subName)
			if subUnit == nil {
				// We have already deleted this unit
				matches = false
				continue
			}
			matches, err = f(subUnit)
			if err != nil {
				return false, err
			}
			considered[subName] = matches
		}

		return matches, nil
	}
	return f
}

// Status is a stub version of FullStatus that was introduced in 1.16
func (c *Client) Status() (api.LegacyStatus, error) {
	var legacyStatus api.LegacyStatus
	status, err := c.FullStatus(params.StatusParams{})
	if err != nil {
		return legacyStatus, err
	}

	legacyStatus.Machines = make(map[string]api.LegacyMachineStatus)
	for machineName, machineStatus := range status.Machines {
		legacyStatus.Machines[machineName] = api.LegacyMachineStatus{
			InstanceId: string(machineStatus.InstanceId),
		}
	}
	return legacyStatus, nil
}

type statusContext struct {
	machines     map[string][]*state.Machine
	services     map[string]*state.Service
	relations    map[string][]*state.Relation
	units        map[string]map[string]*state.Unit
	networks     map[string]*state.Network
	latestCharms map[charm.URL]string
}

// fetchMachines returns a map from top level machine id to machines, where machines[0] is the host
// machine and machines[1..n] are any containers (including nested ones).
//
// If machineIds is non-nil, only machines whose IDs are in the set are returned.
func fetchMachines(st *state.State, machineIds *set.Strings) (map[string][]*state.Machine, error) {
	v := make(map[string][]*state.Machine)
	machines, err := st.AllMachines()
	if err != nil {
		return nil, err
	}
	// AllMachines gives us machines sorted by id.
	for _, m := range machines {
		if machineIds != nil && !machineIds.Contains(m.Id()) {
			continue
		}
		parentId, ok := m.ParentId()
		if !ok {
			// Only top level host machines go directly into the machine map.
			v[m.Id()] = []*state.Machine{m}
		} else {
			topParentId := state.TopParentId(m.Id())
			machines, ok := v[topParentId]
			if !ok {
				panic(fmt.Errorf("unexpected machine id %q", parentId))
			}
			machines = append(machines, m)
			v[topParentId] = machines
		}
	}
	return v, nil
}

// fetchAllServicesAndUnits returns a map from service name to service,
// a map from service name to unit name to unit, and a map from base charm URL to latest URL.
func fetchAllServicesAndUnits(
	st *state.State,
) (map[string]*state.Service, map[string]map[string]*state.Unit, map[charm.URL]string, error) {

	svcMap := make(map[string]*state.Service)
	unitMap := make(map[string]map[string]*state.Unit)
	latestCharms := make(map[charm.URL]string)
	services, err := st.AllServices()
	if err != nil {
		return nil, nil, nil, err
	}
	for _, s := range services {
		units, err := s.AllUnits()
		if err != nil {
			return nil, nil, nil, err
		}
		svcUnitMap := make(map[string]*state.Unit)
		for _, u := range units {
			svcUnitMap[u.Name()] = u
		}
		if len(svcUnitMap) > 0 {
			unitMap[s.Name()] = svcUnitMap
			svcMap[s.Name()] = s
			// Record the base URL for the service's charm so that
			// the latest store revision can be looked up.
			charmURL, _ := s.CharmURL()
			if charmURL.Schema == "cs" {
				latestCharms[*charmURL.WithRevision(-1)] = ""
			}
		}
	}
	for baseURL, _ := range latestCharms {
		ch, err := st.LatestPlaceholderCharm(&baseURL)
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, nil, nil, err
		}
		latestCharms[baseURL] = ch.String()
	}
	return svcMap, unitMap, latestCharms, nil
}

// fetchUnitMachineIds returns a set of IDs for machines that
// the specified units reside on, and those machines' ancestors.
func fetchUnitMachineIds(units map[string]map[string]*state.Unit) (*set.Strings, error) {
	machineIds := new(set.Strings)
	for _, svcUnitMap := range units {
		for _, unit := range svcUnitMap {
			if !unit.IsPrincipal() {
				continue
			}
			mid, err := unit.AssignedMachineId()
			if err != nil {
				return nil, err
			}
			for mid != "" {
				machineIds.Add(mid)
				mid = state.ParentId(mid)
			}
		}
	}
	return machineIds, nil
}

// fetchRelations returns a map of all relations keyed by service name.
//
// This structure is useful for processServiceRelations() which needs
// to have the relations for each service. Reading them once here
// avoids the repeated DB hits to retrieve the relations for each
// service that used to happen in processServiceRelations().
func fetchRelations(st *state.State) (map[string][]*state.Relation, error) {
	relations, err := st.AllRelations()
	if err != nil {
		return nil, err
	}
	out := make(map[string][]*state.Relation)
	for _, relation := range relations {
		for _, ep := range relation.Endpoints() {
			out[ep.ServiceName] = append(out[ep.ServiceName], relation)
		}
	}
	return out, nil
}

// fetchNetworks returns a map from network name to network.
func fetchNetworks(st *state.State) (map[string]*state.Network, error) {
	networks, err := st.AllNetworks()
	if err != nil {
		return nil, err
	}
	out := make(map[string]*state.Network)
	for _, n := range networks {
		out[n.Name()] = n
	}
	return out, nil
}

func (context *statusContext) processMachines() map[string]api.MachineStatus {
	machinesMap := make(map[string]api.MachineStatus)
	for id, machines := range context.machines {
		for mn, m := range machines {
			hostStatus := context.makeMachineStatus(m)
			context.processMachine(machines, &hostStatus, mn)
			machinesMap[id] = hostStatus
		}
	}
	return machinesMap
}

func (context *statusContext) processMachine(machines []*state.Machine, host *api.MachineStatus, startIndex int) (nextIndex int) {
	nextIndex = startIndex + 1
	currentHost := host
	var previousContainer *api.MachineStatus
	for nextIndex < len(machines) {
		machine := machines[nextIndex]
		container := context.makeMachineStatus(machine)
		if currentHost.Id == state.ParentId(machine.Id()) {
			currentHost.Containers[machine.Id()] = container
			previousContainer = &container
			nextIndex++
		} else {
			if state.NestingLevel(machine.Id()) > state.NestingLevel(previousContainer.Id) {
				nextIndex = context.processMachine(machines, previousContainer, nextIndex-1)
			} else {
				break
			}
		}
	}
	return
}

func (context *statusContext) makeMachineStatus(machine *state.Machine) (status api.MachineStatus) {
	status.Id = machine.Id()
	status.Agent, status.AgentState, status.AgentStateInfo = processAgent(machine)
	status.AgentVersion = status.Agent.Version
	status.Life = status.Agent.Life
	status.Err = status.Agent.Err
	status.Series = machine.Series()
	status.Jobs = paramsJobsFromJobs(machine.Jobs())
	status.WantsVote = machine.WantsVote()
	status.HasVote = machine.HasVote()
	instid, err := machine.InstanceId()
	if err == nil {
		status.InstanceId = instid
		status.InstanceState, err = machine.InstanceStatus()
		if err != nil {
			status.InstanceState = "error"
		}
		status.DNSName = network.SelectPublicAddress(machine.Addresses())
	} else {
		if state.IsNotProvisionedError(err) {
			status.InstanceId = "pending"
		} else {
			status.InstanceId = "error"
		}
		// There's no point in reporting a pending agent state
		// if the machine hasn't been provisioned. This
		// also makes unprovisioned machines visually distinct
		// in the output.
		status.AgentState = ""
	}
	hc, err := machine.HardwareCharacteristics()
	if err != nil {
		if !errors.IsNotFound(err) {
			status.Hardware = "error"
		}
	} else {
		status.Hardware = hc.String()
	}
	status.Containers = make(map[string]api.MachineStatus)
	return
}

func (context *statusContext) processRelations() []api.RelationStatus {
	var out []api.RelationStatus
	relations := context.getAllRelations()
	for _, relation := range relations {
		var eps []api.EndpointStatus
		var scope charm.RelationScope
		var relationInterface string
		for _, ep := range relation.Endpoints() {
			eps = append(eps, api.EndpointStatus{
				ServiceName: ep.ServiceName,
				Name:        ep.Name,
				Role:        ep.Role,
				Subordinate: context.isSubordinate(&ep),
			})
			// these should match on both sides so use the last
			relationInterface = ep.Interface
			scope = ep.Scope
		}
		relStatus := api.RelationStatus{
			Id:        relation.Id(),
			Key:       relation.String(),
			Interface: relationInterface,
			Scope:     scope,
			Endpoints: eps,
		}
		out = append(out, relStatus)
	}
	return out
}

// This method exists only to dedup the loaded relations as they will
// appear multiple times in context.relations.
func (context *statusContext) getAllRelations() []*state.Relation {
	var out []*state.Relation
	seenRelations := make(map[int]bool)
	for _, relations := range context.relations {
		for _, relation := range relations {
			if _, found := seenRelations[relation.Id()]; !found {
				out = append(out, relation)
				seenRelations[relation.Id()] = true
			}
		}
	}
	return out
}

func (context *statusContext) processNetworks() map[string]api.NetworkStatus {
	networksMap := make(map[string]api.NetworkStatus)
	for name, network := range context.networks {
		networksMap[name] = context.makeNetworkStatus(network)
	}
	return networksMap
}

func (context *statusContext) makeNetworkStatus(network *state.Network) api.NetworkStatus {
	return api.NetworkStatus{
		ProviderId: network.ProviderId(),
		CIDR:       network.CIDR(),
		VLANTag:    network.VLANTag(),
	}
}

func (context *statusContext) isSubordinate(ep *state.Endpoint) bool {
	service := context.services[ep.ServiceName]
	if service == nil {
		return false
	}
	return isSubordinate(ep, service)
}

func isSubordinate(ep *state.Endpoint, service *state.Service) bool {
	return ep.Scope == charm.ScopeContainer && !service.IsPrincipal()
}

// paramsJobsFromJobs converts state jobs to params jobs.
func paramsJobsFromJobs(jobs []state.MachineJob) []params.MachineJob {
	paramsJobs := make([]params.MachineJob, len(jobs))
	for i, machineJob := range jobs {
		paramsJobs[i] = machineJob.ToParams()
	}
	return paramsJobs
}

func (context *statusContext) processServices() map[string]api.ServiceStatus {
	servicesMap := make(map[string]api.ServiceStatus)
	for _, s := range context.services {
		servicesMap[s.Name()] = context.processService(s)
	}
	return servicesMap
}

func (context *statusContext) processService(service *state.Service) (status api.ServiceStatus) {
	serviceCharmURL, _ := service.CharmURL()
	status.Charm = serviceCharmURL.String()
	status.Exposed = service.IsExposed()
	status.Life = processLife(service)

	latestCharm, ok := context.latestCharms[*serviceCharmURL.WithRevision(-1)]
	if ok && latestCharm != serviceCharmURL.String() {
		status.CanUpgradeTo = latestCharm
	}
	var err error
	status.Relations, status.SubordinateTo, err = context.processServiceRelations(service)
	if err != nil {
		status.Err = err
		return
	}
	networks, err := service.Networks()
	if err != nil {
		status.Err = err
		return
	}
	var cons constraints.Value
	if service.IsPrincipal() {
		// Only principals can have constraints.
		cons, err = service.Constraints()
		if err != nil {
			status.Err = err
			return
		}
	}
	if len(networks) > 0 || cons.HaveNetworks() {
		// Only the explicitly requested networks (using "juju deploy
		// <svc> --networks=...") will be enabled, and altough when
		// specified, networks constraints will be used for instance
		// selection, they won't be actually enabled.
		status.Networks = api.NetworksSpecification{
			Enabled:  networks,
			Disabled: append(cons.IncludeNetworks(), cons.ExcludeNetworks()...),
		}
	}
	if service.IsPrincipal() {
		status.Units = context.processUnits(context.units[service.Name()], serviceCharmURL.String())
	}
	return status
}

func (context *statusContext) processUnits(units map[string]*state.Unit, serviceCharm string) map[string]api.UnitStatus {
	unitsMap := make(map[string]api.UnitStatus)
	for _, unit := range units {
		unitsMap[unit.Name()] = context.processUnit(unit, serviceCharm)
	}
	return unitsMap
}

func (context *statusContext) processUnit(unit *state.Unit, serviceCharm string) (status api.UnitStatus) {
	status.PublicAddress, _ = unit.PublicAddress()
	unitPorts, _ := unit.OpenedPorts()
	for _, port := range unitPorts {
		status.OpenedPorts = append(status.OpenedPorts, port.String())
	}
	if unit.IsPrincipal() {
		status.Machine, _ = unit.AssignedMachineId()
	}
	curl, _ := unit.CharmURL()
	if serviceCharm != "" && curl != nil && curl.String() != serviceCharm {
		status.Charm = curl.String()
	}
	status.Agent, status.AgentState, status.AgentStateInfo = processAgent(unit)
	status.AgentVersion = status.Agent.Version
	status.Life = status.Agent.Life
	status.Err = status.Agent.Err
	if subUnits := unit.SubordinateNames(); len(subUnits) > 0 {
		status.Subordinates = make(map[string]api.UnitStatus)
		for _, name := range subUnits {
			subUnit := context.unitByName(name)
			// subUnit may be nil if subordinate was filtered out.
			if subUnit != nil {
				status.Subordinates[name] = context.processUnit(subUnit, serviceCharm)
			}
		}
	}
	return
}

func (context *statusContext) unitByName(name string) *state.Unit {
	serviceName := strings.Split(name, "/")[0]
	return context.units[serviceName][name]
}

func (context *statusContext) processServiceRelations(service *state.Service) (
	related map[string][]string, subord []string, err error) {
	var subordSet set.Strings
	related = make(map[string][]string)
	relations := context.relations[service.Name()]
	for _, relation := range relations {
		ep, err := relation.Endpoint(service.Name())
		if err != nil {
			return nil, nil, err
		}
		relationName := ep.Relation.Name
		eps, err := relation.RelatedEndpoints(service.Name())
		if err != nil {
			return nil, nil, err
		}
		for _, ep := range eps {
			if isSubordinate(&ep, service) {
				subordSet.Add(ep.ServiceName)
			}
			related[relationName] = append(related[relationName], ep.ServiceName)
		}
	}
	for relationName, serviceNames := range related {
		sn := set.NewStrings(serviceNames...)
		related[relationName] = sn.SortedValues()
	}
	return related, subordSet.SortedValues(), nil
}

type lifer interface {
	Life() state.Life
}

type stateAgent interface {
	lifer
	AgentPresence() (bool, error)
	AgentTools() (*tools.Tools, error)
	Status() (state.Status, string, map[string]interface{}, error)
}

// processAgent retrieves version and status information from the given entity.
func processAgent(entity stateAgent) (
	out api.AgentStatus, compatStatus params.Status, compatInfo string) {

	out.Life = processLife(entity)

	if t, err := entity.AgentTools(); err == nil {
		out.Version = t.Version.Number.String()
	}

	var st state.Status
	st, out.Info, out.Data, out.Err = entity.Status()
	out.Status = params.Status(st)
	compatStatus = out.Status
	compatInfo = out.Info
	out.Data = filterStatusData(out.Data)
	if out.Err != nil {
		return
	}

	if out.Status == params.StatusPending {
		// The status is pending - there's no point
		// in enquiring about the agent liveness.
		return
	}
	agentAlive, err := entity.AgentPresence()
	if err != nil {
		return
	}

	if entity.Life() != state.Dead && !agentAlive {
		// The agent *should* be alive but is not. Set status to
		// StatusDown and munge Info to indicate the previous status and
		// info. This is unfortunately making presentation decisions
		// on behalf of the client (crappy).
		//
		// This is munging is only being left in place for
		// compatibility with older clients.  TODO: At some point we
		// should change this so that Info left alone. API version may
		// help here.
		//
		// Better yet, Status shouldn't be changed here in the API at
		// all! Status changes should only happen in State. One
		// problem caused by this is that this status change won't be
		// seen by clients using a watcher because it didn't happen in
		// State.
		if out.Info != "" {
			compatInfo = fmt.Sprintf("(%s: %s)", out.Status, out.Info)
		} else {
			compatInfo = fmt.Sprintf("(%s)", out.Status)
		}
		compatStatus = params.StatusDown
	}

	return
}

// filterStatusData limits what agent StatusData data is passed over
// the API. This prevents unintended leakage of internal-only data.
func filterStatusData(status map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	for name, value := range status {
		// use a set here if we end up with a larger whitelist
		if name == "relation-id" {
			out[name] = value
		}
	}
	return out
}

func processLife(entity lifer) string {
	if life := entity.Life(); life != state.Alive {
		// alive is the usual state so omit it by default.
		return life.String()
	}
	return ""
}
