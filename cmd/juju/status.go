package main

import (
	"fmt"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils/set"
)

type statusMap map[string]interface{}

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

	services, err := fetchAllServices(conn.State)
	if err != nil {
		return err
	}

	result := map[string]interface{}{
		"machines": checkError(processMachines(machines, instances)),
		"services": checkError(processServices(services)),
	}

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
func fetchAllServices(st *state.State) (map[string]*state.Service, error) {
	v := make(map[string]*state.Service)
	services, err := st.AllServices()
	if err != nil {
		return nil, err
	}
	for _, s := range services {
		v[s.Name()] = s
	}
	return v, nil
}

// processMachines gathers information about machines.
func processMachines(machines map[string]*state.Machine, instances map[state.InstanceId]environs.Instance) (statusMap, error) {
	machinesMap := make(statusMap)
	for _, m := range machines {
		instid, ok := m.InstanceId()
		if !ok {
			machinesMap[m.Id()] = statusMap{
				"instance-id": "pending",
			}
		} else {
			instance, ok := instances[instid]
			if !ok {
				// Double plus ungood. There is an instance id recorded for this machine in the state,
				// yet the environ cannot find that id.
				return nil, fmt.Errorf("instance %s for machine %s not found", instid, m.Id())
			}
			machinesMap[m.Id()] = checkError(processMachine(m, instance))
		}
	}
	return machinesMap, nil
}

func processStatus(sm statusMap, status params.Status, info string, agentAlive, entityDead bool) {
	if status != params.StatusPending {
		if !agentAlive && !entityDead {
			// Add the original status to the info, so it's not lost.
			if info != "" {
				info = fmt.Sprintf("(%s: %s)", status, info)
			} else {
				info = fmt.Sprintf("(%s)", status)
			}
			// Agent should be running but it's not.
			status = params.StatusDown
		}
	}
	sm["agent-state"] = status
	if info != "" {
		sm["agent-state-info"] = info
	}
}

func processMachine(machine *state.Machine, instance environs.Instance) (statusMap, error) {
	machineMap := make(statusMap)
	machineMap["instance-id"] = instance.Id()

	if dnsname, err := instance.DNSName(); err == nil {
		machineMap["dns-name"] = dnsname
	}

	processVersion(machineMap, machine)

	agentAlive, err := machine.AgentAlive()
	if err != nil {
		return nil, err
	}
	machineDead := machine.Life() == state.Dead
	status, info, err := machine.Status()
	if err != nil {
		return nil, err
	}
	processStatus(machineMap, status, info, agentAlive, machineDead)

	return machineMap, nil
}

// processServices gathers information about services.
func processServices(services map[string]*state.Service) (statusMap, error) {
	servicesMap := make(statusMap)
	for _, s := range services {
		servicesMap[s.Name()] = checkError(processService(s))
	}
	return servicesMap, nil
}

func processService(service *state.Service) (statusMap, error) {
	serviceMap := make(statusMap)
	ch, _, err := service.Charm()
	if err != nil {
		return nil, err
	}
	serviceMap["charm"] = ch.String()
	serviceMap["exposed"] = service.IsExposed()

	// TODO(dfc) service.IsSubordinate() ?

	units, err := service.AllUnits()
	if err != nil {
		return nil, err
	}

	if u := checkError(processUnits(units)); len(u) > 0 {
		serviceMap["units"] = u
	}

	if r := checkError(processRelations(service)); len(r) > 0 {
		serviceMap["relations"] = r
	}

	return serviceMap, nil
}

func processUnits(units []*state.Unit) (statusMap, error) {
	unitsMap := make(statusMap)
	for _, unit := range units {
		unitsMap[unit.Name()] = checkError(processUnit(unit))
	}
	return unitsMap, nil
}

func processUnit(unit *state.Unit) (statusMap, error) {
	unitMap := make(statusMap)

	if addr, ok := unit.PublicAddress(); ok {
		unitMap["public-address"] = addr
	}

	if id, err := unit.AssignedMachineId(); err == nil {
		// TODO(dfc) we could make this nicer, ie machine/0
		unitMap["machine"] = id
	}

	processVersion(unitMap, unit)

	agentAlive, err := unit.AgentAlive()
	if err != nil {
		return nil, err
	}
	unitDead := unit.Life() == state.Dead
	status, info, err := unit.Status()
	if err != nil {
		return nil, err
	}
	processStatus(unitMap, status, info, agentAlive, unitDead)

	return unitMap, nil
}

func processRelations(service *state.Service) (statusMap, error) {
	// TODO(mue) This way the same relation is read twice (for each service).
	// Maybe add Relations() to state, read them only once and pass them to each
	// call of this function. 
	relations, err := service.Relations()
	if err != nil {
		return nil, err
	}
	relationMap := make(statusMap)
	for _, relation := range relations {
		ep, err := relation.Endpoint(service.Name())
		if err != nil {
			return nil, err
		}
		relationName := ep.Relation.Name
		eps, err := relation.RelatedEndpoints(service.Name())
		if err != nil {
			return nil, err
		}
		serviceNames := []string{}
		if relationMap[relationName] != nil {
			serviceNames = relationMap[relationName].([]string)
		}
		for _, ep := range eps {
			serviceNames = append(serviceNames, ep.ServiceName)
		}
		relationMap[relationName] = serviceNames
	}
	// Normalize service names by removing duplicates and sorting them.
	// TODO(mue) Check if and why duplicates can happen and what this means.
	for relationName, serviceNames := range relationMap {
		sn := set.NewStrings(serviceNames.([]string)...)
		relationMap[relationName] = sn.SortedValues()
	}
	return relationMap, nil
}

type versioned interface {
	AgentTools() (*state.Tools, error)
}

func processVersion(sm statusMap, v versioned) {
	if t, err := v.AgentTools(); err == nil {
		sm["agent-version"] = t.Binary.Number.String()
	}
}

func checkError(sm statusMap, err error) statusMap {
	if err != nil {
		return map[string]interface{}{"status-error": err.Error()}
	}
	return sm
}
