package main

import (
	"fmt"
	"strconv"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
)

type StatusCommand struct {
	EnvName string
	out     cmd.Output
}

var statusDoc = "This command will report on the runtime state of various system entities."

func (c *StatusCommand) Info() *cmd.Info {
	return &cmd.Info{
		"status", "", "output status information about an environment", statusDoc,
	}
}

func (c *StatusCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
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

	result := m()

	result["machines"], err = processMachines(machines, instances)
	if err != nil {
		return err
	}

	result["services"], err = processServices(services)
	if err != nil {
		return err
	}

	if c.out.Name() == "json" {
		return c.out.Write(ctx, jsonify(result))
	}
	return c.out.Write(ctx, result)
}

// fetchAllInstances returns a map[string]environs.Instance representing
// a mapping of instance ids to their respective instance.
func fetchAllInstances(env environs.Environ) (map[string]environs.Instance, error) {
	m := make(map[string]environs.Instance)
	insts, err := env.AllInstances()
	if err != nil {
		return nil, err
	}
	for _, i := range insts {
		m[i.Id()] = i
	}
	return m, nil
}

// fetchAllMachines returns a map[int]*state.Machine representing
// a mapping of machine ids to machines.
func fetchAllMachines(st *state.State) (map[int]*state.Machine, error) {
	v := make(map[int]*state.Machine)
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
func processMachines(machines map[int]*state.Machine, instances map[string]environs.Instance) (map[int]interface{}, error) {
	r := make(map[int]interface{})
	for _, m := range machines {
		instid, err := m.InstanceId()
		if err, ok := err.(*state.NotFoundError); ok {
			r[m.Id()] = map[string]interface{}{
				"instance-id": "pending",
			}
		} else if err != nil {
			return nil, err
		} else {
			instance, ok := instances[instid]
			if !ok {
				// Double plus ungood. There is an instance id recorded for this machine in the state,
				// yet the environ cannot find that id. 
				return nil, fmt.Errorf("instance %s for machine %d not found", instid, m.Id())
			}
			r[m.Id()] = checkError(processMachine(m, instance))
		}
	}
	return r, nil
}

func processMachine(machine *state.Machine, instance environs.Instance) (map[string]interface{}, error) {
	r := m()
	r["instance-id"] = instance.Id()

	if dnsname, err := instance.DNSName(); err == nil {
		r["dns-name"] = dnsname
	}

	processVersion(r, machine)
	processAgentStatus(r, machine)

	// TODO(dfc) unit-status
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
	r["charm"] = ch.Meta().Name

	if exposed, err := service.IsExposed(); err == nil {
		r["exposed"] = exposed
	}

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

	if addr, err := unit.PublicAddress(); err == nil {
		r["public-address"] = addr
	}

	if id, err := unit.AssignedMachineId(); err == nil {
		// TODO(dfc) we could make this nicer, ie machine/0
		r["machine"] = id
	}

	processVersion(r, unit)
	processStatus(r, unit)
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

type status interface {
	Status() (state.UnitStatus, string, error)
}

func processStatus(r map[string]interface{}, s status) {
	if status, info, err := s.Status(); err == nil {
		r["status"] = status
		if len(info) > 0 {
			r["status-info"] = info
		}
	}
}

type agent interface {
	AgentAlive() (bool, error)
}

func processAgentStatus(r map[string]interface{}, a agent) {
	if alive, err := a.AgentAlive(); err == nil && alive {
		r["agent-state"] = "running"
	}
}

// jsonify converts the keys of the machines map into their string
// equivalents for compatibility with encoding/json.
func jsonify(r map[string]interface{}) map[string]map[string]interface{} {
	m := map[string]map[string]interface{}{
		"services": r["services"].(map[string]interface{}),
		"machines": m(),
	}
	for k, v := range r["machines"].(map[int]interface{}) {
		m["machines"][strconv.Itoa(k)] = v
	}
	return m
}

func m() map[string]interface{} { return make(map[string]interface{}) }

func checkError(m map[string]interface{}, err error) map[string]interface{} {
	if err != nil {
		return map[string]interface{}{"status-error": err.Error()}
	}
	return m
}
