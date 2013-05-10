package main

import (
	"encoding/json"
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/set"
	"strings"
)

type StatusCommand struct {
	EnvCommandBase
	out cmd.Output
}

var statusDoc = "This command will report on the runtime state of various system entities."

func (c *StatusCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "status",
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

type statusContext struct {
	instances map[state.InstanceId]environs.Instance
	machines  map[string]*state.Machine
	services  map[string]*state.Service
	units     map[string]map[string]*state.Unit
	statuses  map[string]params.Status
	infos     map[string]string
}

func (c *StatusCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()

	var ctxt statusContext
	if ctxt.machines, err = fetchAllMachines(conn.State); err != nil {
		return err
	}
	if ctxt.services, ctxt.units, err = fetchAllServicesAndUnits(conn.State); err != nil {
		return err
	}
	if ctx.statuses, ctx.infos, err = fetchAllStatuses(conn.State); err != nil {
		return err
	}
	ctxt.instances, err = fetchAllInstances(conn.Environ)
	if err != nil {
		// We cannot see instances from the environment, but
		// there's still lots of potentially useful info to print.
		fmt.Fprintf(ctx.Stderr, "cannot retrieve instances from the environment: %v\n", err)
	}
	result := struct {
		Machines map[string]machineStatus `json:"machines"`
		Services map[string]serviceStatus `json:"services"`
	}{
		Machines: ctxt.processMachines(),
		Services: ctxt.processServices(),
	}
	return c.out.Write(ctx, result)
}

// fetchAllInstances returns a map from instance id to instance.
func fetchAllInstances(env environs.Environ) (map[state.InstanceId]environs.Instance, error) {
	defer utils.Timeit("fetchAllInstances()")()
	m := make(map[state.InstanceId]environs.Instance)
	insts, err := env.AllInstances()
	if err != nil {
		return nil, err
	}
	for _, i := range insts {
		m[i.Id()] = i
	}
	return m, nil
}

// fetchAllMachines returns a map from machine id to machine.
func fetchAllMachines(st *state.State) (map[string]*state.Machine, error) {
	defer utils.Timeit("fetchAllMachines()")()
	v := make(map[string]*state.Machine)
	machines, err := st.AllMachines()
	if err != nil {
		return nil, err
	}
	for _, m := range machines {
		v[m.Id()] = m
	}
	return v, nil
}

// fetchAllServicesAndUnits returns a map from service name to service
// and a map from service name to unit name to unit.
func fetchAllServicesAndUnits(st *state.State) (map[string]*state.Service, map[string]map[string]*state.Unit, error) {
	svcMap := make(map[string]*state.Service)
	unitMap := make(map[string]map[string]*state.Unit)
	services, err := st.AllServices()
	if err != nil {
		return nil, nil, err
	}
	for _, s := range services {
		svcMap[s.Name()] = s
		units, err := s.AllUnits()
		if err != nil {
			return nil, nil, err
		}
		svcUnitMap := make(map[string]*state.Unit)
		for _, u := range units {
			svcUnitMap[u.Name()] = u
		}
		unitMap[s.Name()] = svcUnitMap
	}
	return svcMap, unitMap, nil
}

func (ctxt *statusContext) processMachines() map[string]machineStatus {
	defer utils.Timeit("processMachines()")()
	machinesMap := make(map[string]machineStatus)
	for _, m := range ctxt.machines {
		machinesMap[m.Id()] = ctxt.processMachine(m)
	}
	return machinesMap
}

func (ctxt *statusContext) processMachine(machine *state.Machine) (status machineStatus) {
	defer utils.Timeit("processMachine()")()
	status.Life,
		status.AgentVersion,
		status.AgentState,
		status.AgentStateInfo,
		status.Err = ctxt.processAgent(machine)
	status.Series = machine.Series()
	instid, ok := machine.InstanceId()
	if ok {
		status.InstanceId = instid
		instance, ok := ctxt.instances[instid]
		if ok {
			status.DNSName, _ = instance.DNSName()
		} else {
			// Double plus ungood.  There is an instance id recorded
			// for this machine in the state, yet the environ cannot
			// find that id.
			status.InstanceState = "missing"
		}
	} else {
		status.InstanceId = "pending"
		// There's no point in reporting a pending agent state
		// if the machine hasn't been provisioned.  This
		// also makes unprovisioned machines visually distinct
		// in the output.
		status.AgentState = ""
	}
	return
}

func (ctxt *statusContext) processServices() map[string]serviceStatus {
	servicesMap := make(map[string]serviceStatus)
	for _, s := range ctxt.services {
		servicesMap[s.Name()] = ctxt.processService(s)
	}
	return servicesMap
}

func (ctxt *statusContext) processService(service *state.Service) (status serviceStatus) {
	url, _ := service.CharmURL()
	status.Charm = url.String()
	status.Exposed = service.IsExposed()
	status.Life = processLife(service)
	var err error
	status.Relations, status.SubordinateTo, err = ctxt.processRelations(service)
	if err != nil {
		status.Err = err
		return
	}
	if service.IsPrincipal() {
		status.Units = ctxt.processUnits(ctxt.units[service.Name()])
	}
	return status
}

func (ctxt *statusContext) processUnits(units map[string]*state.Unit) map[string]unitStatus {
	unitsMap := make(map[string]unitStatus)
	for _, unit := range units {
		unitsMap[unit.Name()] = ctxt.processUnit(unit)
	}
	return unitsMap
}

func (ctxt *statusContext) processUnit(unit *state.Unit) (status unitStatus) {
	status.PublicAddress, _ = unit.PublicAddress()
	if unit.IsPrincipal() {
		status.Machine, _ = unit.AssignedMachineId()
	}
	status.Life,
		status.AgentVersion,
		status.AgentState,
		status.AgentStateInfo,
		status.Err = ctxt.processAgent(unit)
	if subUnits := unit.SubordinateNames(); len(subUnits) > 0 {
		status.Subordinates = make(map[string]unitStatus)
		for _, name := range subUnits {
			subUnit := ctxt.unitByName(name)
			status.Subordinates[name] = ctxt.processUnit(subUnit)
		}
	}
	return
}

func (ctxt *statusContext) unitByName(name string) *state.Unit {
	serviceName := strings.Split(name, "/")[0]
	return ctxt.units[serviceName][name]
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
	AgentTools() (*state.Tools, error)
	Status() (params.Status, string, error)
}

// processAgent retrieves version and status information from the given entity
// and sets the destination version, status and info values accordingly.
func (ctxt *statusContext) processAgent(entity stateAgent) (life string, version string, status params.Status, info string, err error) {
	defer utils.Timeit("processAgent()")()
	life = processLife(entity)
	if t, err := entity.AgentTools(); err == nil {
		version = t.Binary.Number.String()
	}
	toc := utils.Timeit("processAgent.entity.Status()")
	queryInfo = false
	globalKey := "m#" + entity.Id()
	if status, ok := ctxt.statuses[
	status, info, err = entity.Status()
	toc()
	if err != nil {
		return
	}
	if status == params.StatusPending {
		// The status is pending - there's no point
		// in enquiring about the agent liveness.
		return
	}
	toc = utils.Timeit("processAgent.entity.AgentAlive()")
	agentAlive, err := entity.AgentAlive()
	toc()
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
	Err            error            `json:"-" yaml:",omitempty"`
	AgentState     params.Status    `json:"agent-state,omitempty" yaml:"agent-state,omitempty"`
	AgentStateInfo string           `json:"agent-state-info,omitempty" yaml:"agent-state-info,omitempty"`
	AgentVersion   string           `json:"agent-version,omitempty" yaml:"agent-version,omitempty"`
	DNSName        string           `json:"dns-name,omitempty" yaml:"dns-name,omitempty"`
	InstanceId     state.InstanceId `json:"instance-id,omitempty" yaml:"instance-id,omitempty"`
	InstanceState  string           `json:"instance-state,omitempty" yaml:"instance-state,omitempty"`
	Life           string           `json:"life,omitempty" yaml:"life,omitempty"`
	Series         string           `json:"series,omitempty" yaml:"series,omitempty"`
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
