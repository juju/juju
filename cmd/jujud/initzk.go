package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/state"
)

type InitzkCommand struct {
	Zookeeper    string
	InstanceId   string
	ProviderType string
}

func (c *InitzkCommand) Info() *cmd.Info {
	return &cmd.Info{
		"initzk",
		"jujud initzk [options]",
		"initialize juju state in a local zookeeper",
		"",
	}
}

func (c *InitzkCommand) InitFlagSet(f *gnuflag.FlagSet) {
	f.StringVar(&c.Zookeeper, "zookeeper-servers", "127.0.0.1:2181", "address of zookeeper to initialize")
	f.StringVar(&c.InstanceId, "instance-id", "", "instance id of this machine")
	f.StringVar(&c.ProviderType, "provider-type", "", "envionment machine provider type")
}

func (c *InitzkCommand) ParsePositional(args []string) error {
	if c.Zookeeper == "" {
		return requiredError("zookeeper-servers")
	}
	if c.InstanceId == "" {
		return requiredError("instance-id")
	}
	if c.ProviderType == "" {
		return requiredError("provider-type")
	}
	return cmd.CheckEmpty(args)
}

func (c *InitzkCommand) Run() error {
	conn, err := Connect(c.Zookeeper)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := state.Initialize(conn); err != nil {
		return err
	}
	return nil
}

func Connect(zk string) (conn *zookeeper.Conn, err error) {
	conn, session, err := zookeeper.Dial(zk, 5e9)
	if err != nil {
		return nil, err
	}
	event := <-session
	if !event.Ok() {
		conn.Close()
		return nil, fmt.Errorf("%s", event)
	}
	go func() {
		for {
			<-session
		}
	}()
	return conn, nil
}
