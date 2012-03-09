// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"errors"
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/charm"
	"strconv"
	"strings"
)

// RetryMode is the type of allowed values for the retry
// variable inside 
type RetryMode int

const (
	ResolvedNoValue    RetryMode = -1
	ResolvedRetryHooks RetryMode = 1000
	ResolvedNoHooks    RetryMode = 1001
)

// Port identifies a network port for a particular protocol.
type Port struct {
	Port  int
	Proto string
}

// Unit represents the state of a service unit.
type Unit struct {
	st          *State
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

// PublicAddress returns the public address of the unit.
func (u *Unit) PublicAddress() (string, error) {
	cn, err := readConfigNode(u.st.zk, u.zkPath())
	if err != nil {
		return "", err
	}
	if address, ok := cn.Get("public-address"); ok {
		return address.(string), nil
	}
	return "", errors.New("unit has no public address")
}

// SetPublicAddress sets the public address of the unit.
func (u *Unit) SetPublicAddress(address string) error {
	cn, err := readConfigNode(u.st.zk, u.zkPath())
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

// PrivateAddress returns the private address of the unit.
func (u *Unit) PrivateAddress() (string, error) {
	cn, err := readConfigNode(u.st.zk, u.zkPath())
	if err != nil {
		return "", err
	}
	if address, ok := cn.Get("private-address"); ok {
		return address.(string), nil
	}
	return "", errors.New("unit has no private address")
}

// SetPrivateAddress sets the private address of the unit.
func (u *Unit) SetPrivateAddress(address string) error {
	cn, err := readConfigNode(u.st.zk, u.zkPath())
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
	cn, err := readConfigNode(u.st.zk, u.zkPath())
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
	cn, err := readConfigNode(u.st.zk, u.zkPath())
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

// AssignedMachineId returns the id of the assigned machine.
func (u *Unit) AssignedMachineId() (int, error) {
	topology, err := readTopology(u.st.zk)
	if err != nil {
		return 0, err
	}
	if !topology.HasService(u.serviceKey) || !topology.HasUnit(u.serviceKey, u.key) {
		return 0, stateChanged
	}
	machineKey, err := topology.UnitMachineKey(u.serviceKey, u.key)
	if err != nil {
		return 0, err
	}
	return machineId(machineKey), nil
}

// AssignToMachine assigns this unit to a given machine.
func (u *Unit) AssignToMachine(machine *Machine) error {
	assignUnit := func(t *topology) error {
		if !t.HasService(u.serviceKey) || !t.HasUnit(u.serviceKey, u.key) {
			return stateChanged
		}
		machineKey, err := t.UnitMachineKey(u.serviceKey, u.key)
		if err == unitNotAssigned {
			return t.AssignUnitToMachine(u.serviceKey, u.key, machine.key)
		} else if err != nil {
			return err
		} else if machineKey == machine.key {
			// Everything is fine, it's already assigned.
			return nil
		}
		return fmt.Errorf("unit %q already assigned to machine %d", u.Name(), machineId(machineKey))
	}
	return retryTopologyChange(u.st.zk, assignUnit)
}

// AssignToUnusedMachine assigns u to a machine without other units.
// If there are no unused machines besides machine 0, an error is returned.
func (u *Unit) AssignToUnusedMachine() (*Machine, error) {
	machineKey := ""
	assignUnusedUnit := func(t *topology) error {
		if !t.HasService(u.serviceKey) || !t.HasUnit(u.serviceKey, u.key) {
			return stateChanged
		}
		for _, machineKey = range t.MachineKeys() {
			if machineId(machineKey) != 0 {
				hasUnits, err := t.MachineHasUnits(machineKey)
				if err != nil {
					return err
				}
				if !hasUnits {
					break
				}
			}
			// Reset machine key.
			machineKey = ""
		}
		if machineKey == "" {
			return errors.New("no unused machine found")
		}
		if err := t.AssignUnitToMachine(u.serviceKey, u.key, machineKey); err != nil {
			return err
		}
		return nil
	}
	if err := retryTopologyChange(u.st.zk, assignUnusedUnit); err != nil {
		return nil, err
	}
	return &Machine{u.st, machineKey}, nil
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
	return retryTopologyChange(u.st.zk, unassignUnit)
}

// NeedsUpgrade returns whether the unit needs an upgrade.
func (u *Unit) NeedsUpgrade() (bool, error) {
	stat, err := u.st.zk.Exists(u.zkNeedsUpgradePath())
	if err != nil {
		return false, err
	}
	return stat != nil, nil
}

// SetNeedsUpgrade informs the unit that it should perform an upgrde.
func (u *Unit) SetNeedsUpgrade() error {
	_, err := u.st.zk.Create(u.zkNeedsUpgradePath(), "", 0, zkPermAll)
	if err == zookeeper.ZNODEEXISTS {
		// Node already exists, so same state.
		return nil
	}
	return err
}

// ClearNeedsUpgrade resets the upgrade notification. Itis typically
// done by the unit agent before beginning the upgrade.
func (u *Unit) ClearNeedsUpgrade() error {
	err := u.st.zk.Delete(u.zkNeedsUpgradePath(), -1)
	if err == zookeeper.ZNONODE {
		// Node doesn't exist, so same state.
		return nil
	}
	return err
}

// validRetryMode ensures that only valid values for the
// resolved retry are used.
func (u *Unit) validRetryMode(retry RetryMode) error {
	if retry != ResolvedRetryHooks && retry != ResolvedNoHooks {
		return fmt.Errorf("invalid retry mode: %d", retry)
	}
	return nil
}

// Resolved returns the value of the resolved setting if any.
func (u *Unit) Resolved() (RetryMode, error) {
	yaml, _, err := u.st.zk.Get(u.zkResolvedPath())
	if err == zookeeper.ZNONODE {
		// Default value.
		return ResolvedNoValue, nil
	}
	if err != nil {
		return ResolvedNoValue, err
	}
	setting := make(map[string]RetryMode)
	if err = goyaml.Unmarshal([]byte(yaml), setting); err != nil {
		return ResolvedNoValue, err
	}
	retry := setting["retry"]
	if err := u.validRetryMode(retry); err != nil {
		return ResolvedNoValue, err
	}
	return retry, nil
}

// SetResolved marks the unit as in need of being resolved. The
// resolved setting is set by the command line to inform a unit
// to attempt a retry transition from an error state.
func (u *Unit) SetResolved(retry RetryMode) error {
	if err := u.validRetryMode(retry); err != nil {
		return err
	}
	setting := map[string]RetryMode{"retry": retry}
	yaml, err := goyaml.Marshal(setting)
	if err != nil {
		return err
	}
	_, err = u.st.zk.Create(u.zkResolvedPath(), string(yaml), 0, zkPermAll)
	if err == zookeeper.ZNODEEXISTS {
		return fmt.Errorf("unit %q resolved already enabled", u.Name())
	}
	return err
}

// ClearResolved removes any resolved setting on the unit.
func (u *Unit) ClearResolved() error {
	err := u.st.zk.Delete(u.zkResolvedPath(), -1)
	if err == zookeeper.ZNONODE {
		// Node doesn't exist, so same state.
		return nil
	}
	return err
}

// OpenPort sets the policy that port (using proto) should be opened.
func (u *Unit) OpenPort(port int, proto string) error {
	openPort := func(yaml string, stat *zookeeper.Stat) (string, error) {
		data := map[string][]Port{}
		if yaml != "" {
			if err := goyaml.Unmarshal([]byte(yaml), data); err != nil {
				return "", err
			}
		}
		open := data["open"]
		openPort := Port{port, proto}
		found := false
		for _, p := range open {
			if p == openPort {
				found = true
				break
			}
		}
		if !found {
			data["open"] = append(open, openPort)
		}
		changedYaml, err := goyaml.Marshal(data)
		if err != nil {
			return "", err
		}
		return string(changedYaml), nil
	}
	return u.st.zk.RetryChange(u.zkPortsPath(), 0, zkPermAll, openPort)
}

// ClosePort sets the policy that port (using proto) should be closed.
func (u *Unit) ClosePort(port int, proto string) error {
	closePort := func(yaml string, stat *zookeeper.Stat) (string, error) {
		data := map[string][]Port{}
		if yaml != "" {
			if err := goyaml.Unmarshal([]byte(yaml), data); err != nil {
				return "", err
			}
		}
		open := data["open"]
		portProto := Port{port, proto}
		changedOpen := []Port{}
		for _, pp := range open {
			if pp != portProto {
				changedOpen = append(changedOpen, pp)
			}
		}
		data["open"] = changedOpen
		changedYaml, err := goyaml.Marshal(data)
		if err != nil {
			return "", err
		}
		return string(changedYaml), nil
	}
	return u.st.zk.RetryChange(u.zkPortsPath(), 0, zkPermAll, closePort)
}

// OpenPorts returns a slice containing the open ports of the unit.
func (u *Unit) OpenPorts() ([]Port, error) {
	yaml, _, err := u.st.zk.Get(u.zkPortsPath())
	if err == zookeeper.ZNONODE {
		// Default value.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var data map[string][]Port
	if err = goyaml.Unmarshal([]byte(yaml), &data); err != nil {
		return nil, err
	}
	return data["open"], nil
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

// zkNeedsUpgradePath returns the ZooKeeper path for the upgrade flag.
func (u *Unit) zkNeedsUpgradePath() string {
	return fmt.Sprintf("/units/%s/upgrade", u.key)
}

// zkResolvedPath returns the ZooKeeper path for the mark to resolve a unit.
func (u *Unit) zkResolvedPath() string {
	return fmt.Sprintf("/units/%s/resolved", u.key)
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
