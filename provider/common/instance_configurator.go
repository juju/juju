// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/ssh"

	"github.com/juju/juju/network"
	"github.com/juju/juju/network/iptables"
)

const (
	iptablesComment = "managed by juju"
)

// Implementations of this interface should provide a way to configure external
// IP allocation and add firewall functionality.
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/instance_configurator.go github.com/juju/juju/provider/common InstanceConfigurator
type InstanceConfigurator interface {

	// Close all ports.
	DropAllPorts(exceptPorts []int, addr string) error

	// Add network interface and allocate external IP address.
	// Implementations should also configure this interface and initialise  ports state.
	ConfigureExternalIpAddress(apiPort int) error

	// Open or close ports.
	ChangeIngressRules(ipAddress string, insert bool, rules []network.IngressRule) error

	// List all ingress rules.
	FindIngressRules() ([]network.IngressRule, error)
}

type sshInstanceConfigurator struct {
	client  ssh.Client
	host    string
	options *ssh.Options
}

// NewSshInstanceConfigurator creates new sshInstanceConfigurator.
func NewSshInstanceConfigurator(host string) InstanceConfigurator {
	options := ssh.Options{}
	options.SetIdentities("/var/lib/juju/system-identity")

	// Disable host key checking. We're not sending any sensitive data
	// across, and we don't have access to the host's keys from here.
	//
	// TODO(axw) 2017-12-07 #1732665
	// Stop using SSH, instead manage iptables on the machine
	// itself. This will also provide firewalling for MAAS and
	// LXD machines.
	options.SetStrictHostKeyChecking(ssh.StrictHostChecksNo)
	options.SetKnownHostsFile(os.DevNull)

	return &sshInstanceConfigurator{
		client:  ssh.DefaultClient,
		host:    "ubuntu@" + host,
		options: &options,
	}
}

func (c *sshInstanceConfigurator) runCommand(cmd string) (string, error) {
	command := c.client.Command(c.host, []string{"/bin/bash"}, c.options)
	command.Stdin = strings.NewReader(cmd)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", errors.Trace(err)
	}
	return string(output), nil
}

// DropAllPorts implements InstanceConfigurator interface.
func (c *sshInstanceConfigurator) DropAllPorts(exceptPorts []int, addr string) error {
	cmds := []string{
		iptables.DropCommand{DestinationAddress: addr}.Render(),
	}
	for _, port := range exceptPorts {
		cmds = append(cmds, iptables.AcceptInternalCommand{
			Protocol:           "tcp",
			DestinationAddress: addr,
			DestinationPort:    port,
		}.Render())
	}

	output, err := c.runCommand(strings.Join(cmds, "\n"))
	if err != nil {
		return errors.Errorf("failed to drop all ports: %s", output)
	}
	logger.Tracef("drop all ports output: %s", output)
	return nil
}

// ConfigureExternalIpAddressCommands returns the commands to run to configure
// the external IP address
func ConfigureExternalIpAddressCommands(apiPort int) []string {
	commands := []string{
		`printf 'auto eth1\niface eth1 inet dhcp' | sudo tee -a /etc/network/interfaces.d/eth1.cfg`,
		"sudo ifup eth1",
		iptables.DropCommand{Interface: "eth1"}.Render(),
	}
	if apiPort > 0 {
		commands = append(commands, iptables.AcceptInternalCommand{
			Protocol:        "tcp",
			DestinationPort: apiPort,
		}.Render())
	}
	return commands
}

// ConfigureExternalIpAddress implements InstanceConfigurator interface.
func (c *sshInstanceConfigurator) ConfigureExternalIpAddress(apiPort int) error {
	cmds := ConfigureExternalIpAddressCommands(apiPort)
	output, err := c.runCommand(strings.Join(cmds, "\n"))
	if err != nil {
		return errors.Errorf("failed to allocate external IP address: %s", output)
	}
	logger.Tracef("configure external ip address output: %s", output)
	return nil
}

// ChangeIngressRules implements InstanceConfigurator interface.
func (c *sshInstanceConfigurator) ChangeIngressRules(ipAddress string, insert bool, rules []network.IngressRule) error {
	var cmds []string
	for _, rule := range rules {
		cmds = append(cmds, iptables.IngressRuleCommand{
			Rule:               rule,
			DestinationAddress: ipAddress,
			Delete:             !insert,
		}.Render())
	}

	output, err := c.runCommand(strings.Join(cmds, "\n"))
	if err != nil {
		return errors.Annotatef(err, "configuring ports for address %q: %s", ipAddress, output)
	}
	logger.Tracef("change ports output: %s", output)
	return nil
}

// FindIngressRules implements InstanceConfigurator interface.
func (c *sshInstanceConfigurator) FindIngressRules() ([]network.IngressRule, error) {
	output, err := c.runCommand("sudo iptables -L INPUT -n")
	if err != nil {
		return nil, errors.Errorf("failed to list open ports: %s", output)
	}
	logger.Tracef("find open ports output: %s", output)
	return iptables.ParseIngressRules(strings.NewReader(output))
}
