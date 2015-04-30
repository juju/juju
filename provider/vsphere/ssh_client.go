// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"fmt"
	"github.com/juju/errors"
	"golang.org/x/crypto/ssh"

	"github.com/juju/juju/network"
)

type sshClient struct {
	client *ssh.Client
}

func newSshClient(ipAddress, key string) (*sshClient, error) {
	privateKey, err := ssh.ParsePrivateKey([]byte(key))
	if err != nil {
		return nil, errors.Trace(err)
	}
	config := &ssh.ClientConfig{
		User: "ubuntu",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(privateKey),
		},
	}
	client, err := ssh.Dial("tcp", ipAddress+":22", config)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to dial to %s", ipAddress)
	}
	return &sshClient{
		client: client,
	}, nil
}

func (c *sshClient) GetNicNameByAddress(ipAddress string) (string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", errors.Annotatef(err, "failed to create ssh session")
	}
	defer session.Close()
	res, err := session.Output("ip addr show | grep '172.16.16.80' | awk -F' ' '{print $7}'")
	if err != nil {
		return "", errors.Trace(err)
	}
	return string(res), nil
}

func (c *sshClient) ChangePorts(nicName, target string, ports []network.PortRange) error {
	session, err := c.client.NewSession()
	if err != nil {
		return errors.Annotatef(err, "failed to create ssh session")
	}
	defer session.Close()
	cmd := ""
	for _, port := range ports {
		cmd += fmt.Sprintf("sudo iptables -i %s -A INPUT -p %s --match multiport --dports %s:%s -j %s\n", nicName, port.Protocol, port.FromPort, port.ToPort, target)
	}
	cmd += "sudo service iptables save"
	err = session.Run(cmd)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
