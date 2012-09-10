package main

import (
	"errors"
	"os/exec"
	"strconv"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

// SSHCommand is responsible for launchin an ssh shell on a given unit or machine.
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
	switch len(args) {
	case 0:
		return errors.New("no service name specified")
	default:
		c.Args = args[1:]
		fallthrough
	case 1:
		c.Target = args[0]
	}
	return nil
}

// Run resolves c.Target to a machine, or host of a unit and
// forks ssh with c.Args, if provided.
func (c *SSHCommand) Run(ctx *cmd.Context) error {
	var err error
	c.Conn, err = juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer c.Close()
	host, err := c.hostFromTarget()
	args := []string{"-l", "ubuntu", "-t", "-o", "StrictHostKeyChecking no", "-o", "PasswordAuthentication no", host, "--"}
	args = append(args, c.Args...)
	cmd := exec.Command("/usr/bin/ssh", args...)
	cmd.Stdin = ctx.Stdin
	cmd.Stdout = ctx.Stdout
	cmd.Stderr = ctx.Stderr
	return cmd.Run()
}

func (c *SSHCommand) hostFromTarget() (string, error) {
	// is the target the id of a machine ?
	if id, err := strconv.Atoi(c.Target); err == nil {
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
				return inst[0].DNSName()
			}
		}
		// oops, watcher closed before we could get an answer
		return "", w.Stop()
	}
	// maybe the target is a unit
	if unit, err := c.State.Unit(c.Target); err == nil {
		id, err := unit.AssignedMachineId()
		// TODO(dfc) add a watcher here
		if err != nil {
			return "", err
		}
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
				return inst[0].DNSName()
			}
		}
		// oops, watcher closed before we could get an answer
		return "", w.Stop()
	}
	return "", errors.New("no such unit or machine")
}
