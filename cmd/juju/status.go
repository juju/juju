// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils/set"
)

type StatusCommand struct {
	cmd.EnvCommandBase
	out      cmd.Output
	patterns []string
}

var statusDoc = `
This command will report on the runtime state of various system entities.

Service or unit names may be specified to filter the status to only those
services and units that match, along with the related machines, services
and units. If a subordinate unit is matched, then its principal unit will
be displayed. If a principal unit is matched, then all of its subordinates
will be displayed.

Wildcards ('*') may be specified in service/unit names to match any sequence
of characters. For example, 'nova-*' will match any service whose name begins
with 'nova-': 'nova-compute', 'nova-volume', etc.`[1:]

func (c *StatusCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "status",
		Args:    "[pattern ...]",
		Purpose: "output status information about an environment",
		Doc:     statusDoc,
		Aliases: []string{"stat"},
	}
}

func (c *StatusCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

func (c *StatusCommand) Init(args []string) error {
	c.patterns = args
	return nil
}

type statusContext struct {
	instances map[instance.Id]instance.Instance
	machines  map[string][]*state.Machine
	services  map[string]*state.Service
	units     map[string]map[string]*state.Unit
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

// newUnitMatcher returns a unitMatcher that matches units
// with one of the specified patterns, or all units if no
// patterns are specified.
//
// An error will be returned if any of the specified patterns
// is invalid. Patterns are valid if they contain only
// alpha-numeric characters, hyphens, or asterisks (and one
// optional '/' to separate service/unit).
func newUnitMatcher(patterns []string) (unitMatcher, error) {
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

var connectionError = `Unable to connect to environment "%s".
Please check your credentials or use 'juju bootstrap' to create a new environment.

Error details:
%v
`

func (c *StatusCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return fmt.Errorf(connectionError, c.EnvName, err)
	}
	defer conn.Close()

	var context statusContext
	unitMatcher, err := newUnitMatcher(c.patterns)
	if err != nil {
		return err
	}
	if context.services, context.units, err = fetchAllServicesAndUnits(conn.State, unitMatcher); err != nil {
		return err
	}

	// Filter machines by units in scope.
	var machineIds *set.Strings
	if !unitMatcher.matchesAny() {
		machineIds, err = fetchUnitMachineIds(context.units)
		if err != nil {
			return err
		}
	}
	if context.machines, err = fetchMachines(conn.State, machineIds); err != nil {
		return err
	}

	context.instances, err = fetchAllInstances(conn.Environ)
	if err != nil {
		// We cannot see instances from the environment, but
		// there's still lots of potentially useful info to print.
		fmt.Fprintf(ctx.Stderr, "cannot retrieve instances from the environment: %v\n", err)
	}
	result := struct {
		Environment string                   `json:"environment"`
		Machines    map[string]machineStatus `json:"machines"`
		Services    map[string]serviceStatus `json:"services"`
	}{
		Environment: conn.Environ.Name(),
		Machines:    context.processMachines(),
		Services:    context.processServices(),
	}
	return c.out.Write(ctx, result)
}

// fetchAllInstances returns a map from instance id to instance.
func fetchAllInstances(env environs.Environ) (map[instance.Id]instance.Instance, error) {
	m := make(map[instance.Id]instance.Instance)
	insts, err := env.AllInstances()
	if err != nil {
		return nil, err
	}
	for _, i := range insts {
		m[i.Id()] = i
	}
	return m, nil
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

// fetchAllServicesAndUnits returns a map from service name to service
// and a map from service name to unit name to unit.
func fetchAllServicesAndUnits(st *state.State, unitMatcher unitMatcher) (map[string]*state.Service, map[string]map[string]*state.Unit, error) {
	svcMap := make(map[string]*state.Service)
	unitMap := make(map[string]map[string]*state.Unit)
	services, err := st.AllServices()
	if err != nil {
		return nil, nil, err
	}
	for _, s := range services {
		units, err := s.AllUnits()
		if err != nil {
			return nil, nil, err
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
		}
	}
	return svcMap, unitMap, nil
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

func (context *statusContext) processMachines() map[string]machineStatus {
	machinesMap := make(map[string]machineStatus)
	for id, machines := range context.machines {
		hostStatus := context.makeMachineStatus(machines[0])
		context.processMachine(machines, &hostStatus, 0)
		machinesMap[id] = hostStatus
	}
	return machinesMap
}

func (context *statusContext) processMachine(machines []*state.Machine, host *machineStatus, startIndex int) (nextIndex int) {
	nextIndex = startIndex + 1
	currentHost := host
	var previousContainer *machineStatus
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

func (context *statusContext) makeMachineStatus(machine *state.Machine) (status machineStatus) {
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
		inst, ok := context.instances[instid]
		if ok {
			status.DNSName, _ = inst.DNSName()
		} else {
			// Double plus ungood.  There is an instance id recorded
			// for this machine in the state, yet the environ cannot
			// find that id.
			status.InstanceState = "missing"
		}
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
	status.Containers = make(map[string]machineStatus)
	return
}

func (context *statusContext) processServices() map[string]serviceStatus {
	servicesMap := make(map[string]serviceStatus)
	for _, s := range context.services {
		servicesMap[s.Name()] = context.processService(s)
	}
	return servicesMap
}

func (context *statusContext) processService(service *state.Service) (status serviceStatus) {
	url, _ := service.CharmURL()
	status.Charm = url.String()
	status.Exposed = service.IsExposed()
	status.Life = processLife(service)
	var err error
	status.Relations, status.SubordinateTo, err = context.processRelations(service)
	if err != nil {
		status.Err = err
		return
	}
	if service.IsPrincipal() {
		status.Units = context.processUnits(context.units[service.Name()])
	}
	return status
}

func (context *statusContext) processUnits(units map[string]*state.Unit) map[string]unitStatus {
	unitsMap := make(map[string]unitStatus)
	for _, unit := range units {
		unitsMap[unit.Name()] = context.processUnit(unit)
	}
	return unitsMap
}

func (context *statusContext) processUnit(unit *state.Unit) (status unitStatus) {
	status.PublicAddress, _ = unit.PublicAddress()
	for _, port := range unit.OpenedPorts() {
		status.OpenedPorts = append(status.OpenedPorts, port.String())
	}
	if unit.IsPrincipal() {
		status.Machine, _ = unit.AssignedMachineId()
	}
	status.Life,
		status.AgentVersion,
		status.AgentState,
		status.AgentStateInfo,
		status.Err = processAgent(unit)
	if subUnits := unit.SubordinateNames(); len(subUnits) > 0 {
		status.Subordinates = make(map[string]unitStatus)
		for _, name := range subUnits {
			subUnit := context.unitByName(name)
			// subUnit may be nil if subordinate was filtered out.
			if subUnit != nil {
				status.Subordinates[name] = context.processUnit(subUnit)
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
	Status() (params.Status, string, error)
}

// processAgent retrieves version and status information from the given entity
// and sets the destination version, status and info values accordingly.
func processAgent(entity stateAgent) (life string, version string, status params.Status, info string, err error) {
	life = processLife(entity)
	if t, err := entity.AgentTools(); err == nil {
		version = t.Version.Number.String()
	}
	status, info, err = entity.Status()
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

type machineStatus struct {
	Err            error                    `json:"-" yaml:",omitempty"`
	AgentState     params.Status            `json:"agent-state,omitempty" yaml:"agent-state,omitempty"`
	AgentStateInfo string                   `json:"agent-state-info,omitempty" yaml:"agent-state-info,omitempty"`
	AgentVersion   string                   `json:"agent-version,omitempty" yaml:"agent-version,omitempty"`
	DNSName        string                   `json:"dns-name,omitempty" yaml:"dns-name,omitempty"`
	InstanceId     instance.Id              `json:"instance-id,omitempty" yaml:"instance-id,omitempty"`
	InstanceState  string                   `json:"instance-state,omitempty" yaml:"instance-state,omitempty"`
	Life           string                   `json:"life,omitempty" yaml:"life,omitempty"`
	Series         string                   `json:"series,omitempty" yaml:"series,omitempty"`
	Id             string                   `json:"-" yaml:"-"`
	Containers     map[string]machineStatus `json:"containers,omitempty" yaml:"containers,omitempty"`
	Hardware       string                   `json:"hardware,omitempty" yaml:"hardware,omitempty"`
}

// A goyaml bug means we can't declare these types
// locally to the GetYAML methods.
type machineStatusNoMarshal machineStatus

type errorStatus struct {
	StatusError string `json:"status-error" yaml:"status-error"`
}

func (s machineStatus) MarshalJSON() ([]byte, error) {
	if s.Err != nil {
		return json.Marshal(errorStatus{s.Err.Error()})
	}
	return json.Marshal(machineStatusNoMarshal(s))
}

func (s machineStatus) GetYAML() (tag string, value interface{}) {
	if s.Err != nil {
		return "", errorStatus{s.Err.Error()}
	}
	// TODO(rog) rename mNoMethods to noMethods (and also in
	// the other GetYAML methods) when people are using the non-buggy
	// goyaml version.
	type mNoMethods machineStatus
	return "", mNoMethods(s)
}

type serviceStatus struct {
	Err           error                 `json:"-" yaml:",omitempty"`
	Charm         string                `json:"charm" yaml:"charm"`
	Exposed       bool                  `json:"exposed" yaml:"exposed"`
	Life          string                `json:"life,omitempty" yaml:"life,omitempty"`
	Relations     map[string][]string   `json:"relations,omitempty" yaml:"relations,omitempty"`
	SubordinateTo []string              `json:"subordinate-to,omitempty" yaml:"subordinate-to,omitempty"`
	Units         map[string]unitStatus `json:"units,omitempty" yaml:"units,omitempty"`
}
type serviceStatusNoMarshal serviceStatus

func (s serviceStatus) MarshalJSON() ([]byte, error) {
	if s.Err != nil {
		return json.Marshal(errorStatus{s.Err.Error()})
	}
	type sNoMethods serviceStatus
	return json.Marshal(sNoMethods(s))
}

func (s serviceStatus) GetYAML() (tag string, value interface{}) {
	if s.Err != nil {
		return "", errorStatus{s.Err.Error()}
	}
	type sNoMethods serviceStatus
	return "", sNoMethods(s)
}

type unitStatus struct {
	Err            error                 `json:"-" yaml:",omitempty"`
	AgentState     params.Status         `json:"agent-state,omitempty" yaml:"agent-state,omitempty"`
	AgentStateInfo string                `json:"agent-state-info,omitempty" yaml:"agent-state-info,omitempty"`
	AgentVersion   string                `json:"agent-version,omitempty" yaml:"agent-version,omitempty"`
	Life           string                `json:"life,omitempty" yaml:"life,omitempty"`
	Machine        string                `json:"machine,omitempty" yaml:"machine,omitempty"`
	OpenedPorts    []string              `json:"open-ports,omitempty" yaml:"open-ports,omitempty"`
	PublicAddress  string                `json:"public-address,omitempty" yaml:"public-address,omitempty"`
	Subordinates   map[string]unitStatus `json:"subordinates,omitempty" yaml:"subordinates,omitempty"`
}

type unitStatusNoMarshal unitStatus

func (s unitStatus) MarshalJSON() ([]byte, error) {
	if s.Err != nil {
		return json.Marshal(errorStatus{s.Err.Error()})
	}
	return json.Marshal(unitStatusNoMarshal(s))
}

func (s unitStatus) GetYAML() (tag string, value interface{}) {
	if s.Err != nil {
		return "", errorStatus{s.Err.Error()}
	}
	type uNoMethods unitStatus
	return "", unitStatusNoMarshal(s)
}
