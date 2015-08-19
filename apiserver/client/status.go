// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/worker/uniter/operation"
)

func agentStatusFromStatusInfo(s []state.StatusInfo, kind params.HistoryKind) []params.AgentStatus {
	result := []params.AgentStatus{}
	for _, v := range s {
		result = append(result, params.AgentStatus{
			Status: params.Status(v.Status),
			Info:   v.Message,
			Data:   v.Data,
			Since:  v.Since,
			Kind:   kind,
		})
	}
	return result

}

type sortableStatuses []params.AgentStatus

func (s sortableStatuses) Len() int {
	return len(s)
}
func (s sortableStatuses) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s sortableStatuses) Less(i, j int) bool {
	return s[i].Since.Before(*s[j].Since)
}

// TODO(perrito666) this client method requires more testing, only its parts are unittested.
// UnitStatusHistory returns a slice of past statuses for a given unit.
func (c *Client) UnitStatusHistory(args params.StatusHistory) (params.UnitStatusHistory, error) {
	size := args.Size - 1
	if size < 1 {
		return params.UnitStatusHistory{}, errors.Errorf("invalid history size: %d", args.Size)
	}
	unit, err := c.api.state.Unit(args.Name)
	if err != nil {
		return params.UnitStatusHistory{}, errors.Trace(err)
	}
	statuses := params.UnitStatusHistory{}
	if args.Kind == params.KindCombined || args.Kind == params.KindWorkload {
		unitStatuses, err := unit.StatusHistory(size)
		if err != nil {
			return params.UnitStatusHistory{}, errors.Trace(err)
		}

		current, err := unit.Status()
		if err != nil {
			return params.UnitStatusHistory{}, errors.Trace(err)
		}
		unitStatuses = append(unitStatuses, current)

		statuses.Statuses = append(statuses.Statuses, agentStatusFromStatusInfo(unitStatuses, params.KindWorkload)...)
	}
	if args.Kind == params.KindCombined || args.Kind == params.KindAgent {
		agent := unit.Agent()
		agentStatuses, err := agent.StatusHistory(size)
		if err != nil {
			return params.UnitStatusHistory{}, errors.Trace(err)
		}

		current, err := agent.Status()
		if err != nil {
			return params.UnitStatusHistory{}, errors.Trace(err)
		}
		agentStatuses = append(agentStatuses, current)

		statuses.Statuses = append(statuses.Statuses, agentStatusFromStatusInfo(agentStatuses, params.KindAgent)...)
	}

	sort.Sort(sortableStatuses(statuses.Statuses))
	if args.Kind == params.KindCombined {

		if len(statuses.Statuses) > args.Size {
			statuses.Statuses = statuses.Statuses[len(statuses.Statuses)-args.Size:]
		}

	}
	return statuses, nil
}

// FullStatus gives the information needed for juju status over the api
func (c *Client) FullStatus(args params.StatusParams) (params.FullStatus, error) {
	cfg, err := c.api.state.EnvironConfig()
	if err != nil {
		return params.FullStatus{}, errors.Annotate(err, "could not get environ config")
	}
	var noStatus params.FullStatus
	var context statusContext
	if context.services, context.units, context.latestCharms, err =
		fetchAllServicesAndUnits(c.api.state, len(args.Patterns) <= 0); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch services and units")
	} else if context.machines, err = fetchMachines(c.api.state, nil); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch machines")
	} else if context.relations, err = fetchRelations(c.api.state); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch relations")
	} else if context.networks, err = fetchNetworks(c.api.state); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch networks")
	}

	logger.Debugf("Services: %v", context.services)

	if len(args.Patterns) > 0 {
		predicate := BuildPredicateFor(args.Patterns)

		// Filter units
		unfilteredSvcs := make(set.Strings)
		unfilteredMachines := make(set.Strings)
		unitChainPredicate := UnitChainPredicateFn(predicate, context.unitByName)
		for _, unitMap := range context.units {
			for name, unit := range unitMap {
				// Always start examining at the top-level. This
				// prevents a situation where we filter a subordinate
				// before we discover its parent is a match.
				if !unit.IsPrincipal() {
					continue
				} else if matches, err := unitChainPredicate(unit); err != nil {
					return noStatus, errors.Annotate(err, "could not filter units")
				} else if !matches {
					delete(unitMap, name)
					continue
				}

				// Track which services are utilized by the units so
				// that we can be sure to not filter that service out.
				unfilteredSvcs.Add(unit.ServiceName())
				machineId, err := unit.AssignedMachineId()
				if err != nil {
					return noStatus, err
				}
				unfilteredMachines.Add(machineId)
			}
		}

		// Filter services
		for svcName, svc := range context.services {
			if unfilteredSvcs.Contains(svcName) {
				// Don't filter services which have units that were
				// not filtered.
				continue
			} else if matches, err := predicate(svc); err != nil {
				return noStatus, errors.Annotate(err, "could not filter services")
			} else if !matches {
				delete(context.services, svcName)
			}
		}

		// Filter machines
		for status, machineList := range context.machines {
			filteredList := make([]*state.Machine, 0, len(machineList))
			for _, m := range machineList {
				machineContainers, err := m.Containers()
				if err != nil {
					return noStatus, err
				}
				machineContainersSet := set.NewStrings(machineContainers...)

				if unfilteredMachines.Contains(m.Id()) || !unfilteredMachines.Intersection(machineContainersSet).IsEmpty() {
					// Don't filter machines which have an unfiltered
					// unit running on them.
					logger.Debugf("mid %s is hosting something.", m.Id())
					filteredList = append(filteredList, m)
					continue
				} else if matches, err := predicate(m); err != nil {
					return noStatus, errors.Annotate(err, "could not filter machines")
				} else if matches {
					filteredList = append(filteredList, m)
				}
			}
			context.machines[status] = filteredList
		}
	}

	return params.FullStatus{
		EnvironmentName: cfg.Name(),
		Machines:        processMachines(context.machines),
		Services:        context.processServices(),
		Networks:        context.processNetworks(),
		Relations:       context.processRelations(),
	}, nil
}

// Status is a stub version of FullStatus that was introduced in 1.16
func (c *Client) Status() (params.LegacyStatus, error) {
	var legacyStatus params.LegacyStatus
	status, err := c.FullStatus(params.StatusParams{})
	if err != nil {
		return legacyStatus, err
	}

	legacyStatus.Machines = make(map[string]params.LegacyMachineStatus)
	for machineName, machineStatus := range status.Machines {
		legacyStatus.Machines[machineName] = params.LegacyMachineStatus{
			InstanceId: string(machineStatus.InstanceId),
		}
	}
	return legacyStatus, nil
}

type statusContext struct {
	// machines: top-level machine id -> list of machines nested in
	// this machine.
	machines map[string][]*state.Machine
	// services: service name -> service
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
func fetchMachines(st *state.State, machineIds set.Strings) (map[string][]*state.Machine, error) {
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
	matchAny bool,
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
		if matchAny || len(svcUnitMap) > 0 {
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
	for baseURL := range latestCharms {
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
func fetchUnitMachineIds(units map[string]map[string]*state.Unit) (set.Strings, error) {
	machineIds := make(set.Strings)
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

type machineAndContainers map[string][]*state.Machine

func (m machineAndContainers) HostForMachineId(id string) *state.Machine {
	// Element 0 is assumed to be the top-level machine.
	return m[id][0]
}

func (m machineAndContainers) Containers(id string) []*state.Machine {
	return m[id][1:]
}

func processMachines(idToMachines map[string][]*state.Machine) map[string]params.MachineStatus {
	machinesMap := make(map[string]params.MachineStatus)
	cache := make(map[string]params.MachineStatus)
	for id, machines := range idToMachines {

		if len(machines) <= 0 {
			continue
		}

		// Element 0 is assumed to be the top-level machine.
		hostStatus := makeMachineStatus(machines[0])
		machinesMap[id] = hostStatus
		cache[id] = hostStatus

		for _, machine := range machines[1:] {
			parent, ok := cache[state.ParentId(machine.Id())]
			if !ok {
				panic("We've broken an assumpution.")
			}

			status := makeMachineStatus(machine)
			parent.Containers[machine.Id()] = status
			cache[machine.Id()] = status
		}
	}
	return machinesMap
}

func makeMachineStatus(machine *state.Machine) (status params.MachineStatus) {
	status.Id = machine.Id()
	agentStatus, compatStatus := processMachine(machine)
	status.Agent = agentStatus

	// These legacy status values will be deprecated for Juju 2.0.
	status.AgentState = compatStatus.Status
	status.AgentStateInfo = compatStatus.Info
	status.AgentVersion = compatStatus.Version
	status.Life = compatStatus.Life
	status.Err = compatStatus.Err

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
		if errors.IsNotProvisioned(err) {
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
	status.Containers = make(map[string]params.MachineStatus)
	return
}

func (context *statusContext) processRelations() []params.RelationStatus {
	var out []params.RelationStatus
	relations := context.getAllRelations()
	for _, relation := range relations {
		var eps []params.EndpointStatus
		var scope charm.RelationScope
		var relationInterface string
		for _, ep := range relation.Endpoints() {
			eps = append(eps, params.EndpointStatus{
				ServiceName: ep.ServiceName,
				Name:        ep.Name,
				Role:        ep.Role,
				Subordinate: context.isSubordinate(&ep),
			})
			// these should match on both sides so use the last
			relationInterface = ep.Interface
			scope = ep.Scope
		}
		relStatus := params.RelationStatus{
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

func (context *statusContext) processNetworks() map[string]params.NetworkStatus {
	networksMap := make(map[string]params.NetworkStatus)
	for name, network := range context.networks {
		networksMap[name] = context.makeNetworkStatus(network)
	}
	return networksMap
}

func (context *statusContext) makeNetworkStatus(network *state.Network) params.NetworkStatus {
	return params.NetworkStatus{
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
func paramsJobsFromJobs(jobs []state.MachineJob) []multiwatcher.MachineJob {
	paramsJobs := make([]multiwatcher.MachineJob, len(jobs))
	for i, machineJob := range jobs {
		paramsJobs[i] = machineJob.ToParams()
	}
	return paramsJobs
}

func (context *statusContext) processServices() map[string]params.ServiceStatus {
	servicesMap := make(map[string]params.ServiceStatus)
	for _, s := range context.services {
		servicesMap[s.Name()] = context.processService(s)
	}
	return servicesMap
}

func (context *statusContext) processService(service *state.Service) (status params.ServiceStatus) {
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
	// TODO(dimitern): Drop support for this in a follow-up.
	if len(networks) > 0 || cons.HaveNetworks() {
		// Only the explicitly requested networks (using "juju deploy
		// <svc> --networks=...") will be enabled, and altough when
		// specified, networks constraints will be used for instance
		// selection, they won't be actually enabled.
		status.Networks = params.NetworksSpecification{
			Enabled:  networks,
			Disabled: append(cons.IncludeNetworks(), cons.ExcludeNetworks()...),
		}
	}
	if service.IsPrincipal() {
		status.Units = context.processUnits(context.units[service.Name()], serviceCharmURL.String())
		serviceStatus, err := service.Status()
		if err != nil {
			status.Err = err
			return
		}
		status.Status.Status = params.Status(serviceStatus.Status)
		status.Status.Info = serviceStatus.Message
		status.Status.Data = serviceStatus.Data
		status.Status.Since = serviceStatus.Since

		status.MeterStatuses = context.processUnitMeterStatuses(context.units[service.Name()])
	}
	return status
}

func isColorStatus(code state.MeterStatusCode) bool {
	return code == state.MeterGreen || code == state.MeterAmber || code == state.MeterRed
}

func (context *statusContext) processUnitMeterStatuses(units map[string]*state.Unit) map[string]params.MeterStatus {
	unitsMap := make(map[string]params.MeterStatus)
	for _, unit := range units {
		status, err := unit.GetMeterStatus()
		if err != nil {
			continue
		}
		if isColorStatus(status.Code) {
			unitsMap[unit.Name()] = params.MeterStatus{Color: strings.ToLower(status.Code.String()), Message: status.Info}
		}
	}
	if len(unitsMap) > 0 {
		return unitsMap
	}
	return nil
}

func (context *statusContext) processUnits(units map[string]*state.Unit, serviceCharm string) map[string]params.UnitStatus {
	unitsMap := make(map[string]params.UnitStatus)
	for _, unit := range units {
		unitsMap[unit.Name()] = context.processUnit(unit, serviceCharm)
	}
	return unitsMap
}

func (context *statusContext) processUnit(unit *state.Unit, serviceCharm string) params.UnitStatus {
	var result params.UnitStatus
	result.PublicAddress, _ = unit.PublicAddress()
	unitPorts, _ := unit.OpenedPorts()
	for _, port := range unitPorts {
		result.OpenedPorts = append(result.OpenedPorts, port.String())
	}
	if unit.IsPrincipal() {
		result.Machine, _ = unit.AssignedMachineId()
	}
	curl, _ := unit.CharmURL()
	if serviceCharm != "" && curl != nil && curl.String() != serviceCharm {
		result.Charm = curl.String()
	}
	processUnitAndAgentStatus(unit, &result)

	if subUnits := unit.SubordinateNames(); len(subUnits) > 0 {
		result.Subordinates = make(map[string]params.UnitStatus)
		for _, name := range subUnits {
			subUnit := context.unitByName(name)
			// subUnit may be nil if subordinate was filtered out.
			if subUnit != nil {
				result.Subordinates[name] = context.processUnit(subUnit, serviceCharm)
			}
		}
	}
	return result
}

func (context *statusContext) unitByName(name string) *state.Unit {
	serviceName := strings.Split(name, "/")[0]
	return context.units[serviceName][name]
}

func (context *statusContext) processServiceRelations(service *state.Service) (related map[string][]string, subord []string, err error) {
	subordSet := make(set.Strings)
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

// processUnitAndAgentStatus retrieves status information for both unit and unitAgents.
func processUnitAndAgentStatus(unit *state.Unit, status *params.UnitStatus) {
	status.UnitAgent, status.Workload = processUnitStatus(unit)

	// Legacy fields required until Juju 2.0.
	// We only display pending, started, error, stopped.
	var ok bool
	legacyState, ok := state.TranslateToLegacyAgentState(
		state.Status(status.UnitAgent.Status),
		state.Status(status.Workload.Status),
		status.Workload.Info,
	)
	if !ok {
		logger.Warningf(
			"translate to legacy status encounted unexpected workload status %q and agent status %q",
			status.Workload.Status, status.UnitAgent.Status)
	}
	status.AgentState = params.Status(legacyState)
	if status.AgentState == params.StatusError {
		status.AgentStateInfo = status.Workload.Info
	}
	status.AgentVersion = status.UnitAgent.Version
	status.Life = status.UnitAgent.Life
	status.Err = status.UnitAgent.Err

	processUnitLost(unit, status)

	return
}

// populateStatusFromGetter creates status information for machines, units.
func populateStatusFromGetter(agent *params.AgentStatus, getter state.StatusGetter) {
	statusInfo, err := getter.Status()
	agent.Err = err
	agent.Status = params.Status(statusInfo.Status)
	agent.Info = statusInfo.Message
	agent.Data = filterStatusData(statusInfo.Data)
	agent.Since = statusInfo.Since
}

// processMachine retrieves version and status information for the given machine.
// It also returns deprecated legacy status information.
func processMachine(machine *state.Machine) (out params.AgentStatus, compat params.AgentStatus) {
	out.Life = processLife(machine)

	if t, err := machine.AgentTools(); err == nil {
		out.Version = t.Version.Number.String()
	}

	populateStatusFromGetter(&out, machine)
	compat = out

	if out.Err != nil {
		return
	}
	if out.Status == params.StatusPending {
		// The status is pending - there's no point
		// in enquiring about the agent liveness.
		return
	}
	agentAlive, err := machine.AgentPresence()
	if err != nil {
		return
	}

	if machine.Life() != state.Dead && !agentAlive {
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
			compat.Info = fmt.Sprintf("(%s: %s)", out.Status, out.Info)
		} else {
			compat.Info = fmt.Sprintf("(%s)", out.Status)
		}
		compat.Status = params.StatusDown
	}

	return
}

// processUnit retrieves version and status information for the given unit.
func processUnitStatus(unit *state.Unit) (agentStatus, workloadStatus params.AgentStatus) {
	// First determine the agent status information.
	unitAgent := unit.Agent()
	populateStatusFromGetter(&agentStatus, unitAgent)
	agentStatus.Life = processLife(unit)
	if t, err := unit.AgentTools(); err == nil {
		agentStatus.Version = t.Version.Number.String()
	}

	// Second, determine the workload (unit) status.
	populateStatusFromGetter(&workloadStatus, unit)
	return
}

func canBeLost(status *params.UnitStatus) bool {
	// Pending and Installing are deprecated.
	// Need to still check pending for existing deployments.
	switch status.UnitAgent.Status {
	case params.StatusPending, params.StatusInstalling, params.StatusAllocating:
		return false
	case params.StatusExecuting:
		return status.UnitAgent.Info != operation.RunningHookMessage(string(hooks.Install))
	}
	// TODO(fwereade/wallyworld): we should have an explicit place in the model
	// to tell us when we've hit this point, instead of piggybacking on top of
	// status and/or status history.
	isInstalled := status.Workload.Status != params.StatusMaintenance || status.Workload.Info != state.MessageInstalling
	return isInstalled
}

// processUnitLost determines whether the given unit should be marked as lost.
// TODO(fwereade/wallyworld): this is also model-level code and should sit in
// between state and this package.
func processUnitLost(unit *state.Unit, status *params.UnitStatus) {
	if !canBeLost(status) {
		// The status is allocating or installing - there's no point
		// in enquiring about the agent liveness.
		return
	}
	agentAlive, err := unit.AgentPresence()
	if err != nil {
		return
	}

	if unit.Life() != state.Dead && !agentAlive {
		// If the unit is in error, it would be bad to throw away
		// the error information as when the agent reconnects, that
		// error information would then be lost.
		if status.Workload.Status != params.StatusError {
			status.Workload.Status = params.StatusUnknown
			status.Workload.Info = fmt.Sprintf("agent is lost, sorry! See 'juju status-history %s'", unit.Name())
		}
		status.UnitAgent.Status = params.StatusLost
		status.UnitAgent.Info = "agent is not communicating with the server"
	}
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
