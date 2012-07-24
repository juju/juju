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
		"status", "", "Output status information about an environment.", statusDoc,
	}
}

func (c *StatusCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	c.out.AddFlags(f, "yaml", cmd.DefaultFormatters)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

func (c *StatusCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConn(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()

	instances, err := fetchAllInstances(conn.Environ)
	if err != nil {
		return err
	}

	state, err := conn.State()
	if err != nil {
		return err
	}

	machines, err := fetchAllMachines(state)
	if err != nil {
		return err
	}

	var result = make(map[string]interface{})

	result["machines"], err = processMachines(machines, instances)
	if err != nil {
		return err
	}

	// TODO(dfc) process services and units
	result["services"] = make(map[string]interface{})

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
// a mapping of machine ids to their respective machine.
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

// processMachines gathers information about machines.
// nb. due to the limitations of encoding/json, the key of the map is a string, not an int.
func processMachines(machines map[int]*state.Machine, instances map[string]environs.Instance) (map[int]interface{}, error) {
	r := make(map[int]interface{})
	for _, m := range machines {
		instid, err := m.InstanceId()
		if err, ok := err.(*state.NoInstanceIdError); ok {
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
			machine, err := processMachine(m, instance)
			if err != nil {
				return nil, err
			}
			r[m.Id()] = machine
		}
	}
	return r, nil
}

func processMachine(machine *state.Machine, instance environs.Instance) (map[string]interface{}, error) {
	r := make(map[string]interface{})
	dnsname, err := instance.DNSName()
	if err != nil {
		return nil, err
	}
	r["dns-name"] = dnsname
	r["instance-id"] = instance.Id()

	alive, err := machine.AgentAlive()
	if err != nil {
		return nil, err
	}

	// TODO(dfc) revisit this once unit-status is done
	if alive {
		r["agent-state"] = "running"
	}

	// TODO(dfc) unit-status
	return r, nil
}

// jsonify converts the result struct into a structure which is compatibile with
// encoding/json.
func jsonify(r map[string]interface{}) map[string]map[string]interface{} {
	m := map[string]map[string]interface{}{
		"services": r["services"].(map[string]interface{}),
		"machines": make(map[string]interface{}),
	}
	for k, v := range r["machines"].(map[int]interface{}) {
		m["machines"][strconv.Itoa(k)] = v
	}
	return m
}
