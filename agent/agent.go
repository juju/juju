package agent

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
)

type AgentConf struct {
	JujuDir     string
	Zookeeper   string
	SessionFile string
}

type Agent interface {
	run(conn *zookeeper.Conn, jujuDir string) error
}

func connect(conf *AgentConf) (*zookeeper.Conn, error) {
	// TODO...
	return nil, fmt.Errorf("agent.connect not implemented")
}

func Run(agent Agent, conf *AgentConf) error {
	conn, err := connect(conf)
	if err != nil {
		return err
	}
	return agent.run(conn, conf.JujuDir)
}

type UnitAgent struct {
	conn    *zookeeper.Conn
	jujuDir string
	Name    string
}

func (a *UnitAgent) run(conn *zookeeper.Conn, jujuDir string) error {
	a.conn = conn
	a.jujuDir = jujuDir
	return fmt.Errorf("agent.UnitAgent.Run not implemented")
}

type MachineAgent struct {
	conn    *zookeeper.Conn
	jujuDir string
	Id      string
}

func (a *MachineAgent) run(conn *zookeeper.Conn, jujuDir string) error {
	a.conn = conn
	a.jujuDir = jujuDir
	return fmt.Errorf("agent.MachineAgent.Run not implemented")
}

type ProvisioningAgent struct {
	conn    *zookeeper.Conn
	jujuDir string
}

func (a *ProvisioningAgent) run(conn *zookeeper.Conn, jujuDir string) error {
	a.conn = conn
	a.jujuDir = jujuDir
	return fmt.Errorf("agent.ProvisioningAgent.Run not implemented")
}
