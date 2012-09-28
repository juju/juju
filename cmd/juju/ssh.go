package main

import (
	"errors"
	"os/exec"
	"strconv"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
)

// SSHCommand is responsible for launching a ssh shell on a given unit or machine.
type SSHCommand struct {
	EnvName string
	Target  string
	Args    []string
	*juju.Conn
}

func (c *SSHCommand) Info() *cmd.Info {
	return &cmd.Info{"ssh", "", "launch an ssh shell on a given unit or machine", ""}
}

func (c *SSHCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	if len(args) == 0 {
		return errors.New("no service name specified")
	}
	c.Target, c.Args = args[0], args[1:]
	return nil
}

// Run resolves c.Target to a machine, to the address of a i
// machine or unit forks ssh passing any arguments provided.
func (c *SSHCommand) Run(ctx *cmd.Context) error {
	var err error
	c.Conn, err = juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer c.Close()
	host, err := c.hostFromTarget()
	if err != nil {
		return err
	}
	args := []string{"-l", "ubuntu", "-t", "-o", "StrictHostKeyChecking no", "-o", "PasswordAuthentication no", host}
	args = append(args, c.Args...)
	cmd := exec.Command("ssh", args...)
	cmd.Stdin = ctx.Stdin
	cmd.Stdout = ctx.Stdout
	cmd.Stderr = ctx.Stderr
	return cmd.Run()
}

func (c *SSHCommand) hostFromTarget() (string, error) {
	// is the target the id of a machine ?
	if id, err := strconv.Atoi(c.Target); err == nil {
		log.Printf("looking up address for machine %d...", id)
		// TODO(dfc) maybe we should have machine.PublicAddres() ?
		return c.machinePublicAddress(id)
	}
	// maybe the target is a unit
	if state.IsUnitName(c.Target) {
		log.Printf("Looking up address for unit %q...", c.Target)
		unit, err := c.State.Unit(c.Target)
		if err != nil {
			return "", err
		}
		return unit.PublicAddress()
	}
	return "", errors.New("no such unit or machine")
}

func (c *SSHCommand) machinePublicAddress(id int) (string, error) {
	machine, err := c.State.Machine(id)
	if err != nil {
		return "", err
	}
	// wait for instance id
	w := machine.Watch()
	for _ = range w.Changes() {
		instid, err := machine.InstanceId()
		if err == nil {
			w.Stop()
			inst, err := c.Environ.Instances([]string{instid})
			if err != nil {
				return "", err
			}
			return inst[0].WaitDNSName()
		}
	}
	// oops, watcher closed before we could get an answer
	return "", w.Stop()
}
