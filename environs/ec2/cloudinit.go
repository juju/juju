package juju

import (
	"fmt"
	"launchpad.net/juju/go/cloudinit"
	"os/exec"
	"strings"
)

// CloudConfig represents initialization information for a new juju machine.
type CloudConfig struct {
	// The new machine will run a zookeeper instance.
	Zookeeper bool

	// InstanceIdAccessor holds bash code that evaluates to the current instance id.
	InstanceIdAccessor string

	// AdminSecret holds a secret that will be used to authenticate to zookeeper.
	AdminSecret string

	// ZookeeperHosts lists the names of hosts running zookeeper.
	// Unless the new machine is running zookeeper (Zookeeper is set),
	// there must be at least one host name supplied.
	ZookeeperHosts []string

	// MachineId identifies the new machine. It must be
	// non-empty.
	MachineId string
}
