// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/status"
	"github.com/juju/juju/worker/uniter/operation"
)

func agentStatusFromStatusInfo(s []status.StatusInfo, kind params.HistoryKind) []params.DetailedStatus {
	result := []params.DetailedStatus{}
	for _, v := range s {
		result = append(result, params.DetailedStatus{
			Status: v.Status,
			Info:   v.Message,
			Data:   v.Data,
			Since:  v.Since,
			Kind:   kind,
		})
	}
	return result

}

type sortableStatuses []params.DetailedStatus

func (s sortableStatuses) Len() int {
	return len(s)
}
func (s sortableStatuses) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s sortableStatuses) Less(i, j int) bool {
	return s[i].Since.Before(*s[j].Since)
}

// unitStatusHistory returns a list of status history entries for unit agents or workloads.
func (c *Client) unitStatusHistory(unitName string, size int, kind params.HistoryKind) ([]params.DetailedStatus, error) {
	unit, err := c.api.stateAccessor.Unit(unitName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	statuses := []params.DetailedStatus{}
	if kind == params.KindUnit || kind == params.KindWorkload {
		unitStatuses, err := unit.StatusHistory(size)
		if err != nil {
			return nil, errors.Trace(err)
		}
		statuses = agentStatusFromStatusInfo(unitStatuses, params.KindWorkload)

	}
	if kind == params.KindUnit || kind == params.KindUnitAgent {
		agentStatuses, err := unit.AgentHistory().StatusHistory(size)
		if err != nil {
			return nil, errors.Trace(err)
		}
		statuses = append(statuses, agentStatusFromStatusInfo(agentStatuses, params.KindUnitAgent)...)
	}

	sort.Sort(sortableStatuses(statuses))
	if kind == params.KindUnit {
		if len(statuses) > size {
			statuses = statuses[len(statuses)-size:]
		}
	}

	return statuses, nil
}

// machineInstanceStatusHistory returns status history for the instance of a given machine.
func (c *Client) machineInstanceStatusHistory(machineName string, size int, kind params.HistoryKind) ([]params.DetailedStatus, error) {
	machine, err := c.api.stateAccessor.Machine(machineName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sInfo, err := machine.InstanceStatusHistory(size)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return agentStatusFromStatusInfo(sInfo, kind), nil
}

// machineStatusHistory returns status history for the given machine.
func (c *Client) machineStatusHistory(machineName string, size int, kind params.HistoryKind) ([]params.DetailedStatus, error) {
	machine, err := c.api.stateAccessor.Machine(machineName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sInfo, err := machine.StatusHistory(size)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return agentStatusFromStatusInfo(sInfo, kind), nil
}

// StatusHistory returns a slice of past statuses for several entities.
func (c *Client) StatusHistory(args params.StatusHistoryArgs) (params.StatusHistoryResults, error) {
	if args.Size < 1 {
		return params.StatusHistoryResults{}, errors.Errorf("invalid history size: %d", args.Size)
	}
	history := params.StatusHistoryResults{}
	statuses := []params.DetailedStatus{}
	var err error
	switch args.Kind {
	case params.KindUnit, params.KindWorkload, params.KindUnitAgent:
		statuses, err = c.unitStatusHistory(args.Name, args.Size, args.Kind)
		if err != nil {
			return params.StatusHistoryResults{}, errors.Annotatef(err, "fetching unit status history for %q", args.Name)
		}
	case params.KindMachineInstance:
		mIStatuses, err := c.machineInstanceStatusHistory(args.Name, args.Size, params.KindMachineInstance)
		if err != nil {
			return params.StatusHistoryResults{}, errors.Annotate(err, "fetching machine instance status history")
		}
		statuses = mIStatuses
	case params.KindMachine:
		mStatuses, err := c.machineStatusHistory(args.Name, args.Size, params.KindMachine)
		if err != nil {
			return params.StatusHistoryResults{}, errors.Annotate(err, "fetching juju agent status history for machine")
		}
		statuses = mStatuses
	case params.KindContainerInstance:
		cIStatuses, err := c.machineStatusHistory(args.Name, args.Size, params.KindContainerInstance)
		if err != nil {
			return params.StatusHistoryResults{}, errors.Annotate(err, "fetching container status history")
		}
		statuses = cIStatuses
	case params.KindContainer:
		cStatuses, err := c.machineStatusHistory(args.Name, args.Size, params.KindContainer)
		if err != nil {
			return params.StatusHistoryResults{}, errors.Annotate(err, "fetching juju agent status history for container")
		}
		statuses = cStatuses
	}
	history.Statuses = statuses
	sort.Sort(sortableStatuses(history.Statuses))
	return history, nil
}

// FullStatus gives the information needed for juju status over the api
func (c *Client) FullStatus(args params.StatusParams) (params.FullStatus, error) {
	cfg, err := c.api.stateAccessor.ModelConfig()
	if err != nil {
		return params.FullStatus{}, errors.Annotate(err, "could not get environ config")
	}
	var noStatus params.FullStatus
	var context statusContext
	if context.services, context.units, context.latestCharms, err =
		fetchAllServicesAndUnits(c.api.stateAccessor, len(args.Patterns) <= 0); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch services and units")
	} else if context.machines, err = fetchMachines(c.api.stateAccessor, nil); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch machines")
	} else if context.relations, err = fetchRelations(c.api.stateAccessor); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch relations")
	} else if context.networks, err = fetchNetworks(c.api.stateAccessor); err != nil {
		return noStatus, errors.Annotate(err, "could not fetch networks")
	}

	logger.Debugf("Services: %v", context.services)

	if len(args.Patterns) > 0 {
		predicate := BuildPredicateFor(args.Patterns)

		// First, attempt to match machines. Any units on those
		// machines are implicitly matched.
		matchedMachines := make(set.Strings)
		for _, machineList := range context.machines {
			for _, m := range machineList {
				matches, err := predicate(m)
				if err != nil {
					return noStatus, errors.Annotate(
						err, "could not filter machines",
					)
				}
				if matches {
					matchedMachines.Add(m.Id())
				}
			}
		}

		// Filter units
		matchedSvcs := make(set.Strings)
		unitChainPredicate := UnitChainPredicateFn(predicate, context.unitByName)
		for _, unitMap := range context.units {
			for name, unit := range unitMap {
				machineId, err := unit.AssignedMachineId()
				if err != nil {
					machineId = ""
				} else if matchedMachines.Contains(machineId) {
					// Unit is on a matching machine.
					matchedSvcs.Add(unit.ServiceName())
					continue
				}

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
				matchedSvcs.Add(unit.ServiceName())
				if machineId != "" {
					matchedMachines.Add(machineId)
				}
			}
		}

		// Filter services
		for svcName, svc := range context.services {
			if matchedSvcs.Contains(svcName) {
				// There are matched units for this service.
				continue
			} else if matches, err := predicate(svc); err != nil {
				return noStatus, errors.Annotate(err, "could not filter services")
			} else if !matches {
				delete(context.services, svcName)
			}
		}

		// Filter machines
		for status, machineList := range context.machines {
			matched := make([]*state.Machine, 0, len(machineList))
			for _, m := range machineList {
				machineContainers, err := m.Containers()
				if err != nil {
					return noStatus, err
				}
				machineContainersSet := set.NewStrings(machineContainers...)

				if matchedMachines.Contains(m.Id()) || !matchedMachines.Intersection(machineContainersSet).IsEmpty() {
					// The machine is matched directly, or contains a unit
					// or container that matches.
					logger.Tracef("machine %s is hosting something.", m.Id())
					matched = append(matched, m)
					continue
				}
			}
			context.machines[status] = matched
		}
	}

	newToolsVersion, err := c.newToolsVersionAvailable()
	if err != nil {
		return noStatus, errors.Annotate(err, "cannot determine if there is a new tools version available")
	}
	if err != nil {
		return noStatus, errors.Annotate(err, "cannot determine mongo information")
	}
	return params.FullStatus{
		ModelName:        cfg.Name(),
		AvailableVersion: newToolsVersion,
		Machines:         processMachines(context.machines),
		Services:         context.processServices(),
		Networks:         context.processNetworks(),
		Relations:        context.processRelations(),
	}, nil
}

// newToolsVersionAvailable will return a string representing a tools
// version only if the latest check is newer than current tools.
func (c *Client) newToolsVersionAvailable() (string, error) {
	env, err := c.api.stateAccessor.Model()
	if err != nil {
		return "", errors.Annotate(err, "cannot get model")
	}

	latestVersion := env.LatestToolsVersion()

	envConfig, err := c.api.stateAccessor.ModelConfig()
	if err != nil {
		return "", errors.Annotate(err, "cannot obtain current environ config")
	}
	oldV, ok := envConfig.AgentVersion()
	if !ok {
		return "", nil
	}
	if oldV.Compare(latestVersion) < 0 {
		return latestVersion.String(), nil
	}
	return "", nil
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
	latestCharms map[charm.URL]*state.Charm
}

// fetchMachines returns a map from top level machine id to machines, where machines[0] is the host
// machine and machines[1..n] are any containers (including nested ones).
//
// If machineIds is non-nil, only machines whose IDs are in the set are returned.
func fetchMachines(st stateInterface, machineIds set.Strings) (map[string][]*state.Machine, error) {
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
	st stateInterface,
	matchAny bool,
) (map[string]*state.Service, map[string]map[string]*state.Unit, map[charm.URL]*state.Charm, error) {

	svcMap := make(map[string]*state.Service)
	unitMap := make(map[string]map[string]*state.Unit)
	latestCharms := make(map[charm.URL]*state.Charm)
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
				latestCharms[*charmURL.WithRevision(-1)] = nil
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
		latestCharms[baseURL] = ch
	}

	return svcMap, unitMap, latestCharms, nil
}

// fetchRelations returns a map of all relations keyed by service name.
//
// This structure is useful for processServiceRelations() which needs
// to have the relations for each service. Reading them once here
// avoids the repeated DB hits to retrieve the relations for each
// service that used to happen in processServiceRelations().
func fetchRelations(st stateInterface) (map[string][]*state.Relation, error) {
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
func fetchNetworks(st stateInterface) (map[string]*state.Network, error) {
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
		tlMachine := machines[0]
		hostStatus := makeMachineStatus(tlMachine)
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
	var err error
	status.Id = machine.Id()
	agentStatus := processMachine(machine)
	status.AgentStatus = agentStatus

	status.Series = machine.Series()
	status.Jobs = paramsJobsFromJobs(machine.Jobs())
	status.WantsVote = machine.WantsVote()
	status.HasVote = machine.HasVote()
	sInfo, err := machine.InstanceStatus()
	populateStatusFromStatusInfoAndErr(&status.InstanceStatus, sInfo, err)
	instid, err := machine.InstanceId()
	if err == nil {
		status.InstanceId = instid
		addr, err := machine.PublicAddress()
		if err != nil {
			// Usually this indicates that no addresses have been set on the
			// machine yet.
			addr = network.Address{}
			logger.Debugf("error fetching public address: %q", err)
		}
		status.DNSName = addr.Value
	} else {
		if errors.IsNotProvisioned(err) {
			status.InstanceId = "pending"
		} else {
			status.InstanceId = "error"
		}
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

func (context *statusContext) processService(service *state.Service) (processedStatus params.ServiceStatus) {
	serviceCharmURL, _ := service.CharmURL()
	processedStatus.Charm = serviceCharmURL.String()
	processedStatus.Exposed = service.IsExposed()
	processedStatus.Life = processLife(service)

	if latestCharm, ok := context.latestCharms[*serviceCharmURL.WithRevision(-1)]; ok && latestCharm != nil {
		if latestCharm.Revision() > serviceCharmURL.Revision {
			processedStatus.CanUpgradeTo = latestCharm.String()
		}
	}

	var err error
	processedStatus.Relations, processedStatus.SubordinateTo, err = context.processServiceRelations(service)
	if err != nil {
		processedStatus.Err = err
		return
	}
	networks, err := service.Networks()
	if err != nil {
		processedStatus.Err = err
		return
	}
	var cons constraints.Value
	if service.IsPrincipal() {
		// Only principals can have constraints.
		cons, err = service.Constraints()
		if err != nil {
			processedStatus.Err = err
			return
		}
	}
	// TODO(dimitern): Drop support for this in a follow-up.
	if len(networks) > 0 || cons.HaveNetworks() {
		// Only the explicitly requested networks (using "juju deploy
		// <svc> --networks=...") will be enabled, and altough when
		// specified, networks constraints will be used for instance
		// selection, they won't be actually enabled.
		processedStatus.Networks = params.NetworksSpecification{
			Enabled:  networks,
			Disabled: append(cons.IncludeNetworks(), cons.ExcludeNetworks()...),
		}
	}
	if service.IsPrincipal() {
		processedStatus.Units = context.processUnits(context.units[service.Name()], serviceCharmURL.String())
		serviceStatus, err := service.Status()
		if err != nil {
			processedStatus.Err = err
			return
		}
		processedStatus.Status.Status = serviceStatus.Status
		processedStatus.Status.Info = serviceStatus.Message
		processedStatus.Status.Data = serviceStatus.Data
		processedStatus.Status.Since = serviceStatus.Since

		processedStatus.MeterStatuses = context.processUnitMeterStatuses(context.units[service.Name()])
	}
	return processedStatus
}

func isColorStatus(code state.MeterStatusCode) bool {
	return code == state.MeterGreen || code == state.MeterAmber || code == state.MeterRed
}

func (context *statusContext) processUnitMeterStatuses(units map[string]*state.Unit) map[string]params.MeterStatus {
	unitsMap := make(map[string]params.MeterStatus)
	for _, unit := range units {
		meterStatus, err := unit.GetMeterStatus()
		if err != nil {
			continue
		}
		if isColorStatus(meterStatus.Code) {
			unitsMap[unit.Name()] = params.MeterStatus{Color: strings.ToLower(meterStatus.Code.String()), Message: meterStatus.Info}
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
	addr, err := unit.PublicAddress()
	if err != nil {
		// Usually this indicates that no addresses have been set on the
		// machine yet.
		addr = network.Address{}
		logger.Debugf("error fetching public address: %v", err)
	}
	result.PublicAddress = addr.Value
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
func processUnitAndAgentStatus(unit *state.Unit, unitStatus *params.UnitStatus) {
	unitStatus.AgentStatus, unitStatus.WorkloadStatus = processUnitStatus(unit)
	processUnitLost(unit, unitStatus)
}

// populateStatusFromGetter creates status information for machines, units.
func populateStatusFromGetter(agent *params.DetailedStatus, getter status.StatusGetter) {
	statusInfo, err := getter.Status()
	populateStatusFromStatusInfoAndErr(agent, statusInfo, err)
}

// populateStatusFromStatusInfoAndErr creates AgentStatus from the typical output
// of a status getter.
func populateStatusFromStatusInfoAndErr(agent *params.DetailedStatus, statusInfo status.StatusInfo, err error) {
	agent.Err = err
	agent.Status = statusInfo.Status
	agent.Info = statusInfo.Message
	agent.Data = filterStatusData(statusInfo.Data)
	agent.Since = statusInfo.Since
}

// processMachine retrieves version and status information for the given machine.
// It also returns deprecated legacy status information.
func processMachine(machine *state.Machine) (out params.DetailedStatus) {
	out.Life = processLife(machine)

	if t, err := machine.AgentTools(); err == nil {
		out.Version = t.Version.Number.String()
	}

	populateStatusFromGetter(&out, machine)

	if out.Err != nil {
		return
	}
	if out.Status == status.StatusPending || out.Status == status.StatusAllocating {
		// The status is pending - there's no point
		// in enquiring about the agent liveness.
		return
	}

	return
}

// processUnit retrieves version and status information for the given unit.
func processUnitStatus(unit *state.Unit) (agentStatus, workloadStatus params.DetailedStatus) {
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

func canBeLost(unitStatus *params.UnitStatus) bool {
	switch unitStatus.AgentStatus.Status {
	case status.StatusAllocating:
		return false
	case status.StatusExecuting:
		return unitStatus.AgentStatus.Info != operation.RunningHookMessage(string(hooks.Install))
	}
	// TODO(fwereade/wallyworld): we should have an explicit place in the model
	// to tell us when we've hit this point, instead of piggybacking on top of
	// status and/or status history.
	isInstalled := unitStatus.WorkloadStatus.Status != status.StatusMaintenance || unitStatus.WorkloadStatus.Info != status.MessageInstalling
	return isInstalled
}

// processUnitLost determines whether the given unit should be marked as lost.
// TODO(fwereade/wallyworld): this is also model-level code and should sit in
// between state and this package.
func processUnitLost(unit *state.Unit, unitStatus *params.UnitStatus) {
	if !canBeLost(unitStatus) {
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
		if unitStatus.WorkloadStatus.Status != status.StatusError {
			unitStatus.WorkloadStatus.Status = status.StatusUnknown
			unitStatus.WorkloadStatus.Info = fmt.Sprintf("agent is lost, sorry! See 'juju status-history %s'", unit.Name())
		}
		unitStatus.AgentStatus.Status = status.StatusLost
		unitStatus.AgentStatus.Info = "agent is not communicating with the server"
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
