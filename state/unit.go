package state

import (
	"errors"
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state/presence"
	"strconv"
	"strings"
	"time"
)

// ResolvedMode describes the way state transition errors 
// are resolved. 
type ResolvedMode int

const (
	ResolvedNone       ResolvedMode = 0
	ResolvedRetryHooks ResolvedMode = 1000
	ResolvedNoHooks    ResolvedMode = 1001
)

// AssignmentPolicy controls what machine a unit will be assigned to.
type AssignmentPolicy string

const (
	// AssignLocal indicates that all service units should be assigned to machine 0.
	AssignLocal AssignmentPolicy = "local"
	// AssignUnused indicates that every service unit should be assigned to a
	// dedicated machine, and that new machines should be launched if required.
	AssignUnused AssignmentPolicy = "unused"
)

// NeedsUpgrade describes if a unit needs an
// upgrade and if this is forced.
type NeedsUpgrade struct {
	Upgrade bool
	Force   bool
}

// needsUpgradeNode represents the content of the
// upgrade node of a unit.
type needsUpgradeNode struct {
	Force bool
}

// agentPingerPeriod defines the period of pinging the
// ZooKeeper to signal that a unit agent is alive. It's
// also used by machine.
var (
	agentPingerPeriod = 1 * time.Second
)

// Port identifies a network port number for a particular protocol.
type Port struct {
	Protocol string `yaml:"proto"`
	Number   int    `yaml:"port"`
}

// openPortsNode represents the content of the
// ports node of a unit.
type openPortsNode struct {
	Open []Port
}

// Unit represents the state of a service unit.
type Unit struct {
	st           *State
	key          string
	serviceName  string
	principalKey string
}

// ServiceName returns the service name.
func (u *Unit) ServiceName() string {
	return u.serviceName
}

// Name returns the unit name.
func (u *Unit) Name() string {
	return fmt.Sprintf("%s/%d", u.serviceName, keySeq(u.key))
}

// String returns the unit as string.
func (u *Unit) String() string {
	return u.Name()
}

// makeUnitKey returns a unit key made up from the service key
// and the unit sequence number within the service.
func makeUnitKey(serviceKey string, unitSeq int) string {
	if !strings.HasPrefix(serviceKey, "service-") {
		panic(fmt.Errorf("invalid service key %q", serviceKey))
	}
	return fmt.Sprintf("unit-%s-%010d", serviceKey[len("service-"):], unitSeq)
}

func serviceKeyForUnitKey(unitKey string) (string, error) {
	if !strings.HasPrefix(unitKey, "unit-") {
		return "", fmt.Errorf("invalid unit key %q", unitKey)
	}
	k := unitKey[len("unit-"):]
	i := strings.Index(k, "-")
	if i <= 0 {
		return "", fmt.Errorf("invalid unit key %q", unitKey)
	}
	return "service-" + k[0:i], nil
}

func (st *State) unitFromKey(t *topology, unitKey string) (*Unit, error) {
	tsvc, tunit, err := t.serviceAndUnit(unitKey)
	if err != nil {
		return nil, err
	}
	principalKey := tunit.Principal
	if principalKey == "" {
		principalKey = unitKey
	}
	return &Unit{
		st:           st,
		key:          unitKey,
		serviceName:  tsvc.Name,
		principalKey: principalKey,
	}, nil
}

// PublicAddress returns the public address of the unit.
func (u *Unit) PublicAddress() (string, error) {
	cn, err := readConfigNode(u.st.zk, u.zkPath())
	if err != nil {
		return "", fmt.Errorf("can't get public address of unit %q: %v", u, err)
	}
	if address, ok := cn.Get("public-address"); ok {
		return address.(string), nil
	}
	return "", errors.New("unit has no public address")
}

// SetPublicAddress sets the public address of the unit.
func (u *Unit) SetPublicAddress(address string) (err error) {
	defer errorContextf(&err, "can't set public address of unit %q to %q", u, address)
	cn, err := readConfigNode(u.st.zk, u.zkPath())
	if err != nil {
		return err
	}
	cn.Set("public-address", address)
	_, err = cn.Write()
	return err
}

// PrivateAddress returns the private address of the unit.
func (u *Unit) PrivateAddress() (string, error) {
	cn, err := readConfigNode(u.st.zk, u.zkPath())
	if err != nil {
		return "", fmt.Errorf("can't get private address of unit %q: %v", u, err)
	}
	if address, ok := cn.Get("private-address"); ok {
		return address.(string), nil
	}
	return "", errors.New("unit has no private address")
}

// SetPrivateAddress sets the private address of the unit.
func (u *Unit) SetPrivateAddress(address string) (err error) {
	defer errorContextf(&err, "can't set private address of unit %q", u)
	cn, err := readConfigNode(u.st.zk, u.zkPath())
	if err != nil {
		return err
	}
	cn.Set("private-address", address)
	_, err = cn.Write()
	return err
}

// CharmURL returns the charm URL this unit is supposed
// to use.
func (u *Unit) CharmURL() (url *charm.URL, err error) {
	defer errorContextf(&err, "can't get charm URL of unit %q", u)
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
func (u *Unit) SetCharmURL(url *charm.URL) (err error) {
	defer errorContextf(&err, "can't set charm URL of unit %q to %q", u, url)
	cn, err := readConfigNode(u.st.zk, u.zkPath())
	if err != nil {
		return err
	}
	cn.Set("charm", url.String())
	_, err = cn.Write()
	return err
}

// IsPrincipal returns whether the unit is deployed in its own container,
// and can therefore have subordinate services deployed alongside it.
func (u *Unit) IsPrincipal() bool {
	return u.key == u.principalKey
}

// AssignedMachineId returns the id of the assigned machine.
func (u *Unit) AssignedMachineId() (id int, err error) {
	defer errorContextf(&err, "can't get machine id of unit %q", u)
	topology, err := readTopology(u.st.zk)
	if err != nil {
		return 0, err
	}
	if !topology.HasUnit(u.key) {
		return 0, stateChanged
	}
	machineKey, err := topology.UnitMachineKey(u.key)
	if err != nil {
		return 0, err
	}
	return keySeq(machineKey), nil
}

// AssignToMachine assigns this unit to a given machine.
func (u *Unit) AssignToMachine(machine *Machine) (err error) {
	defer errorContextf(&err, "can't assign unit %q to machine %s", u, machine)
	assignUnit := func(t *topology) error {
		if !t.HasUnit(u.key) {
			return stateChanged
		}
		machineKey, err := t.UnitMachineKey(u.key)
		if err == unitNotAssigned {
			return t.AssignUnitToMachine(u.key, machine.key)
		} else if err != nil {
			return err
		} else if machineKey == machine.key {
			// Everything is fine, it's already assigned.
			return nil
		}
		return fmt.Errorf("unit already assigned to machine %d", keySeq(machineKey))
	}
	return retryTopologyChange(u.st.zk, assignUnit)
}

var noUnusedMachines = errors.New("all machines in use")

// AssignToUnusedMachine assigns u to a machine without other units.
// If there are no unused machines besides machine 0, an error is returned.
func (u *Unit) AssignToUnusedMachine() (m *Machine, err error) {
	machineKey := ""
	assignUnusedUnit := func(t *topology) error {
		if !t.HasUnit(u.key) {
			return stateChanged
		}
		for _, machineKey = range t.MachineKeys() {
			if keySeq(machineKey) != 0 {
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
			return noUnusedMachines
		}
		if err := t.AssignUnitToMachine(u.key, machineKey); err != nil {
			return err
		}
		return nil
	}
	if err := retryTopologyChange(u.st.zk, assignUnusedUnit); err != nil {
		if err == noUnusedMachines {
			return nil, err
		}
		return nil, fmt.Errorf("can't assign unit %q to unused machine: %v", u, err)
	}
	return &Machine{u.st, machineKey}, nil
}

// UnassignFromMachine removes the assignment between this unit and
// the machine it's assigned to.
func (u *Unit) UnassignFromMachine() (err error) {
	defer errorContextf(&err, "can't unassign unit %q from machine", u.Name())
	unassignUnit := func(t *topology) error {
		if !t.HasUnit(u.key) {
			return stateChanged
		}
		// If for whatever reason it's already not assigned to a
		// machine, ignore it and move forward so that we don't
		// have to deal with conflicts.
		key, err := t.UnitMachineKey(u.key)
		if err == nil && key != "" {
			t.UnassignUnitFromMachine(u.key)
		}
		return nil
	}
	return retryTopologyChange(u.st.zk, unassignUnit)
}

// NeedsUpgrade returns whether the unit needs an upgrade 
// and if it does, if this is forced.
func (u *Unit) NeedsUpgrade() (needsUpgrade *NeedsUpgrade, err error) {
	defer errorContextf(&err, "can't check if unit %q needs an upgrade", u.Name())
	yaml, _, err := u.st.zk.Get(u.zkNeedsUpgradePath())
	if zookeeper.IsError(err, zookeeper.ZNONODE) {
		return &NeedsUpgrade{}, nil
	}
	if err != nil {
		return nil, err
	}
	var setting needsUpgradeNode
	if err = goyaml.Unmarshal([]byte(yaml), &setting); err != nil {
		return nil, err
	}
	return &NeedsUpgrade{true, setting.Force}, nil
}

// SetNeedsUpgrade informs the unit that it should perform 
// a regular or forced upgrade.
func (u *Unit) SetNeedsUpgrade(force bool) (err error) {
	defer errorContextf(&err, "can't inform unit %q about upgrade", u.Name())
	setNeedsUpgrade := func(oldYaml string, stat *zookeeper.Stat) (string, error) {
		var setting needsUpgradeNode
		if oldYaml == "" {
			setting.Force = force
			newYaml, err := goyaml.Marshal(setting)
			if err != nil {
				return "", err
			}
			return string(newYaml), nil
		}
		if err := goyaml.Unmarshal([]byte(oldYaml), &setting); err != nil {
			return "", err
		}
		if setting.Force != force {
			return "", fmt.Errorf("upgrade already enabled")
		}
		return oldYaml, nil
	}
	return u.st.zk.RetryChange(u.zkNeedsUpgradePath(), 0, zkPermAll, setNeedsUpgrade)
}

// ClearNeedsUpgrade resets the upgrade notification. It is typically
// done by the unit agent before beginning the upgrade.
func (u *Unit) ClearNeedsUpgrade() (err error) {
	defer errorContextf(&err, "upgrade notification for unit %q can't be reset", u.Name())
	err = u.st.zk.Delete(u.zkNeedsUpgradePath(), -1)
	if zookeeper.IsError(err, zookeeper.ZNONODE) {
		// Node doesn't exist, so same state.
		return nil
	}
	return err
}

// WatchNeedsUpgrade creates a watcher for the upgrade notification
// of the unit. See SetNeedsUpgrade and ClearNeedsUpgrade for details.
func (u *Unit) WatchNeedsUpgrade() *NeedsUpgradeWatcher {
	return newNeedsUpgradeWatcher(u.st, u.zkNeedsUpgradePath())
}

// Resolved returns the resolved mode for the unit.
func (u *Unit) Resolved() (mode ResolvedMode, err error) {
	defer errorContextf(&err, "can't get resolved mode for unit %q", u)
	yaml, _, err := u.st.zk.Get(u.zkResolvedPath())
	if zookeeper.IsError(err, zookeeper.ZNONODE) {
		// Default value.
		return ResolvedNone, nil
	}
	if err != nil {
		return ResolvedNone, err
	}
	setting := &struct{ Retry ResolvedMode }{}
	if err = goyaml.Unmarshal([]byte(yaml), setting); err != nil {
		return ResolvedNone, err
	}
	mode = setting.Retry
	if err := validResolvedMode(mode, false); err != nil {
		return ResolvedNone, err
	}
	return mode, nil
}

// SetResolved marks the unit as having had any previous state
// transition problems resolved, and informs the unit that it may
// attempt to reestablish normal workflow.
// The resolved mode parameter informs whether to attempt to 
// reexecute previous failed hooks or to continue as if they had 
// succeeded before.
func (u *Unit) SetResolved(mode ResolvedMode) (err error) {
	defer errorContextf(&err, "can't set resolved mode for unit %q", u)
	if err := validResolvedMode(mode, false); err != nil {
		return err
	}
	setting := &struct{ Retry ResolvedMode }{mode}
	yaml, err := goyaml.Marshal(setting)
	if err != nil {
		return err
	}
	_, err = u.st.zk.Create(u.zkResolvedPath(), string(yaml), 0, zkPermAll)
	if zookeeper.IsError(err, zookeeper.ZNODEEXISTS) {
		return fmt.Errorf("flag already set")
	}
	return err
}

// ClearResolved removes any resolved setting on the unit.
func (u *Unit) ClearResolved() (err error) {
	defer errorContextf(&err, "resolved mode for unit %q can't be cleared", u)
	err = u.st.zk.Delete(u.zkResolvedPath(), -1)
	if zookeeper.IsError(err, zookeeper.ZNONODE) {
		// Node doesn't exist, so same state.
		return nil
	}
	return err
}

// WatchResolved returns a watcher that fires when the unit 
// is marked as having had its problems resolved. See 
// SetResolved for details.
func (u *Unit) WatchResolved() *ResolvedWatcher {
	return newResolvedWatcher(u.st, u.zkResolvedPath())
}

// OpenPort sets the policy of the port with protocol and number to be opened.
func (u *Unit) OpenPort(protocol string, number int) (err error) {
	defer errorContextf(&err, "can't open port %s:%d for unit %q", protocol, number, u)
	openPort := func(oldYaml string, stat *zookeeper.Stat) (string, error) {
		var ports openPortsNode
		if oldYaml != "" {
			if err := goyaml.Unmarshal([]byte(oldYaml), &ports); err != nil {
				return "", err
			}
		}
		portToOpen := Port{protocol, number}
		found := false
		for _, openPort := range ports.Open {
			if openPort == portToOpen {
				found = true
				break
			}
		}
		if !found {
			ports.Open = append(ports.Open, portToOpen)
		}
		newYaml, err := goyaml.Marshal(ports)
		if err != nil {
			return "", err
		}
		return string(newYaml), nil
	}
	return u.st.zk.RetryChange(u.zkPortsPath(), 0, zkPermAll, openPort)
}

// ClosePort sets the policy of the port with protocol and number to be closed.
func (u *Unit) ClosePort(protocol string, number int) (err error) {
	defer errorContextf(&err, "can't close port %s:%d for unit %q", protocol, number, u)
	closePort := func(oldYaml string, stat *zookeeper.Stat) (string, error) {
		var ports openPortsNode
		if oldYaml != "" {
			if err := goyaml.Unmarshal([]byte(oldYaml), &ports); err != nil {
				return "", err
			}
		}
		portToClose := Port{protocol, number}
		newOpenPorts := []Port{}
		for _, oldOpenPort := range ports.Open {
			if oldOpenPort != portToClose {
				newOpenPorts = append(newOpenPorts, oldOpenPort)
			}
		}
		ports.Open = newOpenPorts
		newYaml, err := goyaml.Marshal(ports)
		if err != nil {
			return "", err
		}
		return string(newYaml), nil
	}
	return u.st.zk.RetryChange(u.zkPortsPath(), 0, zkPermAll, closePort)
}

// OpenPorts returns a slice containing the open ports of the unit.
func (u *Unit) OpenPorts() (openPorts []Port, err error) {
	defer errorContextf(&err, "can't get open ports of unit %q", u)
	yaml, _, err := u.st.zk.Get(u.zkPortsPath())
	if zookeeper.IsError(err, zookeeper.ZNONODE) {
		// Default value.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ports openPortsNode
	if err = goyaml.Unmarshal([]byte(yaml), &ports); err != nil {
		return nil, err
	}
	return ports.Open, nil
}

// WatchResolved returns a watcher that fires when the
// list of open ports of the unit is changed.
// See OpenPorts for details.
func (u *Unit) WatchPorts() *PortsWatcher {
	return newPortsWatcher(u.st, u.zkPortsPath())
}

// AgentAlive returns whether the respective remote agent is alive.
func (u *Unit) AgentAlive() (bool, error) {
	return presence.Alive(u.st.zk, u.zkAgentPath())
}

// WaitAgentAlive blocks until the respective agent is alive.
func (u *Unit) WaitAgentAlive(timeout time.Duration) error {
	err := presence.WaitAlive(u.st.zk, u.zkAgentPath(), timeout)
	if err != nil {
		return fmt.Errorf("waiting for agent of unit %q: %v", u, err)
	}
	return nil
}

// SetAgentAlive signals that the agent for unit u is alive
// by starting a pinger on its presence node. It returns the
// started pinger.
func (u *Unit) SetAgentAlive() (*presence.Pinger, error) {
	return presence.StartPinger(u.st.zk, u.zkAgentPath(), agentPingerPeriod)
}

// zkPath returns the ZooKeeper base path for the unit.
func (u *Unit) zkPath() string {
	skey, err := serviceKeyForUnitKey(u.key)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("/services/%s/units/%s", skey, u.key)
}

// zkPortsPath returns the ZooKeeper path for the open ports.
func (u *Unit) zkPortsPath() string {
	return u.zkPath() + "/ports"
}

// zkAgentPath returns the ZooKeeper path for the unit agent.
func (u *Unit) zkAgentPath() string {
	return u.zkPath() + "/agent"
}

// zkNeedsUpgradePath returns the ZooKeeper path for the upgrade flag.
func (u *Unit) zkNeedsUpgradePath() string {
	return u.zkPath() + "/upgrade"
}

// zkResolvedPath returns the ZooKeeper path for the mark to resolve a unit.
func (u *Unit) zkResolvedPath() string {
	return u.zkPath() + "/resolved"
}

// parseUnitName parses a unit name like "wordpress/0" into
// its service name and sequence number parts.
func parseUnitName(name string) (serviceName string, serviceSeq int, err error) {
	parts := strings.Split(name, "/")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("%q is not a valid unit name", name)
	}
	seq, err := strconv.ParseInt(parts[1], 10, 0)
	if err != nil {
		return "", 0, err
	}
	return parts[0], int(seq), nil
}

// parseResolvedMode returns the resolved mode serialized
// in yaml if it is valid, or an error otherwise.
func parseResolvedMode(yaml string) (ResolvedMode, error) {
	var setting struct {
		Retry ResolvedMode
	}
	if err := goyaml.Unmarshal([]byte(yaml), &setting); err != nil {
		return ResolvedNone, err
	}
	mode := setting.Retry
	if err := validResolvedMode(mode, true); err != nil {
		return ResolvedNone, err
	}
	return mode, nil
}

// validResolvedMode returns an error if the provided
// mode isn't valid. ResolvedNone is only considered a
// valid mode if acceptNone is true.
func validResolvedMode(mode ResolvedMode, acceptNone bool) error {
	if acceptNone && mode == ResolvedNone {
		return nil
	}
	if mode != ResolvedRetryHooks && mode != ResolvedNoHooks {
		return fmt.Errorf("invalid error resolution mode: %d", mode)
	}
	return nil
}
