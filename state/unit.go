// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"strconv"
	"strings"
)

// Unit represents the state of a service
// and its subordinate parts.
type Unit struct {
	zk          *zookeeper.Conn
	id          string
	serviceId   string
	serviceName string
	sequenceNo  int
}

// ServiceName returns the service name.
func (u Unit) ServiceName() string {
	return u.serviceName
}

// Id returns the unit id.
func (u Unit) Id() string {
	return u.id
}

// Name returns the name of the unit based on the service
// name and the sequence number.
func (u Unit) Name() string {
	return fmt.Sprintf("%s/%d", u.serviceName, u.sequenceNo)
}

// zkPortsPath returns the ZooKeeper path for the open ports.
func (u Unit) zkPortsPath() string {
	return fmt.Sprintf("/units/%s/ports", u.id)
}

// zkAgentPath returns the ZooKeeper path for the unit agent.
func (u Unit) zkAgentPath() string {
	return fmt.Sprintf("/units/%s/agent", u.id)
}

// parseUnitName parses a unit name into its service name
// and sequence number. So a name like 'wordpress/0' will lead
// to 'wordpress' as service name and the int 0 as the
// sequence number.
func parseUnitName(name string) (string, int, error) {
	parts := strings.Split(name, "/")
	if len(parts) != 2 {
		return "", 0, newError("'%v' is no valid unit name", name)
	}
	sequenceNo, err := strconv.ParseInt(parts[1], 10, 0)
	if err != nil {
		return "", 0, err
	}
	return parts[0], int(sequenceNo), nil
}
