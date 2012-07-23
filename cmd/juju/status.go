package main

import (
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

type result struct {
	Machines map[string]interface{} `yaml:"machines" json:"machines"`
	Services map[string]interface{} `yaml:"services" json:"services"`
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

	r := result{
		make(map[string]interface{}),
		make(map[string]interface{}),
	}

	r.Machines, err = processMachines(machines, instances)
	if err != nil {
		return err
	}

	// TODO(dfc) process services and units

	return c.out.Write(ctx, r)
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
// nb. due to a limitation encoding/json, the key of the map is a string, not an int.
func processMachines(machines map[int]*state.Machine, instances map[string]environs.Instance) (map[string]interface{}, error) {
	r := make(map[string]interface{})
	var err error
	for _, m := range machines {
		r[strconv.Itoa(m.Id())], err = processMachine(m, instances)
		if err != nil {
			return nil, err
		}
	}
	return r, nil
}

func processMachine(machine *state.Machine, instances map[string]environs.Instance) (map[string]interface{}, error) {
	r := make(map[string]interface{})

	return r, nil
}
