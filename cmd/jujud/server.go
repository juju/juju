package main

import (
	"encoding/base64"
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
)

type ServerCommand struct {
	Conf       AgentConf
	Addr string
	CertFile string
	KeyFile string
}

func (c *ServerCommand) Info() *cmd.Info {
	return &cmd.Info{"server", "", "run juju API server", ""}
}

func (c *ServerCommand) Init(f *gnuflag.FlagSet, args []string) error {
	c.Conf.addFlags(f, flagStateInfo|flagInitialPassword)
	f.StringVar(&c.Addr, "addr", "", "server address to listen on")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if c.Addr == "" {
		return requiredError("addr")
	}
	return c.Conf.checkArgs(f.Args())
}

func (c *ServerCommand) Run(_ *cmd.Context) error {
}

func (c *ServerCommand) runOnce() error {
	st, password, err := openState(state.UnitEntityName(a.UnitName), &a.Conf)
	if err != nil {
		return err
	}
	defer st.Close()

	// TODO set password in state
	_ = password

	lis, err := net.Listen("tcp", c.Addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %q: %v", c.Addr, err)
	}
	return api.Serve(
