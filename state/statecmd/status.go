// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statecmd

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils/set"
)

func Status(conn *juju.Conn, patterns []string) (*api.Status, error) {
	var nilStatus api.Status
	var context statusContext
	unitMatcher, err := NewUnitMatcher(patterns)
	if err != nil {
		return &nilStatus, err
	}
	if context.services,
		context.units, context.latestCharms, err = fetchAllServicesAndUnits(conn.State, unitMatcher); err != nil {
		return &nilStatus, err
	}

	// Filter machines by units in scope.
	var machineIds *set.Strings
	if !unitMatcher.matchesAny() {
		machineIds, err = fetchUnitMachineIds(context.units)
		if err != nil {
			return &nilStatus, err
		}
	}
	if context.machines, err = fetchMachines(conn.State, machineIds); err != nil {
		return &nilStatus, err
	}

	return &api.Status{
		EnvironmentName: conn.Environ.Name(),
		Machines:        context.processMachines(),
		Services:        context.processServices(),
	}, nil
}

type statusContext struct {
	machines     map[string][]*state.Machine
	services     map[string]*state.Service
	units        map[string]map[string]*state.Unit
	latestCharms map[charm.URL]string
}

type unitMatcher struct {
	patterns []string
}

// matchesAny returns true if the unitMatcher will
// match any unit, regardless of its attributes.
func (m unitMatcher) matchesAny() bool {
	return len(m.patterns) == 0
}

// matchUnit attempts to match a state.Unit to one of
// a set of patterns, taking into account subordinate
// relationships.
func (m unitMatcher) matchUnit(u *state.Unit) bool {
	if m.matchesAny() {
		return true
	}

	// Keep the unit if:
	//  (a) its name matches a pattern, or
	//  (b) it's a principal and one of its subordinates matches, or
	//  (c) it's a subordinate and its principal matches.
	//
	// Note: do *not* include a second subordinate if the principal is
	// only matched on account of a first subordinate matching.
	if m.matchString(u.Name()) {
		return true
	}
	if u.IsPrincipal() {
		for _, s := range u.SubordinateNames() {
			if m.matchString(s) {
				return true
			}
		}
		return false
	}
	principal, valid := u.PrincipalName()
	if !valid {
		panic("PrincipalName failed for subordinate unit")
	}
	return m.matchString(principal)
}

// matchString matches a string to one of the patterns in
// the unit matcher, returning an error if a pattern with
// invalid syntax is encountered.
func (m unitMatcher) matchString(s string) bool {
	for _, pattern := range m.patterns {
		ok, err := path.Match(pattern, s)
		if err != nil {
			// We validate patterns, so should never get here.
			panic(fmt.Errorf("pattern syntax error in %q", pattern))
		} else if ok {
			return true
		}
	}
	return false
}

// validPattern must match the parts of a unit or service name
// pattern either side of the '/' for it to be valid.
var validPattern = regexp.MustCompile("^[a-z0-9-*]+$")

// NewUnitMatcher returns a unitMatcher that matches units
// with one of the specified patterns, or all units if no
// patterns are specified.
//
// An error will be returned if any of the specified patterns
// is invalid. Patterns are valid if they contain only
// alpha-numeric characters, hyphens, or asterisks (and one
// optional '/' to separate service/unit).
func NewUnitMatcher(patterns []string) (unitMatcher, error) {
	for i, pattern := range patterns {
		fields := strings.Split(pattern, "/")
		if len(fields) > 2 {
			return unitMatcher{}, fmt.Errorf("pattern %q contains too many '/' characters", pattern)
		}
		for _, f := range fields {
			if !validPattern.MatchString(f) {
				return unitMatcher{}, fmt.Errorf("pattern %q contains invalid characters", pattern)
			}
		}
		if len(fields) == 1 {
			patterns[i] += "/*"
		}
	}
	return unitMatcher{patterns}, nil
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
	st *state.State, unitMatcher unitMatcher) (
	map[string]*state.Service, map[string]map[string]*state.Unit, map[charm.URL]string, error) {

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
			if !unitMatcher.matchUnit(u) {
				continue
			}
			svcUnitMap[u.Name()] = u
		}
		if unitMatcher.matchesAny() || len(svcUnitMap) > 0 {
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
		if errors.IsNotFoundError(err) {
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

func (context *statusContext) processMachines() map[string]api.MachineStatus {
	machinesMap := make(map[string]api.MachineStatus)
	for id, machines := range context.machines {
		hostStatus := context.makeMachineStatus(machines[0])
		context.processMachine(machines, &hostStatus, 0)
		machinesMap[id] = hostStatus
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
	status.Life,
		status.AgentVersion,
		status.AgentState,
		status.AgentStateInfo,
		status.Err = processAgent(machine)
	status.Series = machine.Series()
	instid, err := machine.InstanceId()
	if err == nil {
		status.InstanceId = instid
		status.InstanceState, err = machine.InstanceStatus()
		if err != nil {
			status.InstanceState = "error"
		}
		status.DNSName = instance.SelectPublicAddress(machine.Addresses())
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
		if !errors.IsNotFoundError(err) {
			status.Hardware = "error"
		}
	} else {
		status.Hardware = hc.String()
	}
	status.Containers = make(map[string]api.MachineStatus)
	return
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
	status.Relations, status.SubordinateTo, err = context.processRelations(service)
	if err != nil {
		status.Err = err
		return
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
	for _, port := range unit.OpenedPorts() {
		status.OpenedPorts = append(status.OpenedPorts, port.String())
	}
	if unit.IsPrincipal() {
		status.Machine, _ = unit.AssignedMachineId()
	}
	curl, _ := unit.CharmURL()
	if serviceCharm != "" && curl != nil && curl.String() != serviceCharm {
		status.Charm = curl.String()
	}
	status.Life,
		status.AgentVersion,
		status.AgentState,
		status.AgentStateInfo,
		status.Err = processAgent(unit)
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

func (*statusContext) processRelations(service *state.Service) (related map[string][]string, subord []string, err error) {
	// TODO(mue) This way the same relation is read twice (for each service).
	// Maybe add Relations() to state, read them only once and pass them to each
	// call of this function.
	relations, err := service.Relations()
	if err != nil {
		return nil, nil, err
	}
	var subordSet set.Strings
	related = make(map[string][]string)
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
			if ep.Scope == charm.ScopeContainer && !service.IsPrincipal() {
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
	AgentAlive() (bool, error)
	AgentTools() (*tools.Tools, error)
	Status() (params.Status, string, params.StatusData, error)
}

// processAgent retrieves version and status information from the given entity
// and sets the destination version, status and info values accordingly.
func processAgent(entity stateAgent) (life string, version string, status params.Status, info string, err error) {
	life = processLife(entity)
	if t, err := entity.AgentTools(); err == nil {
		version = t.Version.Number.String()
	}
	// TODO(mue) StatusData may be useful here too.
	status, info, _, err = entity.Status()
	if err != nil {
		return
	}
	if status == params.StatusPending {
		// The status is pending - there's no point
		// in enquiring about the agent liveness.
		return
	}
	agentAlive, err := entity.AgentAlive()
	if err != nil {
		return
	}
	if entity.Life() != state.Dead && !agentAlive {
		// The agent *should* be alive but is not.
		// Add the original status to the info, so it's not lost.
		if info != "" {
			info = fmt.Sprintf("(%s: %s)", status, info)
		} else {
			info = fmt.Sprintf("(%s)", status)
		}
		status = params.StatusDown
	}
	return
}

func processLife(entity lifer) string {
	if life := entity.Life(); life != state.Alive {
		// alive is the usual state so omit it by default.
		return life.String()
	}
	return ""
}
