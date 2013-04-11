package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
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
func processMachines(machines map[string]*state.Machine, instances map[state.InstanceId]environs.Instance) (map[string]interface{}, error) {
	r := make(map[string]interface{})
	for _, m := range machines {
		instid, ok := m.InstanceId()
		if !ok {
			r[m.Id()] = map[string]interface{}{
				"instance-id": "pending",
			}
		} else {
			instance, ok := instances[instid]
			if !ok {
				// Double plus ungood. There is an instance id recorded for this machine in the state,
				// yet the environ cannot find that id.
				return nil, fmt.Errorf("instance %s for machine %s not found", instid, m.Id())
			}
			r[m.Id()] = checkError(processMachine(m, instance))
		}
	}
	return r, nil
}

func processStatus(r map[string]interface{}, status params.Status, info string, agentAlive, entityDead bool) {
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
	r["agent-state"] = status
	if info != "" {
		r["agent-state-info"] = info
	}
}

func processMachine(machine *state.Machine, instance environs.Instance) (map[string]interface{}, error) {
	r := m()
	r["instance-id"] = instance.Id()

	if dnsname, err := instance.DNSName(); err == nil {
		r["dns-name"] = dnsname
	}

	processVersion(r, machine)

	agentAlive, err := machine.AgentAlive()
	if err != nil {
		return nil, err
	}
	machineDead := machine.Life() == state.Dead
	status, info, err := machine.Status()
	if err != nil {
		return nil, err
	}
	processStatus(r, status, info, agentAlive, machineDead)

	return r, nil
}

// processServices gathers information about services.
func processServices(services map[string]*state.Service) (map[string]interface{}, error) {
	r := m()
	for _, s := range services {
		r[s.Name()] = checkError(processService(s))
	}
	return r, nil
}

func processService(service *state.Service) (map[string]interface{}, error) {
	r := m()
	ch, _, err := service.Charm()
	if err != nil {
		return nil, err
	}
	r["charm"] = ch.String()
	r["exposed"] = service.IsExposed()

	// TODO(dfc) service.IsSubordinate() ?

	units, err := service.AllUnits()
	if err != nil {
		return nil, err
	}

	u := checkError(processUnits(units))
	if len(u) > 0 {
		r["units"] = u
	}

	// TODO(dfc) process relations
	return r, nil
}

func processUnits(units []*state.Unit) (map[string]interface{}, error) {
	r := m()
	for _, unit := range units {
		r[unit.Name()] = checkError(processUnit(unit))
	}
	return r, nil
}

func processUnit(unit *state.Unit) (map[string]interface{}, error) {
	r := m()

	if addr, ok := unit.PublicAddress(); ok {
		r["public-address"] = addr
	}

	if id, err := unit.AssignedMachineId(); err == nil {
		// TODO(dfc) we could make this nicer, ie machine/0
		r["machine"] = id
	}

	processVersion(r, unit)

	agentAlive, err := unit.AgentAlive()
	if err != nil {
		return nil, err
	}
	unitDead := unit.Life() == state.Dead
	status, info, err := unit.Status()
	if err != nil {
		return nil, err
	}
	processStatus(r, status, info, agentAlive, unitDead)

	return r, nil
}

type versioned interface {
	AgentTools() (*state.Tools, error)
}

func processVersion(r map[string]interface{}, v versioned) {
	if t, err := v.AgentTools(); err == nil {
		r["agent-version"] = t.Binary.Number.String()
	}
}

func m() map[string]interface{} { return make(map[string]interface{}) }

func checkError(m map[string]interface{}, err error) map[string]interface{} {
	if err != nil {
		return map[string]interface{}{"status-error": err.Error()}
	}
	return m
}
