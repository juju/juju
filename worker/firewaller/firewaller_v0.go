// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/tomb"

	apifirewaller "github.com/juju/juju/api/firewaller"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
)

// FirewallerV0 watches the state for ports opened or closed on
// service units and reflects those changes onto the backing
// environment.
//
// TODO(dimitern) Remove this implementation and start using the one
// using APIV1, after there is an upgrade step in place to migrate
// unit ports to machine ports.
type FirewallerV0 struct {
	tomb            tomb.Tomb
	st              *apifirewaller.State
	environ         environs.Environ
	environWatcher  apiwatcher.NotifyWatcher
	machinesWatcher apiwatcher.StringsWatcher
	machineds       map[string]*machineDataV0
	unitsChange     chan *unitsChangeV0
	unitds          map[string]*unitDataV0
	portsChange     chan *portsChangeV0
	serviceds       map[string]*serviceDataV0
	exposedChange   chan *exposedChangeV0
	globalMode      bool
	globalPortRef   map[network.Port]int
}

// NewFirewallerV0 returns a new FirewallerV0.
func NewFirewallerV0(st *apifirewaller.State) (*FirewallerV0, error) {
	environWatcher, err := st.WatchForEnvironConfigChanges()
	if err != nil {
		return nil, err
	}
	machinesWatcher, err := st.WatchEnvironMachines()
	if err != nil {
		return nil, err
	}
	fw := &FirewallerV0{
		st:              st,
		environWatcher:  environWatcher,
		machinesWatcher: machinesWatcher,
		machineds:       make(map[string]*machineDataV0),
		unitsChange:     make(chan *unitsChangeV0),
		unitds:          make(map[string]*unitDataV0),
		portsChange:     make(chan *portsChangeV0),
		serviceds:       make(map[string]*serviceDataV0),
		exposedChange:   make(chan *exposedChangeV0),
	}
	go func() {
		defer fw.tomb.Done()
		fw.tomb.Kill(fw.loop())
	}()
	return fw, nil
}

func (fw *FirewallerV0) loop() error {
	defer fw.stopWatchers()

	var err error
	var reconciled bool

	fw.environ, err = worker.WaitForEnviron(fw.environWatcher, fw.st, fw.tomb.Dying())
	if err != nil {
		return err
	}
	if fw.environ.Config().FirewallMode() == config.FwGlobal {
		fw.globalMode = true
		fw.globalPortRef = make(map[network.Port]int)
	}
	for {
		select {
		case <-fw.tomb.Dying():
			return tomb.ErrDying
		case _, ok := <-fw.environWatcher.Changes():
			if !ok {
				return watcher.MustErr(fw.environWatcher)
			}
			config, err := fw.st.EnvironConfig()
			if err != nil {
				return err
			}
			if err := fw.environ.SetConfig(config); err != nil {
				logger.Errorf("loaded invalid environment configuration: %v", err)
			}
		case change, ok := <-fw.machinesWatcher.Changes():
			if !ok {
				return watcher.MustErr(fw.machinesWatcher)
			}
			for _, machineId := range change {
				fw.machineLifeChanged(names.NewMachineTag(machineId).String())
			}
			if !reconciled {
				reconciled = true
				var err error
				if fw.globalMode {
					err = fw.reconcileGlobal()
				} else {
					err = fw.reconcileInstances()
				}
				if err != nil {
					return err
				}
			}
		case change := <-fw.unitsChange:
			if err := fw.unitsChanged(change); err != nil {
				return err
			}
		case change := <-fw.portsChange:
			change.unitd.ports = change.ports
			if err := fw.flushUnits([]*unitDataV0{change.unitd}); err != nil {
				return errors.Annotate(err, "cannot change firewall ports")
			}
		case change := <-fw.exposedChange:
			change.serviced.exposed = change.exposed
			unitds := []*unitDataV0{}
			for _, unitd := range change.serviced.unitds {
				unitds = append(unitds, unitd)
			}
			if err := fw.flushUnits(unitds); err != nil {
				return errors.Annotate(err, "cannot change firewall ports")
			}
		}
	}
}

// startMachine creates a new data value for tracking details of the
// machine and starts watching the machine for units added or removed.
// TODO(dfc) should take a names.Tag
func (fw *FirewallerV0) startMachine(machineTag string) error {
	tag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return err
	}
	machined := &machineDataV0{
		fw:     fw,
		tag:    tag,
		unitds: make(map[string]*unitDataV0),
		ports:  make([]network.Port, 0),
	}
	m, err := machined.machine()
	if params.IsCodeNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Annotate(err, "cannot watch machine units")
	}
	unitw, err := m.WatchUnits()
	if err != nil {
		return err
	}
	select {
	case <-fw.tomb.Dying():
		stop("units watcher", unitw)
		return tomb.ErrDying
	case change, ok := <-unitw.Changes():
		if !ok {
			stop("units watcher", unitw)
			return watcher.MustErr(unitw)
		}
		fw.machineds[machineTag] = machined
		err = fw.unitsChanged(&unitsChangeV0{machined, change})
		if err != nil {
			stop("units watcher", unitw)
			delete(fw.machineds, machineTag)
			return errors.Annotatef(err, "cannot respond to units changes for %q", tag)
		}
	}
	go machined.watchLoop(unitw)
	return nil
}

// startUnit creates a new data value for tracking details of the unit
// and starts watching the unit for port changes. The provided
// machineTag must be the tag for the machine the unit was last
// observed to be assigned to.
func (fw *FirewallerV0) startUnit(unit *apifirewaller.Unit, machineTag string) error {
	service, err := unit.Service()
	if err != nil {
		return err
	}
	serviceName := service.Name()
	unitName := unit.Name()
	openedPorts, err := unit.OpenedPorts()
	if err != nil {
		return err
	}
	unitd := &unitDataV0{
		fw:    fw,
		unit:  unit,
		ports: openedPorts,
	}
	fw.unitds[unitName] = unitd

	unitd.machined = fw.machineds[machineTag]
	unitd.machined.unitds[unitName] = unitd
	if fw.serviceds[serviceName] == nil {
		err := fw.startService(service)
		if err != nil {
			delete(fw.unitds, unitName)
			return err
		}
	}
	unitd.serviced = fw.serviceds[serviceName]
	unitd.serviced.unitds[unitName] = unitd

	ports := make([]network.Port, len(unitd.ports))
	copy(ports, unitd.ports)

	go unitd.watchLoop(ports)
	return nil
}

// startService creates a new data value for tracking details of the
// service and starts watching the service for exposure changes.
func (fw *FirewallerV0) startService(service *apifirewaller.Service) error {
	exposed, err := service.IsExposed()
	if err != nil {
		return err
	}
	serviced := &serviceDataV0{
		fw:      fw,
		service: service,
		exposed: exposed,
		unitds:  make(map[string]*unitDataV0),
	}
	fw.serviceds[service.Name()] = serviced
	go serviced.watchLoop(serviced.exposed)
	return nil
}

// reconcileGlobal compares the initially started watcher for machines,
// units and services with the opened and closed ports globally and
// opens and closes the appropriate ports for the whole environment.
func (fw *FirewallerV0) reconcileGlobal() error {
	initialPortRanges, err := fw.environ.Ports()
	if err != nil {
		return err
	}
	initialPorts := network.PortRangesToPorts(initialPortRanges)
	collector := make(map[network.Port]bool)
	for _, unitd := range fw.unitds {
		if unitd.serviced.exposed {
			for _, port := range unitd.ports {
				collector[port] = true
			}
		}
	}
	wantedPorts := []network.Port{}
	for port := range collector {
		wantedPorts = append(wantedPorts, port)
	}
	// Check which ports to open or to close.
	toOpen := DiffPorts(wantedPorts, initialPorts)
	toClose := DiffPorts(initialPorts, wantedPorts)
	if len(toOpen) > 0 {
		logger.Infof("opening global ports %v", toOpen)
		if err := fw.environ.OpenPorts(network.PortsToPortRanges(toOpen)); err != nil {
			return err
		}
		network.SortPorts(toOpen)
	}
	if len(toClose) > 0 {
		logger.Infof("closing global ports %v", toClose)
		if err := fw.environ.ClosePorts(network.PortsToPortRanges(toClose)); err != nil {
			return err
		}
		network.SortPorts(toClose)
	}
	return nil
}

// reconcileInstances compares the initially started watcher for machines,
// units and services with the opened and closed ports of the instances and
// opens and closes the appropriate ports for each instance.
func (fw *FirewallerV0) reconcileInstances() error {
	for _, machined := range fw.machineds {
		m, err := machined.machine()
		if params.IsCodeNotFound(err) {
			if err := fw.forgetMachine(machined); err != nil {
				return err
			}
			continue
		} else if err != nil {
			return err
		}
		instanceId, err := m.InstanceId()
		if err != nil {
			return err
		}
		instances, err := fw.environ.Instances([]instance.Id{instanceId})
		if err == environs.ErrNoInstances {
			return nil
		} else if err != nil {
			return err
		}
		machineId := machined.tag.Id()
		initialPortRanges, err := instances[0].Ports(machineId)
		if err != nil {
			return err
		}
		initialPorts := network.PortRangesToPorts(initialPortRanges)
		// Check which ports to open or to close.
		toOpen := DiffPorts(machined.ports, initialPorts)
		toClose := DiffPorts(initialPorts, machined.ports)
		if len(toOpen) > 0 {
			logger.Infof("opening instance ports %v for %q",
				toOpen, machined.tag)
			if err := instances[0].OpenPorts(machineId, network.PortsToPortRanges(toOpen)); err != nil {
				// TODO(mue) Add local retry logic.
				return err
			}
			network.SortPorts(toOpen)
		}
		if len(toClose) > 0 {
			logger.Infof("closing instance ports %v for %q",
				toClose, machined.tag)
			if err := instances[0].ClosePorts(machineId, network.PortsToPortRanges(toClose)); err != nil {
				// TODO(mue) Add local retry logic.
				return err
			}
			network.SortPorts(toClose)
		}
	}
	return nil
}

// unitsChanged responds to changes to the assigned units.
func (fw *FirewallerV0) unitsChanged(change *unitsChangeV0) error {
	changed := []*unitDataV0{}
	for _, name := range change.units {
		unit, err := fw.st.Unit(names.NewUnitTag(name))
		if err != nil && !params.IsCodeNotFound(err) {
			return err
		}
		var machineTag names.Tag
		if unit != nil {
			machineTag, err = unit.AssignedMachine()
			if params.IsCodeNotFound(err) {
				continue
			} else if err != nil && !params.IsCodeNotAssigned(err) {
				return err
			}
		}
		if unitd, known := fw.unitds[name]; known {
			knownMachineTag := fw.unitds[name].machined.tag
			if unit == nil || unit.Life() == params.Dead || machineTag != knownMachineTag {
				fw.forgetUnit(unitd)
				changed = append(changed, unitd)
				logger.Debugf("stopped watching unit %s", name)
			}
			// TODO(dfc) fw.machineds should be map[names.Tag]
		} else if unit != nil && unit.Life() != params.Dead && fw.machineds[machineTag.String()] != nil {
			err = fw.startUnit(unit, machineTag.String())
			if err != nil {
				return err
			}
			changed = append(changed, fw.unitds[name])
			logger.Debugf("started watching unit %s", name)
		}
	}
	if err := fw.flushUnits(changed); err != nil {
		return errors.Annotate(err, "cannot change firewall ports")
	}
	return nil
}

// flushUnits opens and closes ports for the passed unit data.
func (fw *FirewallerV0) flushUnits(unitds []*unitDataV0) error {
	machineds := map[string]*machineDataV0{}
	for _, unitd := range unitds {
		machineds[unitd.machined.tag.String()] = unitd.machined
	}
	for _, machined := range machineds {
		if err := fw.flushMachine(machined); err != nil {
			return err
		}
	}
	return nil
}

// flushMachine opens and closes ports for the passed machine.
func (fw *FirewallerV0) flushMachine(machined *machineDataV0) error {
	// Gather ports to open and close.
	ports := map[network.Port]bool{}
	for _, unitd := range machined.unitds {
		if unitd.serviced.exposed {
			for _, port := range unitd.ports {
				ports[port] = true
			}
		}
	}
	want := []network.Port{}
	for port := range ports {
		want = append(want, port)
	}
	toOpen := DiffPorts(want, machined.ports)
	toClose := DiffPorts(machined.ports, want)
	machined.ports = want
	if fw.globalMode {
		return fw.flushGlobalPorts(toOpen, toClose)
	}
	return fw.flushInstancePorts(machined, toOpen, toClose)
}

// flushGlobalPorts opens and closes global ports in the environment.
// It keeps a reference count for ports so that only 0-to-1 and 1-to-0 events
// modify the environment.
func (fw *FirewallerV0) flushGlobalPorts(rawOpen, rawClose []network.Port) error {
	// Filter which ports are really to open or close.
	var toOpen, toClose []network.Port
	for _, port := range rawOpen {
		if fw.globalPortRef[port] == 0 {
			toOpen = append(toOpen, port)
		}
		fw.globalPortRef[port]++
	}
	for _, port := range rawClose {
		fw.globalPortRef[port]--
		if fw.globalPortRef[port] == 0 {
			toClose = append(toClose, port)
			delete(fw.globalPortRef, port)
		}
	}
	// Open and close the ports.
	if len(toOpen) > 0 {
		if err := fw.environ.OpenPorts(network.PortsToPortRanges(toOpen)); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		network.SortPorts(toOpen)
		logger.Infof("opened ports %v in environment", toOpen)
	}
	if len(toClose) > 0 {
		if err := fw.environ.ClosePorts(network.PortsToPortRanges(toClose)); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		network.SortPorts(toClose)
		logger.Infof("closed ports %v in environment", toClose)
	}
	return nil
}

// flushInstancePorts opens and closes ports global on the machine.
func (fw *FirewallerV0) flushInstancePorts(machined *machineDataV0, toOpen, toClose []network.Port) error {
	// If there's nothing to do, do nothing.
	// This is important because when a machine is first created,
	// it will have no instance id but also no open ports -
	// InstanceId will fail but we don't care.
	if len(toOpen) == 0 && len(toClose) == 0 {
		return nil
	}
	m, err := machined.machine()
	if params.IsCodeNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	machineId := machined.tag.Id()
	instanceId, err := m.InstanceId()
	if err != nil {
		return err
	}
	instances, err := fw.environ.Instances([]instance.Id{instanceId})
	if err != nil {
		return err
	}
	// Open and close the ports.
	if len(toOpen) > 0 {
		if err := instances[0].OpenPorts(machineId, network.PortsToPortRanges(toOpen)); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		network.SortPorts(toOpen)
		logger.Infof("opened ports %v on %q", toOpen, machined.tag)
	}
	if len(toClose) > 0 {
		if err := instances[0].ClosePorts(machineId, network.PortsToPortRanges(toClose)); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		network.SortPorts(toClose)
		logger.Infof("closed ports %v on %q", toClose, machined.tag)
	}
	return nil
}

// machineLifeChanged starts watching new machines when the firewaller
// is starting, or when new machines come to life, and stops watching
// machines that are dying.
func (fw *FirewallerV0) machineLifeChanged(machineTag string) error {
	tag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return err
	}
	m, err := fw.st.Machine(tag)
	found := !params.IsCodeNotFound(err)
	if found && err != nil {
		return err
	}
	dead := !found || m.Life() == params.Dead
	machined, known := fw.machineds[machineTag]
	if known && dead {
		return fw.forgetMachine(machined)
	}
	if !known && !dead {
		err = fw.startMachine(machineTag)
		if err != nil {
			return err
		}
		logger.Debugf("started watching %q", tag)
	}
	return nil
}

// forgetMachine cleans the machine data after the machine is removed.
func (fw *FirewallerV0) forgetMachine(machined *machineDataV0) error {
	for _, unitd := range machined.unitds {
		fw.forgetUnit(unitd)
	}
	if err := fw.flushMachine(machined); err != nil {
		return err
	}
	delete(fw.machineds, machined.tag.String())
	if err := machined.Stop(); err != nil {
		return err
	}
	logger.Debugf("stopped watching %q", machined.tag)
	return nil
}

// forgetUnit cleans the unit data after the unit is removed.
func (fw *FirewallerV0) forgetUnit(unitd *unitDataV0) {
	name := unitd.unit.Name()
	serviced := unitd.serviced
	machined := unitd.machined
	if err := unitd.Stop(); err != nil {
		logger.Errorf("unit watcher %q returned error when stopping: %v", name, err)
	}
	// Clean up after stopping.
	delete(fw.unitds, name)
	delete(machined.unitds, name)
	delete(serviced.unitds, name)
	if len(serviced.unitds) == 0 {
		// Stop service data after all units are removed.
		if err := serviced.Stop(); err != nil {
			logger.Errorf("service watcher %q returned error when stopping: %v", serviced.service.Name(), err)
		}
		delete(fw.serviceds, serviced.service.Name())
	}
}

// stopWatchers stops all the firewaller's watchers.
func (fw *FirewallerV0) stopWatchers() {
	watcher.Stop(fw.environWatcher, &fw.tomb)
	watcher.Stop(fw.machinesWatcher, &fw.tomb)
	for _, unitd := range fw.unitds {
		watcher.Stop(unitd, &fw.tomb)
	}
	for _, serviced := range fw.serviceds {
		watcher.Stop(serviced, &fw.tomb)
	}
	for _, machined := range fw.machineds {
		watcher.Stop(machined, &fw.tomb)
	}
}

// Err returns the reason why the firewaller has stopped or tomb.ErrStillAlive
// when it is still alive.
func (fw *FirewallerV0) Err() (reason error) {
	return fw.tomb.Err()
}

// Kill implements worker.Worker.Kill.
func (fw *FirewallerV0) Kill() {
	fw.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (fw *FirewallerV0) Wait() error {
	return fw.tomb.Wait()
}

// Stop stops the Firewaller and returns any error encountered while stopping.
func (fw *FirewallerV0) Stop() error {
	fw.tomb.Kill(nil)
	return fw.tomb.Wait()
}

// unitsChangeV0 contains the changed units for one specific machine.
// Only used by FirewallerV0.
type unitsChangeV0 struct {
	machined *machineDataV0
	units    []string
}

// machineDataV0 holds machine details and watches units added or removed.
// Only used by FirewallerV0.
type machineDataV0 struct {
	tomb   tomb.Tomb
	fw     *FirewallerV0
	tag    names.MachineTag
	unitds map[string]*unitDataV0
	ports  []network.Port
}

func (md *machineDataV0) machine() (*apifirewaller.Machine, error) {
	return md.fw.st.Machine(md.tag)
}

// watchLoop watches the machine for units added or removed.
func (md *machineDataV0) watchLoop(unitw apiwatcher.StringsWatcher) {
	defer md.tomb.Done()
	defer watcher.Stop(unitw, &md.tomb)
	for {
		select {
		case <-md.tomb.Dying():
			return
		case change, ok := <-unitw.Changes():
			if !ok {
				_, err := md.machine()
				if !params.IsCodeNotFound(err) {
					md.fw.tomb.Kill(watcher.MustErr(unitw))
				}
				return
			}
			select {
			case md.fw.unitsChange <- &unitsChangeV0{md, change}:
			case <-md.tomb.Dying():
				return
			}
		}
	}
}

// Stop stops the machine watching.
func (md *machineDataV0) Stop() error {
	md.tomb.Kill(nil)
	return md.tomb.Wait()
}

// portsChangeV0 contains the changed ports for one specific unit.
// Only used by FirewallerV0.
type portsChangeV0 struct {
	unitd *unitDataV0
	ports []network.Port
}

// unitDataV0 holds unit details and watches port changes.
// Only used by FirewallerV0.
type unitDataV0 struct {
	tomb     tomb.Tomb
	fw       *FirewallerV0
	unit     *apifirewaller.Unit
	serviced *serviceDataV0
	machined *machineDataV0
	ports    []network.Port
}

// watchLoop watches the unit for port changes.
func (ud *unitDataV0) watchLoop(latestPorts []network.Port) {
	defer ud.tomb.Done()
	w, err := ud.unit.Watch()
	if err != nil {
		ud.fw.tomb.Kill(err)
		return
	}
	defer watcher.Stop(w, &ud.tomb)
	for {
		select {
		case <-ud.tomb.Dying():
			return
		case _, ok := <-w.Changes():
			if !ok {
				ud.fw.tomb.Kill(watcher.MustErr(w))
				return
			}
			if err := ud.unit.Refresh(); err != nil {
				if !params.IsCodeNotFound(err) {
					ud.fw.tomb.Kill(err)
				}
				return
			}
			change, err := ud.unit.OpenedPorts()
			if err != nil {
				ud.fw.tomb.Kill(err)
				return
			}
			if samePorts(change, latestPorts) {
				continue
			}
			latestPorts = append(latestPorts[:0], change...)
			select {
			case ud.fw.portsChange <- &portsChangeV0{ud, change}:
			case <-ud.tomb.Dying():
				return
			}
		}
	}
}

// samePorts returns whether old and new contain the same set of ports.
// Both old and new must be sorted.
func samePorts(old, new []network.Port) bool {
	if len(old) != len(new) {
		return false
	}
	for i, p := range old {
		if new[i] != p {
			return false
		}
	}
	return true
}

// Stop stops the unit watching.
func (ud *unitDataV0) Stop() error {
	ud.tomb.Kill(nil)
	return ud.tomb.Wait()
}

// exposedChangeV0 contains the changed exposed flag for one specific service.
// Only used by FirewallerV0.
type exposedChangeV0 struct {
	serviced *serviceDataV0
	exposed  bool
}

// serviceDataV0 holds service details and watches exposure changes.
// Only used by FirewallerV0.
type serviceDataV0 struct {
	tomb    tomb.Tomb
	fw      *FirewallerV0
	service *apifirewaller.Service
	exposed bool
	unitds  map[string]*unitDataV0
}

// watchLoop watches the service's exposed flag for changes.
func (sd *serviceDataV0) watchLoop(exposed bool) {
	defer sd.tomb.Done()
	w, err := sd.service.Watch()
	if err != nil {
		sd.fw.tomb.Kill(err)
		return
	}
	defer watcher.Stop(w, &sd.tomb)
	for {
		select {
		case <-sd.tomb.Dying():
			return
		case _, ok := <-w.Changes():
			if !ok {
				sd.fw.tomb.Kill(watcher.MustErr(w))
				return
			}
			if err := sd.service.Refresh(); err != nil {
				if !params.IsCodeNotFound(err) {
					sd.fw.tomb.Kill(err)
				}
				return
			}
			change, err := sd.service.IsExposed()
			if err != nil {
				sd.fw.tomb.Kill(err)
				return
			}
			if change == exposed {
				continue
			}
			exposed = change
			select {
			case sd.fw.exposedChange <- &exposedChangeV0{sd, change}:
			case <-sd.tomb.Dying():
				return
			}
		}
	}
}

// Stop stops the service watching.
func (sd *serviceDataV0) Stop() error {
	sd.tomb.Kill(nil)
	return sd.tomb.Wait()
}

// Diff returns all the ports that exist in A but not B.
func DiffPorts(A, B []network.Port) (missing []network.Port) {
next:
	for _, a := range A {
		for _, b := range B {
			if a == b {
				continue next
			}
		}
		missing = append(missing, a)
	}
	return
}
