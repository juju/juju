// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"errors"
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/charm"
	"strconv"
	"strings"
)

// Unit represents the state of a service unit.
type Unit struct {
	zk          *zookeeper.Conn
	key         string
	serviceKey  string
	serviceName string
	sequenceNo  int
}

// ServiceName returns the service name.
func (u *Unit) ServiceName() string {
	return u.serviceName
}

// Name returns the unit name.
func (u *Unit) Name() string {
	return fmt.Sprintf("%s/%d", u.serviceName, u.sequenceNo)
}

// PublicAddress returns the public address of the unit. If the unit 
// is unassigned, or its unit agent hasn't started this value it may be empty.
func (u *Unit) PublicAddress() (string, error) {
	cn, err := readConfigNode(u.zk, u.zkPath())
	if err != nil {
		return "", err
	}
	if address, ok := cn.Get("public-address"); ok {
		return address.(string), nil
	}
	return "", nil
}

// SetPublicAddress sets the public address of the unit.
func (u *Unit) SetPublicAddress(address string) error {
	cn, err := readConfigNode(u.zk, u.zkPath())
	if err != nil {
		return err
	}
	cn.Set("public-address", address)	
	_, err = cn.Write()
	if err != nil {
		return err
	}
	return nil
}

// PrivateAddress returns the private address of the unit. If the unit 
// is unassigned, or its unit agent hasn't started this value it may be empty.
func (u *Unit) PrivateAddress() (string, error) {
	cn, err := readConfigNode(u.zk, u.zkPath())
	if err != nil {
		return "", err
	}
	if address, ok := cn.Get("private-address"); ok {
		return address.(string), nil
	}
	return "", nil
}

// SetPrivateAddress sets the private address of the unit.
func (u *Unit) SetPrivateAddress(address string) error {
	cn, err := readConfigNode(u.zk, u.zkPath())
	if err != nil {
		return err
	}
	cn.Set("private-address", address)	
	_, err = cn.Write()
	if err != nil {
		return err
	}
	return nil
}

// CharmURL returns the charm URL this unit is supposed
// to use.
func (u *Unit) CharmURL() (url *charm.URL, err error) {
	cn, err := readConfigNode(u.zk, u.zkPath())
	if err != nil {
		return nil, err
	}
	if id, ok := cn.Get("charm"); ok {
		url, err = charm.ParseURL(id.(string))
		if err != nil {
			return nil, err
		}
		return url, nil
	}
	return nil, errors.New("unit has no charm URL")
}

// SetCharmURL changes the charm URL for the unit.
func (u *Unit) SetCharmURL(url *charm.URL) error {
	cn, err := readConfigNode(u.zk, u.zkPath())
	if err != nil {
		return err
	}
	cn.Set("charm", url.String())
	_, err = cn.Write()
	if err != nil {
		return err
	}
	return nil
}

// AssignedMachineKey returns the key of the assigned machine.
func (u *Unit) AssignedMachineKey() (string, error) {
	topology, err := readTopology(u.zk)
	if err != nil {
		return "", err
	}
	if !topology.HasService(u.serviceKey) || !topology.HasUnit(u.serviceKey, u.key) {
		return "", stateChanged
	}
	machineKey, err := topology.UnitMachineKey(u.serviceKey, u.key)
	if err != nil {
		return "", err
	}
	if machineKey != "" {
		machineKey = publicMachineKey(machineKey)
	}
	return machineKey, nil
}

// AssignToMachine assigns this unit to a given machine.
func (u *Unit) AssignToMachine(machine *Machine) error {
	assignUnit := func(t *topology) error {
		if !t.HasService(u.serviceKey) || !t.HasUnit(u.serviceKey, u.key) {
			return stateChanged
		}
		machineKey, err := t.UnitMachineKey(u.serviceKey, u.key)
		if err != nil {
			return err
		}
		if machineKey == "" {
			if err = t.AssignUnitToMachine(u.serviceKey, u.key, machine.key); err != nil {
				return err
			}
			return nil
		} else if machineKey == machine.key {
			// Everything is fine, it's already assigned.
			return nil
		}
		return fmt.Errorf("unit %q already assigned to a machine", u.Name())
	}
	return retryTopologyChange(u.zk, assignUnit)
}

// AssignToUnusedMachine assigns this unit to an unused machine (if available). 
// Machine 0 is special, so it won't be reused. If there are no available
// machines an error will be returned. This usually should lead to the
// creation and assigning of a new machine.
func (u *Unit) AssignToUnusedMachine() (*Machine, error) {
	unusedMachineKeyWrapper := ""
	assignUnusedUnit := func(t *topology) error {
		if !t.HasService(u.serviceKey) || !t.HasUnit(u.serviceKey, u.key) {
			return stateChanged
		}
		// We cannot reuse the "root" machine (used by the
            	// provisioning agent), but the topology metadata does not
            	// properly reflect its allocation.  In the future, once it
            	// is managed like any other service, this special case can
            	// be removed.
            	rootMachine := fmt.Sprintf("machine-%010d", 0)
            	unusedMachineKeys := []string{}
            	for _, machineKey := range t.MachineKeys() {
            		if machineKey != rootMachine {
            			ok, err := t.MachineHasUnits(machineKey)
            			if err != nil {
            				return err
            			}
            			if !ok {
            				unusedMachineKeys = append(unusedMachineKeys, machineKey)
            			}
            		}
            	}
            	if len(unusedMachineKeys) == 0 {
            		return errors.New("no unused machine found")
            	}
            	unusedMachineKey := unusedMachineKeys[0]
            	if err := t.AssignUnitToMachine(u.serviceKey, u.key, unusedMachineKey); err != nil {
            		return err
            	}
            	unusedMachineKeyWrapper = unusedMachineKey
            	return nil
	}
	if err := retryTopologyChange(u.zk, assignUnusedUnit); err != nil {
		return nil, err
	}
	return &Machine{u.zk, unusedMachineKeyWrapper}, nil
}

// UnassignFromMachine removes the assignment between this unit and
// the machine it's assigned to.
func (u *Unit) UnassignFromMachine() error {
	unassignUnit := func(t *topology) error {
		if !t.HasService(u.serviceKey) || !t.HasUnit(u.serviceKey, u.key) {
			return stateChanged
		}
		// If for whatever reason it's already not assigned to a
		// machine, ignore it and move forward so that we don't
		// have to deal with conflicts.
		key, err := t.UnitMachineKey(u.serviceKey, u.key)
		if err == nil && key != "" {
			t.UnassignUnitFromMachine(u.serviceKey, u.key)
		}
		return nil
	}
	return retryTopologyChange(u.zk, unassignUnit)
}

// zkKey returns the ZooKeeper key of the unit.
func (u *Unit) zkKey() string {
	return u.key
}

// zkPath returns the ZooKeeper base path for the unit.
func (u *Unit) zkPath() string {
	return fmt.Sprintf("/units/%s", u.key)
}

// Name returns the name of the unit based on the service
// zkPortsPath returns the ZooKeeper path for the open ports.
func (u *Unit) zkPortsPath() string {
	return fmt.Sprintf("/units/%s/ports", u.key)
}

// zkAgentPath returns the ZooKeeper path for the unit agent.
func (u *Unit) zkAgentPath() string {
	return fmt.Sprintf("/units/%s/agent", u.key)
}

// parseUnitName parses a unit name like "wordpress/0" into
// its service name and sequence number parts.
func parseUnitName(name string) (serviceName string, seqNo int, err error) {
	parts := strings.Split(name, "/")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("%q is not a valid unit name", name)
	}
	sequenceNo, err := strconv.ParseInt(parts[1], 10, 0)
	if err != nil {
		return "", 0, err
	}
	return parts[0], int(sequenceNo), nil
}
