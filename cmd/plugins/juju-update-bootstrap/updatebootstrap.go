// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	_ "launchpad.net/juju-core/provider/all"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/utils"
)

var logger = loggo.GetLogger("juju.plugins.updatebootstrap")

const updateBootstrapDoc = `
Patches all machines after state server has been restored from backup, to
update state server address to new location.
`

type updateBootstrapCommand struct {
	cmd.EnvCommandBase
}

func (c *updateBootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "juju-update-bootstrap",
		Purpose: "update all machines after recovering state server",
		Doc:     updateBootstrapDoc,
	}
}

func (c *updateBootstrapCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	stateAddr, err := GetStateAddress(conn.Environ)
	if err != nil {
		return err
	}
	fmt.Printf("using state address %v\n", stateAddr)
	return updateAllMachines(conn, stateAddr)
}

// GetStateAddress returns the address of one state server
func GetStateAddress(environ environs.Environ) (string, error) {
	// XXX: Can easily look up state server address using api instead
	stateInfo, _, err := environ.StateInfo()
	if err != nil {
		return "", err
	}
	return strings.Split(stateInfo.Addrs[0], ":")[0], nil
}

var agentAddressTemplate = `
set -ex
cd /var/lib/juju/agents
for agent in *; do
	initctl stop jujud-$agent
	sed -i.old -r "/^(stateaddresses|apiaddresses):/{
		n
		s/- .*(:[0-9]+)/- $ADDR\1/
	}" $agent/agent.conf
	initctl start jujud-$agent
done
sed -i -r 's/^(:syslogtag, startswith, "juju-" @)(.*)(:[0-9]+)$/\1$ADDR\3/' /etc/rsyslog.d/*-juju*.conf
`

// renderScriptArg generates an ssh script argument to update state addresses
func renderScriptArg(stateAddr string) string {
	script := strings.Replace(agentAddressTemplate, "$ADDR", stateAddr, -1)
	return "sudo sh -c " + utils.ShQuote(script)
}

// runMachineUpdate connects via ssh to the machine and runs the update script
func runMachineUpdate(m *state.Machine, sshArg string) error {
	addr := instance.SelectPublicAddress(m.Addresses())
	if addr == "" {
		return fmt.Errorf("no appropriate public address found")
	}
	args := []string{
		"-l", "ubuntu",
		"-T",
		"-o", "StrictHostKeyChecking no",
		"-o", "PasswordAuthentication no",
		addr,
		sshArg,
	}
	c := exec.Command("ssh", args...)
	if data, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("ssh command failed: %v (%q)", err, data)
	}
	return nil
}

// updateAllMachines finds all machines resets the stored state address
func updateAllMachines(conn *juju.Conn, stateAddr string) error {
	// XXX(gz): includes the state server(s) which don't want updating
	machines, err := conn.State.AllMachines()
	if err != nil {
		return err
	}
	for _, machine := range machines {
		logger.Infof("updating machine: %v\n", machine)
		runMachineUpdate(machine, renderScriptArg(stateAddr))
	}
	return nil
}

func Main(args []string) {
	if err := juju.InitJujuHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	command := updateBootstrapCommand{}
	os.Exit(cmd.Main(&command, cmd.DefaultContext(), args[1:]))
}

func main() {
	Main(os.Args)
}
