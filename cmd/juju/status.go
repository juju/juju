package main

import (
	"encoding/json"
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
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
}

func (c *StatusCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()

	instances, err := fetchAllInstances(conn.Environ)
	if err != nil {
		return err
	}

	machines, err := fetchAllMachines(conn.State)
	if err != nil {
		return err
	}

	services, units, err := fetchAllServicesAndUnits(conn.State)
	if err != nil {
		return err
	}

	ctxt := &statusContext{
		instances: instances,
		machines:  machines,
		services:  services,
		units:     units,
	}

	var result struct {
		Machines map[string]machineStatus `json:"machines"`
		Services map[string]serviceStatus `json:"services"`
	}
	result.Machines = ctxt.processMachines()
	result.Services = ctxt.processServices()

	return c.out.Write(ctx, result)
}

// fetchAllInstances returns a map[string]environs.Instance representing
// a mapping of instance ids to their respective instance.
func fetchAllInstances(env environs.Environ) (map[state.InstanceId]environs.Instance, error) {
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

// fetchAllMachines returns a map[string]*state.Machine representing
// a mapping of machine ids to machines.
func fetchAllMachines(st *state.State) (map[string]*state.Machine, error) {
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

// fetchAllServices returns a map representing a mapping of service
// names to services.
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

// processMachines gathers information about machines.
func (ctxt *statusContext) processMachines() map[string]machineStatus {
	machinesMap := make(map[string]machineStatus)
	for _, m := range ctxt.machines {
		var mstatus machineStatus
		instid, ok := m.InstanceId()
		if !ok {
			mstatus.InstanceId = "pending"
		} else {
			instance, ok := ctxt.instances[instid]
			if !ok {
				// Double plus ungood.  There is an
				// instance id recorded for this machine
				// in the state, yet the environ cannot
				// find that id.
				mstatus.Err = fmt.Errorf("instance %s not found", instid)
			} else {
				mstatus = processMachine(m, instance)
			}
		}
		machinesMap[m.Id()] = mstatus
	}
	return machinesMap
}

type statuser interface {
	Life() state.Life
	AgentAlive() (bool, error)
	Status() (params.Status, string, error)
}

func processStatus(dstStatus *params.Status, dstInfo *string, entity statuser) error {
	agentAlive, err := entity.AgentAlive()
	if err != nil {
		return err
	}
	entityDead := entity.Life() == state.Dead
	status, info, err := entity.Status()
	if err != nil {
		return err
	}
	if status != params.StatusPending && !agentAlive && !entityDead {
		// Add the original status to the info, so it's not lost.
		if info != "" {
			info = fmt.Sprintf("(%s: %s)", status, info)
		} else {
			info = fmt.Sprintf("(%s)", status)
		}
		// Agent should be running but it's not.
		status = params.StatusDown
	}
	*dstStatus = status
	*dstInfo = info
	return nil
}

func processMachine(machine *state.Machine, instance environs.Instance) (status machineStatus) {
	status.InstanceId = instance.Id()
	status.DNSName, _ = instance.DNSName()
	processVersion(&status.AgentVersion, machine)
	status.Err = processStatus(&status.AgentState, &status.AgentStateInfo, machine)
	return
}

// processServices gathers information about services.
func (ctxt *statusContext) processServices() map[string]serviceStatus {
	servicesMap := make(map[string]serviceStatus)
	for _, s := range ctxt.services {
		servicesMap[s.Name()] = ctxt.processService(s)
	}
	return servicesMap
}

func (ctxt *statusContext) processService(service *state.Service) (status serviceStatus) {
	ch, _, err := service.Charm()
	if err != nil {
		status.Err = err
		return
	}
	status.Charm = ch.String()
	status.Exposed = service.IsExposed()

	status.Relations, status.SubordinateTo, err = processRelations(service)
	if err != nil {
		status.Err = err
		return
	}
	status.Units = make(map[string]unitStatus)
	units, err := service.AllUnits()
	if err != nil {
		status.Err = err
		return
	}
	if !service.IsSubordinate() {
		status.Units = ctxt.processUnits(units)
	}
	return status
}

func (ctxt *statusContext) processUnits(units []*state.Unit) map[string]unitStatus {
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
	processVersion(&status.AgentVersion, unit)

	if err := processStatus(&status.AgentState, &status.AgentStateInfo, unit); err != nil {
		status.Err = err
		return
	}
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

func processRelations(service *state.Service) (related map[string][]string, subord []string, err error) {
	log.Infof("processing relations of service %v", service)
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
			if ep.Scope == charm.ScopeContainer && service.IsSubordinate() {
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

type versioned interface {
	AgentTools() (*state.Tools, error)
}

func processVersion(status *string, v versioned) {
	if t, err := v.AgentTools(); err == nil {
		*status = t.Binary.Number.String()
	} else {
		*status = ""
	}
}

type machineStatus struct {
	Err            error            `json:"-" yaml:",omitempty"`
	InstanceId     state.InstanceId `json:"instance-id" yaml:"instance-id"`
	DNSName        string           `json:"dns-name,omitempty" yaml:"dns-name,omitempty"`
	AgentVersion   string           `json:"agent-version,omitempty" yaml:"agent-version,omitempty"`
	AgentState     params.Status    `json:"agent-state,omitempty" yaml:"agent-state,omitempty"`
	AgentStateInfo string           `json:"agent-state-info,omitempty" yaml:"agent-state-info,omitempty"`
}

// A goyaml bug means we can't declare these types
// locally to the GetYAML methods.
type machineStatusNoMarshal machineStatus

func (s machineStatus) MarshalJSON() ([]byte, error) {
	if s.Err != nil {
		return marshalError(s.Err)
	}
	return json.Marshal(machineStatusNoMarshal(s))
}

func (s machineStatus) GetYAML() (tag string, value interface{}) {
	if s.Err != nil {
		return "", errorStatus{s.Err.Error()}
	}
	type mNoMethods machineStatus
	return "", mNoMethods(s)
}

type serviceStatus struct {
	Err           error                 `json:"-" yaml:",omitempty"`
	Charm         string                `json:"charm" yaml:"charm"`
	Exposed       bool                  `json:"exposed" yaml:"exposed"`
	Units         map[string]unitStatus `json:"units,omitempty" yaml:"units,omitempty"`
	Relations     map[string][]string   `json:"relations,omitempty" yaml:"relations,omitempty"`
	SubordinateTo []string              `json:"subordinate-to,omitempty" yaml:"subordinate-to,omitempty"`
}
type serviceStatusNoMarshal serviceStatus

func (s serviceStatus) MarshalJSON() ([]byte, error) {
	if s.Err != nil {
		return marshalError(s.Err)
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
	PublicAddress  string                `json:"public-address,omitempty" yaml:"public-address,omitempty"`
	Machine        string                `json:"machine,omitempty" yaml:"machine,omitempty"`
	AgentVersion   string                `json:"agent-version,omitempty" yaml:"agent-version,omitempty"`
	AgentState     params.Status         `json:"agent-state,omitempty" yaml:"agent-state,omitempty"`
	AgentStateInfo string                `json:"agent-state-info,omitempty" yaml:"agent-state-info,omitempty"`
	Subordinates   map[string]unitStatus `json:"subordinates,omitempty" yaml:"subordinates,omitempty"`
}

type unitStatusNoMarshal unitStatus

func (s unitStatus) MarshalJSON() ([]byte, error) {
	if s.Err != nil {
		return marshalError(s.Err)
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

func marshalError(err error) ([]byte, error) {
	return []byte(fmt.Sprintf(`{"status-error": %q}`, err)), nil
}

type errorStatus struct {
	StatusError string `yaml:"status-error"`
}
