// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/network"
	"github.com/juju/juju/utils/ssh"
)

type sshClient struct {
	client  ssh.Client
	host    string
	options *ssh.Options
}

func newSshClient(host string) *sshClient {
	options := ssh.Options{}
	options.SetIdentities("/var/lib/juju/system-identity")
	return &sshClient{
		client:  ssh.DefaultClient,
		host:    "ubuntu@" + host,
		options: &options,
	}
}

func (c *sshClient) configureExternalIpAddress(apiPort int) error {
	cmd := `printf 'auto eth1\niface eth1 inet dhcp' | sudo tee -a /etc/network/interfaces.d/eth1.cfg
sudo ifup eth1
sudo iptables -i eth1 -I INPUT -j DROP`

	if apiPort > 0 {
		cmd += fmt.Sprintf("\nsudo iptables -I INPUT -p tcp --dport %d -j ACCEPT", apiPort)
	}

	command := c.client.Command(c.host, []string{"/bin/bash"}, c.options)
	command.Stdin = strings.NewReader(cmd)
	output, err := command.CombinedOutput()
	if err != nil {
		return errors.Errorf("failed to allocate external IP address: %s", output)
	}
	logger.Tracef("configure external ip address output: %s", output)
	return nil
}

func (c *sshClient) changePorts(ipAddress string, insert bool, ports []network.PortRange) error {
	cmd := ""
	insertArg := "-I"
	if !insert {
		insertArg = "-D"
	}
	for _, port := range ports {
		if port.ToPort-port.FromPort > 0 {
			cmd += fmt.Sprintf("sudo iptables -d %s %s INPUT -p %s --match multiport --dports %d:%d -j ACCEPT\n", ipAddress, insertArg, port.Protocol, port.FromPort, port.ToPort)
		} else {

			cmd += fmt.Sprintf("sudo iptables -d %s %s INPUT -p %s --dport %d -j ACCEPT\n", ipAddress, insertArg, port.Protocol, port.FromPort)
		}
	}
	cmd += "sudo /etc/init.d/iptables-persistent save\n"
	command := c.client.Command(c.host, []string{"/bin/bash"}, c.options)
	command.Stdin = strings.NewReader(cmd)
	output, err := command.CombinedOutput()
	if err != nil {
		return errors.Errorf("failed to configure ports on external network: %s", output)
	}
	logger.Tracef("change ports output: %s", output)
	return nil
}

func (c *sshClient) findOpenPorts() ([]network.PortRange, error) {
	cmd := "sudo iptables -L INPUT -n"
	command := c.client.Command(c.host, []string{"/bin/bash"}, c.options)
	command.Stdin = strings.NewReader(cmd)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, errors.Errorf("failed to list open ports: %s", output)
	}
	logger.Tracef("find open ports output: %s", output)

	//the output have the following format, we will skipp all other rules
	//Chain INPUT (policy ACCEPT)
	//target     prot opt source               destination
	//ACCEPT     tcp  --  0.0.0.0/0            192.168.0.1  multiport dports 3456:3458
	//ACCEPT     tcp  --  0.0.0.0/0            192.168.0.2  tcp dpt:12345

	res := make([]network.PortRange, 0)
	var addSinglePortRange = func(items []string) {
		ports := strings.Split(items[6], ":")
		if len(ports) != 2 {
			return
		}
		to, err := strconv.ParseInt(ports[1], 10, 32)
		if err != nil {
			return
		}

		res = append(res, network.PortRange{
			Protocol: items[1],
			FromPort: int(to),
			ToPort:   int(to),
		})
	}
	var addMultiplePortRange = func(items []string) {
		ports := strings.Split(items[7], ":")
		if len(ports) != 2 {
			return
		}
		from, err := strconv.ParseInt(ports[0], 10, 32)
		if err != nil {
			return
		}
		to, err := strconv.ParseInt(ports[1], 10, 32)
		if err != nil {
			return
		}

		res = append(res, network.PortRange{
			Protocol: items[1],
			FromPort: int(from),
			ToPort:   int(to),
		})
	}

	for i, line := range strings.Split(string(output), "\n") {
		if i == 1 || i == 0 {
			continue
		}
		items := strings.Split(line, " ")
		if len(items) == 7 && items[0] == "ACCEPT" && items[3] == "0.0.0.0/0" {
			addSinglePortRange(items)
		}
		if len(items) == 8 && items[0] == "ACCEPT" && items[3] == "0.0.0.0/0" && items[5] != "multiport" && items[6] != "dports" {
			addMultiplePortRange(items)
		}
	}
	return res, nil
}

func (c *sshClient) addIpAddress(nic string, addr string) error {
	cmd := fmt.Sprintf("ls /etc/network/interfaces.d | grep %s: | sed 's/%s://' | sed 's/.cfg//' | tail -1", nic, nic)
	command := c.client.Command(c.host, []string{"/bin/bash"}, c.options)
	command.Stdin = strings.NewReader(cmd)
	lastIndStr, err := command.CombinedOutput()
	if err != nil {
		return errors.Errorf("failed to obtain last device index: %s", lastIndStr)
	}
	lastInd := 0
	if ind, err := strconv.ParseInt(string(lastIndStr), 10, 64); err != nil {
		lastInd = int(ind) + 1
	}
	nic = fmt.Sprintf("%s:%d", nic, lastInd)
	cmd = fmt.Sprintf("printf 'auto %s\\niface %s inet static\\naddress %s' | sudo tee -a /etc/network/interfaces.d/%s.cfg\nsudo ifup %s", nic, nic, addr, nic, nic)

	command = c.client.Command(c.host, []string{"/bin/bash"}, c.options)
	command.Stdin = strings.NewReader(cmd)
	output, err := command.CombinedOutput()
	if err != nil {
		return errors.Errorf("failed to add IP address: %s", output)
	}
	logger.Tracef("add ip address output: %s", output)
	return nil
}

func (c *sshClient) releaseIpAddress(_ string, addr string) error {
	cmd := fmt.Sprintf("ip addr show | grep %s | awk '{print $7}'", addr)
	command := c.client.Command(c.host, []string{"/bin/bash"}, c.options)
	command.Stdin = strings.NewReader(cmd)
	nic, err := command.CombinedOutput()
	if err != nil {
		return errors.Errorf("faild to get nic by ip address: %s", nic)
	}

	cmd = fmt.Sprintf("sudo rm %s.cfg \nsudo ifdown %s", nic, nic)
	command = c.client.Command(c.host, []string{"/bin/bash"}, c.options)
	command.Stdin = strings.NewReader(cmd)
	output, err := command.CombinedOutput()
	if err != nil {
		return errors.Errorf("failed to release IP address: %s", output)
	}
	logger.Tracef("release ip address output: %s", output)
	return nil
}
