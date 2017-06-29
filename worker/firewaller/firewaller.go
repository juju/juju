// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"io"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/api/remoterelations"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

// FirewallerAPI exposes functionality off the firewaller API facade to a worker.
type FirewallerAPI interface {
	WatchModelMachines() (watcher.StringsWatcher, error)
	WatchOpenedPorts() (watcher.StringsWatcher, error)
	Machine(tag names.MachineTag) (*firewaller.Machine, error)
	Unit(tag names.UnitTag) (*firewaller.Unit, error)
	Relation(tag names.RelationTag) (*firewaller.Relation, error)
	WatchEgressAddressesForRelation(tag names.RelationTag) (watcher.StringsWatcher, error)
	WatchIngressAddressesForRelation(tag names.RelationTag) (watcher.StringsWatcher, error)
}

// CrossModelFirewallerFacade exposes firewaller functionality on the
// remote offering model to a worker.
type CrossModelFirewallerFacade interface {
	PublishIngressNetworkChange(params.IngressNetworksChangeEvent) error
}

// RemoteFirewallerAPICloser implements CrossModelFirewallerFacade
// and adds a Close() method.
type CrossModelFirewallerFacadeCloser interface {
	io.Closer
	CrossModelFirewallerFacade
}

// EnvironFirewaller defines methods to allow the worker to perform
// firewall operations (open/close ports) on a Juju cloud environment.
type EnvironFirewaller interface {
	environs.Firewaller
}

// EnvironInstances defines methods to allow the worker to perform
// operations on instances in a Juju cloud environment.
type EnvironInstances interface {
	Instances(ids []instance.Id) ([]instance.Instance, error)
}

// Config defines the operation of a Worker.
type Config struct {
	ModelUUID          string
	Mode               string
	FirewallerAPI      FirewallerAPI
	RemoteRelationsApi *remoterelations.Client
	EnvironFirewaller  EnvironFirewaller
	EnvironInstances   EnvironInstances

	NewCrossModelFacadeFunc func(modelUUID string) (CrossModelFirewallerFacadeCloser, error)

	Clock clock.Clock
}

// Validate returns an error if cfg cannot drive a Worker.
func (config Config) Validate() error {
	if config.ModelUUID == "" {
		return errors.NotValidf("empty model uuid")
	}
	if config.FirewallerAPI == nil {
		return errors.NotValidf("nil Firewaller Facade")
	}
	if config.RemoteRelationsApi == nil {
		return errors.NotValidf("nil RemoteRelations Facade")
	}
	if config.EnvironFirewaller == nil {
		return errors.NotValidf("nil EnvironFirewaller")
	}
	if config.EnvironInstances == nil {
		return errors.NotValidf("nil EnvironInstances")
	}
	if config.NewCrossModelFacadeFunc == nil {
		return errors.NotValidf("nil Cross Model Facade func")
	}
	return nil
}

type portRanges map[network.PortRange]bool

// Firewaller watches the state for port ranges opened or closed on
// machines and reflects those changes onto the backing environment.
// Uses Firewaller API V1.
type Firewaller struct {
	catacomb           catacomb.Catacomb
	firewallerApi      FirewallerAPI
	remoteRelationsApi *remoterelations.Client
	environFirewaller  EnvironFirewaller
	environInstances   EnvironInstances

	machinesWatcher      watcher.StringsWatcher
	portsWatcher         watcher.StringsWatcher
	machineds            map[names.MachineTag]*machineData
	unitsChange          chan *unitsChange
	unitds               map[names.UnitTag]*unitData
	applicationids       map[names.ApplicationTag]*applicationData
	exposedChange        chan *exposedChange
	globalMode           bool
	globalIngressRuleRef map[string]int // map of rule names to count of occurrences

	modelUUID                   string
	newRemoteFirewallerAPIFunc  func(modelUUID string) (CrossModelFirewallerFacadeCloser, error)
	remoteRelationsWatcher      watcher.StringsWatcher
	remoteRelationNetworkChange chan *remoteRelationNetworkChange
	localRelationsChange        chan *remoteRelationNetworkChange
	relationIngress             map[names.RelationTag]*remoteRelationData
	pollClock                   clock.Clock
}

// NewFirewaller returns a new Firewaller.
func NewFirewaller(cfg Config) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	clk := cfg.Clock
	if clk == nil {
		clk = clock.WallClock
	}
	fw := &Firewaller{
		firewallerApi:               cfg.FirewallerAPI,
		remoteRelationsApi:          cfg.RemoteRelationsApi,
		environFirewaller:           cfg.EnvironFirewaller,
		environInstances:            cfg.EnvironInstances,
		newRemoteFirewallerAPIFunc:  cfg.NewCrossModelFacadeFunc,
		modelUUID:                   cfg.ModelUUID,
		machineds:                   make(map[names.MachineTag]*machineData),
		unitsChange:                 make(chan *unitsChange),
		unitds:                      make(map[names.UnitTag]*unitData),
		applicationids:              make(map[names.ApplicationTag]*applicationData),
		exposedChange:               make(chan *exposedChange),
		relationIngress:             make(map[names.RelationTag]*remoteRelationData),
		remoteRelationNetworkChange: make(chan *remoteRelationNetworkChange),
		localRelationsChange:        make(chan *remoteRelationNetworkChange),
		pollClock:                   clk,
	}

	switch cfg.Mode {
	case config.FwInstance:
	case config.FwGlobal:
		fw.globalMode = true
		fw.globalIngressRuleRef = make(map[string]int)
	default:
		return nil, errors.Errorf("invalid firewall-mode %q", cfg.Mode)
	}

	err := catacomb.Invoke(catacomb.Plan{
		Site: &fw.catacomb,
		Work: fw.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return fw, nil
}

// stubWatcher is used when the cross model feature flag is not turned on.
type stubWatcher struct {
	watcher.StringsWatcher
	changes watcher.StringsChannel
}

func (stubWatcher *stubWatcher) Stop() error {
	return nil
}

func (stubWatcher *stubWatcher) Changes() watcher.StringsChannel {
	return stubWatcher.changes
}

func (fw *Firewaller) setUp() error {
	var err error
	fw.machinesWatcher, err = fw.firewallerApi.WatchModelMachines()
	if err != nil {
		return errors.Trace(err)
	}
	if err := fw.catacomb.Add(fw.machinesWatcher); err != nil {
		return errors.Trace(err)
	}

	fw.portsWatcher, err = fw.firewallerApi.WatchOpenedPorts()
	if err != nil {
		return errors.Annotatef(err, "failed to start ports watcher")
	}
	if err := fw.catacomb.Add(fw.portsWatcher); err != nil {
		return errors.Trace(err)
	}

	if featureflag.Enabled(feature.CrossModelRelations) {
		fw.remoteRelationsWatcher, err = fw.remoteRelationsApi.WatchRemoteRelations()
		if err != nil {
			return errors.Trace(err)
		}
		if err := fw.catacomb.Add(fw.remoteRelationsWatcher); err != nil {
			return errors.Trace(err)
		}
	} else {
		fw.remoteRelationsWatcher = &stubWatcher{changes: make(watcher.StringsChannel)}
	}

	logger.Debugf("started watching opened port ranges for the model")
	return nil
}

func (fw *Firewaller) loop() error {
	if err := fw.setUp(); err != nil {
		return errors.Trace(err)
	}
	var reconciled bool
	portsChange := fw.portsWatcher.Changes()
	for {
		select {
		case <-fw.catacomb.Dying():
			return fw.catacomb.ErrDying()
		case change, ok := <-fw.machinesWatcher.Changes():
			if !ok {
				return errors.New("machines watcher closed")
			}
			for _, machineId := range change {
				if err := fw.machineLifeChanged(names.NewMachineTag(machineId)); err != nil {
					return err
				}
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
					return errors.Trace(err)
				}
			}
		case change, ok := <-portsChange:
			if !ok {
				return errors.New("ports watcher closed")
			}
			for _, portsGlobalKey := range change {
				machineTag, subnetTag, err := parsePortsKey(portsGlobalKey)
				if err != nil {
					return errors.Trace(err)
				}
				if err := fw.openedPortsChanged(machineTag, subnetTag); err != nil {
					return errors.Trace(err)
				}
			}
		case change, ok := <-fw.remoteRelationsWatcher.Changes():
			if !ok {
				return errors.New("remote relations watcher closed")
			}
			for _, relationKey := range change {
				if err := fw.relationLifeChanged(names.NewRelationTag(relationKey)); err != nil {
					return err
				}
			}
		case change := <-fw.remoteRelationNetworkChange:
			// We need to notify the remote (offering) model that it needs
			// to open new ports to allow ingress from the consumer.
			if err := fw.publishNetworkChanged(change); err != nil {
				return errors.Trace(err)
			}
		case change := <-fw.localRelationsChange:
			// We have a notification that the remote (consuming) model
			// has changed egress networks so need to update the local
			// model to allow those networks through the firewall.
			if err := fw.relationIngressChanged(change); err != nil {
				return errors.Trace(err)
			}
		case change := <-fw.unitsChange:
			if err := fw.unitsChanged(change); err != nil {
				return errors.Trace(err)
			}
		case change := <-fw.exposedChange:
			change.applicationd.exposed = change.exposed
			unitds := []*unitData{}
			for _, unitd := range change.applicationd.unitds {
				unitds = append(unitds, unitd)
			}
			if err := fw.flushUnits(unitds); err != nil {
				return errors.Annotate(err, "cannot change firewall ports")
			}
		}
	}
}

func (fw *Firewaller) publishNetworkChanged(change *remoteRelationNetworkChange) error {
	logger.Debugf("process remote relation egress change for %v", change.relationTag)
	relData, ok := fw.relationIngress[change.relationTag]
	if !ok || relData.remoteRelationId == nil {
		logger.Warningf("ignoring unknown relation %v processing egress change", change.relationTag)
		return nil
	}

	remoteModelAPI, err := fw.newRemoteFirewallerAPIFunc(relData.remoteModelUUID)
	if err != nil {
		return errors.Annotate(err, "cannot open facade to remote model to publish network change")
	}
	defer remoteModelAPI.Close()
	event := params.IngressNetworksChangeEvent{
		RelationId:      *relData.remoteRelationId,
		ApplicationId:   *relData.remoteApplicationId,
		Networks:        change.networks.Values(),
		IngressRequired: change.ingressRequired,
		// TODO(wallyworld) - add macaroon
	}
	err = remoteModelAPI.PublishIngressNetworkChange(event)
	if errors.IsNotFound(err) {
		logger.Debugf("relation id not found publishing %+v", event)
		return nil
	}
	return errors.Annotate(err, "cannot publish ingress network change")
}

func (fw *Firewaller) relationIngressChanged(change *remoteRelationNetworkChange) error {
	logger.Debugf("process remote relation ingress change for %v", change.relationTag)
	relData, ok := fw.relationIngress[change.relationTag]
	if ok {
		relData.networks = change.networks
		relData.ingressRequired = change.ingressRequired
	}
	appData, ok := fw.applicationids[change.localApplicationTag]
	if !ok {
		logger.Debugf("ignoring unknown application: %v", change.localApplicationTag)
		return nil
	}
	unitds := []*unitData{}
	for _, unitd := range appData.unitds {
		unitds = append(unitds, unitd)
	}
	if err := fw.flushUnits(unitds); err != nil {
		return errors.Annotate(err, "cannot change firewall ports")
	}
	return nil
}

// startMachine creates a new data value for tracking details of the
// machine and starts watching the machine for units added or removed.
func (fw *Firewaller) startMachine(tag names.MachineTag) error {
	machined := &machineData{
		fw:           fw,
		tag:          tag,
		unitds:       make(map[names.UnitTag]*unitData),
		ingressRules: make([]network.IngressRule, 0),
		definedPorts: make(map[names.UnitTag]portRanges),
	}
	m, err := machined.machine()
	if params.IsCodeNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Annotate(err, "cannot watch machine units")
	}
	unitw, err := m.WatchUnits()
	if err != nil {
		return errors.Trace(err)
	}
	// XXX(fwereade): this is the best of a bunch of bad options. We've started
	// the watch, so we're responsible for it; but we (probably?) need to do this
	// little dance below to update the machined data on the fw loop goroutine,
	// whence it's usually accessed, before we start the machined watchLoop
	// below. That catacomb *should* be the only one responsible -- and it *is*
	// responsible -- but having it in the main fw catacomb as well does no harm,
	// and greatly simplifies the code below (which would otherwise have to
	// manage unitw lifetime and errors manually).
	if err := fw.catacomb.Add(unitw); err != nil {
		return errors.Trace(err)
	}
	select {
	case <-fw.catacomb.Dying():
		return fw.catacomb.ErrDying()
	case change, ok := <-unitw.Changes():
		if !ok {
			return errors.New("machine units watcher closed")
		}
		fw.machineds[tag] = machined
		err = fw.unitsChanged(&unitsChange{machined, change})
		if err != nil {
			delete(fw.machineds, tag)
			return errors.Annotatef(err, "cannot respond to units changes for %q", tag)
		}
	}

	err = catacomb.Invoke(catacomb.Plan{
		Site: &machined.catacomb,
		Work: func() error {
			return machined.watchLoop(unitw)
		},
	})
	if err != nil {
		delete(fw.machineds, tag)
		return errors.Trace(err)
	}

	// register the machined with the firewaller's catacomb.
	return fw.catacomb.Add(machined)
}

// startUnit creates a new data value for tracking details of the unit
// The provided machineTag must be the tag for the machine the unit was last
// observed to be assigned to.
func (fw *Firewaller) startUnit(unit *firewaller.Unit, machineTag names.MachineTag) error {
	application, err := unit.Application()
	if err != nil {
		return err
	}
	applicationTag := application.Tag()
	unitTag := unit.Tag()
	if err != nil {
		return err
	}
	unitd := &unitData{
		fw:   fw,
		unit: unit,
		tag:  unitTag,
	}
	fw.unitds[unitTag] = unitd

	unitd.machined = fw.machineds[machineTag]
	unitd.machined.unitds[unitTag] = unitd
	if fw.applicationids[applicationTag] == nil {
		err := fw.startApplication(application)
		if err != nil {
			delete(fw.unitds, unitTag)
			delete(unitd.machined.unitds, unitTag)
			return err
		}
	}
	unitd.applicationd = fw.applicationids[applicationTag]
	unitd.applicationd.unitds[unitTag] = unitd

	m, err := unitd.machined.machine()
	if err != nil {
		return err
	}

	// check if the machine has ports open on any subnets
	subnetTags, err := m.ActiveSubnets()
	if err != nil {
		return errors.Annotatef(err, "failed getting %q active subnets", machineTag)
	}
	for _, subnetTag := range subnetTags {
		err := fw.openedPortsChanged(machineTag, subnetTag)
		if err != nil {
			return err
		}
	}

	return nil
}

// startApplication creates a new data value for tracking details of the
// application and starts watching the application for exposure changes.
func (fw *Firewaller) startApplication(app *firewaller.Application) error {
	exposed, err := app.IsExposed()
	if err != nil {
		return err
	}
	applicationd := &applicationData{
		fw:          fw,
		application: app,
		exposed:     exposed,
		unitds:      make(map[names.UnitTag]*unitData),
	}
	fw.applicationids[app.Tag()] = applicationd

	err = catacomb.Invoke(catacomb.Plan{
		Site: &applicationd.catacomb,
		Work: func() error {
			return applicationd.watchLoop(exposed)
		},
	})
	if err != nil {
		return errors.Trace(err)
	}
	if err := fw.catacomb.Add(applicationd); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// reconcileGlobal compares the initially started watcher for machines,
// units and applications with the opened and closed ports globally and
// opens and closes the appropriate ports for the whole environment.
func (fw *Firewaller) reconcileGlobal() error {
	var machines []*machineData
	for _, machined := range fw.machineds {
		machines = append(machines, machined)
	}
	want, err := fw.gatherIngressRules(machines...)
	initialPortRanges, err := fw.environFirewaller.IngressRules()
	if err != nil {
		return err
	}

	// Check which ports to open or to close.
	toOpen, toClose := diffRanges(initialPortRanges, want)
	if len(toOpen) > 0 {
		logger.Infof("opening global ports %v", toOpen)
		if err := fw.environFirewaller.OpenPorts(toOpen); err != nil {
			return err
		}
	}
	if len(toClose) > 0 {
		logger.Infof("closing global ports %v", toClose)
		if err := fw.environFirewaller.ClosePorts(toClose); err != nil {
			return err
		}
	}
	return nil
}

// reconcileInstances compares the initially started watcher for machines,
// units and appications with the opened and closed ports of the instances and
// opens and closes the appropriate ports for each instance.
func (fw *Firewaller) reconcileInstances() error {
	for _, machined := range fw.machineds {
		m, err := machined.machine()
		if params.IsCodeNotFound(err) {
			if err := fw.forgetMachine(machined); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		instanceId, err := m.InstanceId()
		if errors.IsNotProvisioned(err) {
			logger.Errorf("Machine not yet provisioned: %v", err)
			continue
		}
		if err != nil {
			return err
		}
		instances, err := fw.environInstances.Instances([]instance.Id{instanceId})
		if err == environs.ErrNoInstances {
			return nil
		}
		if err != nil {
			return err
		}
		machineId := machined.tag.Id()
		initialRules, err := instances[0].IngressRules(machineId)
		if err != nil {
			return err
		}

		// Check which ports to open or to close.
		toOpen, toClose := diffRanges(initialRules, machined.ingressRules)
		if len(toOpen) > 0 {
			logger.Infof("opening instance port ranges %v for %q",
				toOpen, machined.tag)
			if err := instances[0].OpenPorts(machineId, toOpen); err != nil {
				// TODO(mue) Add local retry logic.
				return err
			}
		}
		if len(toClose) > 0 {
			logger.Infof("closing instance port ranges %v for %q",
				toClose, machined.tag)
			if err := instances[0].ClosePorts(machineId, toClose); err != nil {
				// TODO(mue) Add local retry logic.
				return err
			}
		}
	}
	return nil
}

// unitsChanged responds to changes to the assigned units.
func (fw *Firewaller) unitsChanged(change *unitsChange) error {
	changed := []*unitData{}
	for _, name := range change.units {
		unitTag := names.NewUnitTag(name)
		unit, err := fw.firewallerApi.Unit(unitTag)
		if err != nil && !params.IsCodeNotFound(err) {
			return err
		}
		var machineTag names.MachineTag
		if unit != nil {
			machineTag, err = unit.AssignedMachine()
			if params.IsCodeNotFound(err) {
				continue
			} else if err != nil && !params.IsCodeNotAssigned(err) {
				return err
			}
		}
		if unitd, known := fw.unitds[unitTag]; known {
			knownMachineTag := fw.unitds[unitTag].machined.tag
			if unit == nil || unit.Life() == params.Dead || machineTag != knownMachineTag {
				fw.forgetUnit(unitd)
				changed = append(changed, unitd)
				logger.Debugf("stopped watching unit %s", name)
			}
			// TODO(dfc) fw.machineds should be map[names.Tag]
		} else if unit != nil && unit.Life() != params.Dead && fw.machineds[machineTag] != nil {
			err = fw.startUnit(unit, machineTag)
			if err != nil {
				return err
			}
			changed = append(changed, fw.unitds[unitTag])
			logger.Debugf("started watching %q", unitTag)
		}
	}
	if err := fw.flushUnits(changed); err != nil {
		return errors.Annotate(err, "cannot change firewall ports")
	}
	return nil
}

// openedPortsChanged handles port change notifications
func (fw *Firewaller) openedPortsChanged(machineTag names.MachineTag, subnetTag names.SubnetTag) error {

	machined, ok := fw.machineds[machineTag]
	if !ok {
		// It is common to receive a port change notification before
		// registering the machine, so if a machine is not found in
		// firewaller's list, just skip the change.
		logger.Debugf("failed to lookup %q, skipping port change", machineTag)
		return nil
	}

	m, err := machined.machine()
	if err != nil {
		return err
	}

	ports, err := m.OpenedPorts(subnetTag)
	if err != nil {
		return err
	}

	newPortRanges := make(map[names.UnitTag]portRanges)
	for portRange, unitTag := range ports {
		unitd, ok := machined.unitds[unitTag]
		if !ok {
			// It is common to receive port change notification before
			// registering a unit. Skip handling the port change - it will
			// be handled when the unit is registered.
			logger.Debugf("failed to lookup %q, skipping port change", unitTag)
			return nil
		}
		ranges, ok := newPortRanges[unitd.tag]
		if !ok {
			ranges = make(portRanges)
			newPortRanges[unitd.tag] = ranges
		}
		ranges[portRange] = true
	}

	if !unitPortsEqual(machined.definedPorts, newPortRanges) {
		machined.definedPorts = newPortRanges
		return fw.flushMachine(machined)
	}
	return nil
}

func unitPortsEqual(a, b map[names.UnitTag]portRanges) bool {
	if len(a) != len(b) {
		return false
	}
	for key, valueA := range a {
		valueB, exists := b[key]
		if !exists {
			return false
		}
		if !portRangesEqual(valueA, valueB) {
			return false
		}
	}
	return true
}

func portRangesEqual(a, b portRanges) bool {
	if len(a) != len(b) {
		return false
	}
	for key, valueA := range a {
		valueB, exists := b[key]
		if !exists {
			return false
		}
		if valueA != valueB {
			return false
		}
	}
	return true
}

// flushUnits opens and closes ports for the passed unit data.
func (fw *Firewaller) flushUnits(unitds []*unitData) error {
	machineds := map[names.MachineTag]*machineData{}
	for _, unitd := range unitds {
		machineds[unitd.machined.tag] = unitd.machined
	}
	for _, machined := range machineds {
		if err := fw.flushMachine(machined); err != nil {
			return err
		}
	}
	return nil
}

// flushMachine opens and closes ports for the passed machine.
func (fw *Firewaller) flushMachine(machined *machineData) error {
	want, err := fw.gatherIngressRules(machined)
	if err != nil {
		return errors.Trace(err)
	}
	toOpen, toClose := diffRanges(machined.ingressRules, want)
	machined.ingressRules = want
	if fw.globalMode {
		return fw.flushGlobalPorts(toOpen, toClose)
	}
	return fw.flushInstancePorts(machined, toOpen, toClose)
}

// gatherIngressRules returns the ingress rules to open and close
// for the specified machines.
func (fw *Firewaller) gatherIngressRules(machines ...*machineData) ([]network.IngressRule, error) {
	var want []network.IngressRule
	for _, machined := range machines {
		for unitTag, portRanges := range machined.definedPorts {
			unitd, known := machined.unitds[unitTag]
			if !known {
				logger.Debugf("no ingress rules for unknown %v on %v", unitTag, machined.tag)
				continue
			}

			cidrs := set.NewStrings()
			// If the unit is exposed, allow access from everywhere.
			if unitd.applicationd.exposed {
				cidrs.Add("0.0.0.0/0")
			} else {
				// Not exposed, so add any ingress rules required by remote relations.
				if err := fw.updateForRemoteRelationIngress(unitd.applicationd.application.Tag(), cidrs); err != nil {
					return nil, errors.Trace(err)
				}
				logger.Debugf("CIDRS for %v: %v", unitTag, cidrs.Values())
			}
			if cidrs.Size() > 0 {
				for portRange := range portRanges {
					sourceCidrs := cidrs.SortedValues()
					rule, err := network.NewIngressRule(portRange.Protocol, portRange.FromPort, portRange.ToPort, sourceCidrs...)
					if err != nil {
						return nil, errors.Trace(err)
					}
					want = append(want, rule)
				}
			}
		}
	}
	return want, nil
}

func (fw *Firewaller) updateForRemoteRelationIngress(appTag names.ApplicationTag, cidrs set.Strings) error {
	logger.Debugf("finding egress rules for %v", appTag)
	// Now create the rules for any remote relations of which the
	// unit's application is a part.
	for _, data := range fw.relationIngress {
		if data.localApplicationTag != appTag {
			continue
		}
		if !data.ingressRequired {
			continue
		}
		for _, cidr := range data.networks.Values() {
			cidrs.Add(cidr)
		}
	}
	return nil
}

// flushGlobalPorts opens and closes global ports in the environment.
// It keeps a reference count for ports so that only 0-to-1 and 1-to-0 events
// modify the environment.
func (fw *Firewaller) flushGlobalPorts(rawOpen, rawClose []network.IngressRule) error {
	// Filter which ports are really to open or close.
	var toOpen, toClose []network.IngressRule
	for _, rule := range rawOpen {
		ruleName := rule.String()
		if fw.globalIngressRuleRef[ruleName] == 0 {
			toOpen = append(toOpen, rule)
		}
		fw.globalIngressRuleRef[ruleName]++
	}
	for _, rule := range rawClose {
		ruleName := rule.String()
		fw.globalIngressRuleRef[ruleName]--
		if fw.globalIngressRuleRef[ruleName] == 0 {
			toClose = append(toClose, rule)
			delete(fw.globalIngressRuleRef, ruleName)
		}
	}
	// Open and close the ports.
	if len(toOpen) > 0 {
		if err := fw.environFirewaller.OpenPorts(toOpen); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		network.SortIngressRules(toOpen)
		logger.Infof("opened port ranges %v in environment", toOpen)
	}
	if len(toClose) > 0 {
		if err := fw.environFirewaller.ClosePorts(toClose); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		network.SortIngressRules(toClose)
		logger.Infof("closed port ranges %v in environment", toClose)
	}
	return nil
}

// flushInstancePorts opens and closes ports global on the machine.
func (fw *Firewaller) flushInstancePorts(machined *machineData, toOpen, toClose []network.IngressRule) error {
	// If there's nothing to do, do nothing.
	// This is important because when a machine is first created,
	// it will have no instance id but also no open ports -
	// InstanceId will fail but we don't care.
	logger.Debugf("flush instance ports: to open %v, to close %v", toOpen, toClose)
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
	instances, err := fw.environInstances.Instances([]instance.Id{instanceId})
	if err != nil {
		return err
	}
	// Open and close the ports.
	if len(toOpen) > 0 {
		if err := instances[0].OpenPorts(machineId, toOpen); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		network.SortIngressRules(toOpen)
		logger.Infof("opened port ranges %v on %q", toOpen, machined.tag)
	}
	if len(toClose) > 0 {
		if err := instances[0].ClosePorts(machineId, toClose); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		network.SortIngressRules(toClose)
		logger.Infof("closed port ranges %v on %q", toClose, machined.tag)
	}
	return nil
}

// machineLifeChanged starts watching new machines when the firewaller
// is starting, or when new machines come to life, and stops watching
// machines that are dying.
func (fw *Firewaller) machineLifeChanged(tag names.MachineTag) error {
	m, err := fw.firewallerApi.Machine(tag)
	found := !params.IsCodeNotFound(err)
	if found && err != nil {
		return err
	}
	dead := !found || m.Life() == params.Dead
	machined, known := fw.machineds[tag]
	if known && dead {
		return fw.forgetMachine(machined)
	}
	if !known && !dead {
		err = fw.startMachine(tag)
		if err != nil {
			return err
		}
		logger.Debugf("started watching %q", tag)
	}
	return nil
}

// forgetMachine cleans the machine data after the machine is removed.
func (fw *Firewaller) forgetMachine(machined *machineData) error {
	for _, unitd := range machined.unitds {
		fw.forgetUnit(unitd)
	}
	if err := fw.flushMachine(machined); err != nil {
		return errors.Trace(err)
	}

	// Unusually, it's fine to ignore this error, because we know the machined
	// is being tracked in fw.catacomb. But we do still want to wait until the
	// watch loop has stopped before we nuke the last data and return.
	worker.Stop(machined)
	delete(fw.machineds, machined.tag)
	logger.Debugf("stopped watching %q", machined.tag)
	return nil
}

// forgetUnit cleans the unit data after the unit is removed.
func (fw *Firewaller) forgetUnit(unitd *unitData) {
	applicationd := unitd.applicationd
	machined := unitd.machined

	// If it's the last unit in the application, we'll need to stop the applicationd.
	stoppedApplication := false
	if len(applicationd.unitds) == 1 {
		if _, found := applicationd.unitds[unitd.tag]; found {
			// Unusually, it's fine to ignore this error, because we know the
			// applicationd is being tracked in fw.catacomb. But we do still want
			// to wait until the watch loop has stopped before we nuke the last
			// data and return.
			worker.Stop(applicationd)
			stoppedApplication = true
		}
	}

	// Clean up after stopping.
	delete(fw.unitds, unitd.tag)
	delete(machined.unitds, unitd.tag)
	delete(applicationd.unitds, unitd.tag)
	logger.Debugf("stopped watching %q", unitd.tag)
	if stoppedApplication {
		applicationTag := applicationd.application.Tag()
		delete(fw.applicationids, applicationTag)
		logger.Debugf("stopped watching %q", applicationTag)
	}
}

// Kill is part of the worker.Worker interface.
func (fw *Firewaller) Kill() {
	fw.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (fw *Firewaller) Wait() error {
	return fw.catacomb.Wait()
}

// unitsChange contains the changed units for one specific machine.
type unitsChange struct {
	machined *machineData
	units    []string
}

// machineData holds machine details and watches units added or removed.
type machineData struct {
	catacomb     catacomb.Catacomb
	fw           *Firewaller
	tag          names.MachineTag
	unitds       map[names.UnitTag]*unitData
	ingressRules []network.IngressRule
	// ports defined by units on this machine
	definedPorts map[names.UnitTag]portRanges
}

func (md *machineData) machine() (*firewaller.Machine, error) {
	return md.fw.firewallerApi.Machine(md.tag)
}

// watchLoop watches the machine for units added or removed.
func (md *machineData) watchLoop(unitw watcher.StringsWatcher) error {
	if err := md.catacomb.Add(unitw); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-md.catacomb.Dying():
			return md.catacomb.ErrDying()
		case change, ok := <-unitw.Changes():
			if !ok {
				return errors.New("machine units watcher closed")
			}
			select {
			case <-md.catacomb.Dying():
				return md.catacomb.ErrDying()
			case md.fw.unitsChange <- &unitsChange{md, change}:
			}
		}
	}
}

// Kill is part of the worker.Worker interface.
func (md *machineData) Kill() {
	md.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (md *machineData) Wait() error {
	return md.catacomb.Wait()
}

// unitData holds unit details.
type unitData struct {
	fw           *Firewaller
	tag          names.UnitTag
	unit         *firewaller.Unit
	applicationd *applicationData
	machined     *machineData
}

// exposedChange contains the changed exposed flag for one specific application.
type exposedChange struct {
	applicationd *applicationData
	exposed      bool
}

// applicationData holds application details and watches exposure changes.
type applicationData struct {
	catacomb    catacomb.Catacomb
	fw          *Firewaller
	application *firewaller.Application
	exposed     bool
	unitds      map[names.UnitTag]*unitData
}

// watchLoop watches the application's exposed flag for changes.
func (ad *applicationData) watchLoop(exposed bool) error {
	appWatcher, err := ad.application.Watch()
	if err != nil {
		if params.IsCodeNotFound(err) {
			return nil
		}
		return errors.Trace(err)
	}
	if err := ad.catacomb.Add(appWatcher); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-ad.catacomb.Dying():
			return ad.catacomb.ErrDying()
		case _, ok := <-appWatcher.Changes():
			if !ok {
				return errors.New("application watcher closed")
			}
			if err := ad.application.Refresh(); err != nil {
				if !params.IsCodeNotFound(err) {
					return errors.Trace(err)
				}
				return nil
			}
			change, err := ad.application.IsExposed()
			if err != nil {
				return errors.Trace(err)
			}
			if change == exposed {
				continue
			}

			exposed = change
			select {
			case <-ad.catacomb.Dying():
				return ad.catacomb.ErrDying()
			case ad.fw.exposedChange <- &exposedChange{ad, change}:
			}
		}
	}
}

// Kill is part of the worker.Worker interface.
func (ad *applicationData) Kill() {
	ad.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (ad *applicationData) Wait() error {
	return ad.catacomb.Wait()
}

// parsePortsKey parses a ports document global key coming from the ports
// watcher (e.g. "42:0.1.2.0/24") and returns the machine and subnet tags from
// its components (in the last example "machine-42" and "subnet-0.1.2.0/24").
func parsePortsKey(change string) (machineTag names.MachineTag, subnetTag names.SubnetTag, err error) {
	defer errors.DeferredAnnotatef(&err, "invalid ports change %q", change)

	parts := strings.SplitN(change, ":", 2)
	if len(parts) != 2 {
		return names.MachineTag{}, names.SubnetTag{}, errors.Errorf("unexpected format")
	}
	machineID, subnetID := parts[0], parts[1]

	machineTag = names.NewMachineTag(machineID)
	if subnetID != "" {
		subnetTag = names.NewSubnetTag(subnetID)
	}
	return machineTag, subnetTag, nil
}

func diffRanges(currentRules, wantedRules []network.IngressRule) (toOpen, toClose []network.IngressRule) {
	portCidrs := func(rules []network.IngressRule) map[network.PortRange]set.Strings {
		result := make(map[network.PortRange]set.Strings)
		for _, rule := range rules {
			cidrs, ok := result[rule.PortRange]
			if !ok {
				cidrs = set.NewStrings()
				result[rule.PortRange] = cidrs
			}
			ruleCidrs := rule.SourceCIDRs
			if len(ruleCidrs) == 0 {
				ruleCidrs = []string{"0.0.0.0/0"}
			}
			for _, cidr := range ruleCidrs {
				cidrs.Add(cidr)
			}
		}
		return result
	}

	currentPortCidrs := portCidrs(currentRules)
	wantedPortCidrs := portCidrs(wantedRules)
	for portRange, wantedCidrs := range wantedPortCidrs {
		existingCidrs, ok := currentPortCidrs[portRange]

		// If the wanted port range doesn't exist at all, the entire rule is to be opened.
		if !ok {
			rule := network.IngressRule{PortRange: portRange, SourceCIDRs: wantedCidrs.SortedValues()}
			toOpen = append(toOpen, rule)
			continue
		}

		// Figure out the difference between CIDRs to get the rules to open/close.
		toOpenCidrs := wantedCidrs.Difference(existingCidrs)
		if toOpenCidrs.Size() > 0 {
			rule := network.IngressRule{PortRange: portRange, SourceCIDRs: toOpenCidrs.SortedValues()}
			toOpen = append(toOpen, rule)
		}
		toCloseCidrs := existingCidrs.Difference(wantedCidrs)
		if toCloseCidrs.Size() > 0 {
			rule := network.IngressRule{PortRange: portRange, SourceCIDRs: toCloseCidrs.SortedValues()}
			toClose = append(toClose, rule)
		}
	}

	for portRange, currentCidrs := range currentPortCidrs {
		// If a current port range doesn't exist at all in the wanted set, the entire rule is to be closed.
		if _, ok := wantedPortCidrs[portRange]; !ok {
			rule := network.IngressRule{PortRange: portRange, SourceCIDRs: currentCidrs.SortedValues()}
			toClose = append(toClose, rule)
		}
	}
	network.SortIngressRules(toOpen)
	network.SortIngressRules(toClose)
	return toOpen, toClose
}

// relationLifeChanged manages the workers to process ingress changes for
// the specified relation.
func (fw *Firewaller) relationLifeChanged(tag names.RelationTag) error {
	results, err := fw.remoteRelationsApi.Relations([]string{tag.Id()})
	if err != nil {
		return errors.Trace(err)
	}
	relErr := results[0].Error
	notfound := relErr != nil && params.IsCodeNotFound(relErr)
	if relErr != nil && !notfound {
		return err
	}
	rel := results[0].Result

	dead := notfound || rel.Life == params.Dead
	data, known := fw.relationIngress[tag]
	if known && dead {
		logger.Debugf("relation %v was known but has died", tag.Id())
		return fw.forgetRelation(data)
	}
	if !known && !dead {
		err := fw.startRelation(rel, rel.Endpoint.Role)
		if err != nil {
			return err
		}
	}
	return nil
}

type remoteRelationInfo struct {
	relationId          params.RemoteEntityId
	remoteApplicationId params.RemoteEntityId
}

type remoteRelationData struct {
	catacomb      catacomb.Catacomb
	fw            *Firewaller
	relationReady chan remoteRelationInfo

	tag                 names.RelationTag
	localApplicationTag names.ApplicationTag
	remoteRelationId    *params.RemoteEntityId
	remoteApplicationId *params.RemoteEntityId
	remoteModelUUID     string
	endpointRole        charm.RelationRole

	// These values are updated when ingress information on the
	// relation changes in the model.
	ingressRequired bool
	networks        set.Strings
}

// startRelation creates a new data value for tracking details of the
// relation and starts watching the related models for subnets added or removed.
func (fw *Firewaller) startRelation(rel *params.RemoteRelation, role charm.RelationRole) error {
	tag := names.NewRelationTag(rel.Key)
	data := &remoteRelationData{
		fw:                  fw,
		tag:                 tag,
		remoteModelUUID:     rel.SourceModelUUID,
		localApplicationTag: names.NewApplicationTag(rel.ApplicationName),
		endpointRole:        role,
		relationReady:       make(chan remoteRelationInfo),
	}
	fw.relationIngress[tag] = data

	err := catacomb.Invoke(catacomb.Plan{
		Site: &data.catacomb,
		Work: data.watchLoop,
	})
	if err != nil {
		delete(fw.relationIngress, tag)
		return errors.Trace(err)
	}

	// register the relationData with the firewaller's catacomb.
	if err := fw.catacomb.Add(data); err != nil {
		delete(fw.relationIngress, tag)
		return errors.Trace(err)
	}

	var relModelUUID string
	if role == charm.RoleRequirer {
		// The requirer side initiates the relation so the model UUID
		// used to export the relation is that of the consuming side.
		relModelUUID = fw.modelUUID
	} else {
		// Here SourceModelUUID is source model of the offer, which is
		// the model UUID used to register the relation on the offering side.
		relModelUUID = rel.SourceModelUUID
	}
	return fw.startRelationPoller(relModelUUID, rel.SourceModelUUID, rel.Key, rel.RemoteApplicationName, data.relationReady)
}

// watchLoop watches the relation for networks added or removed.
func (rd *remoteRelationData) watchLoop() error {
	// First, wait for relation to become ready.
	for rd.remoteRelationId == nil {
		select {
		case <-rd.catacomb.Dying():
			return rd.catacomb.ErrDying()
		case remoteRelationInfo := <-rd.relationReady:
			rd.remoteRelationId = &remoteRelationInfo.relationId
			rd.remoteApplicationId = &remoteRelationInfo.remoteApplicationId
			logger.Debugf(
				"relation %v for remote app %v in model %v is ready",
				rd.remoteRelationId, rd.remoteApplicationId, rd.remoteModelUUID)
		}
	}

	if rd.endpointRole == charm.RoleRequirer {
		return rd.requirerEndpointLoop()
	}
	return rd.providerEndpointLoop()
}

func (rd *remoteRelationData) requirerEndpointLoop() error {
	logger.Debugf("starting requirer endpoint loop for %v on %v ", rd.tag.Id(), rd.localApplicationTag.Id())
	// Now watch for updates to egress addresses so we can inform the offering
	// model what firewall ingress to allow.
	egressAddressWatcher, err := rd.fw.firewallerApi.WatchEgressAddressesForRelation(rd.tag)
	if err != nil {
		if !params.IsCodeNotFound(err) && !params.IsCodeNotSupported(err) {
			return errors.Trace(err)
		}
		logger.Infof("no egress required for %v", rd.localApplicationTag)
		rd.ingressRequired = false
		return nil
	}
	if err := rd.catacomb.Add(egressAddressWatcher); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-rd.catacomb.Dying():
			return rd.catacomb.ErrDying()
		case cidrs := <-egressAddressWatcher.Changes():
			logger.Debugf("relation egress addresses for %v changed in model %v: %v", rd.tag, rd.fw.modelUUID, cidrs)
			if err := rd.updateProviderModel(*rd.remoteRelationId, cidrs); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (rd *remoteRelationData) providerEndpointLoop() error {
	logger.Debugf("starting provider endpoint loop for %v on %v ", rd.tag.Id(), rd.localApplicationTag.Id())
	// Watch for ingress changes requested by the consuming model.
	ingressAddressWatcher, err := rd.fw.firewallerApi.WatchIngressAddressesForRelation(rd.tag)
	if err != nil {
		if !params.IsCodeNotFound(err) && !params.IsCodeNotSupported(err) {
			return errors.Trace(err)
		}
		logger.Infof("no ingress required for %v", rd.localApplicationTag)
		rd.ingressRequired = false
		return nil
	}
	if err := rd.catacomb.Add(ingressAddressWatcher); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-rd.catacomb.Dying():
			return rd.catacomb.ErrDying()
		case cidrs := <-ingressAddressWatcher.Changes():
			logger.Debugf("relation ingress addresses for %v changed in model %v: %v", rd.tag, rd.fw.modelUUID, cidrs)
			if err := rd.updateIngressNetworks(*rd.remoteRelationId, cidrs); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

type remoteRelationNetworkChange struct {
	relationTag         names.RelationTag
	localApplicationTag names.ApplicationTag
	networks            set.Strings
	ingressRequired     bool
}

// updateProviderModel gathers the ingress CIDRs for the relation and notifies
// that a change has occurred.
func (rd *remoteRelationData) updateProviderModel(remoteRelationId params.RemoteEntityId, cidrs []string) error {
	logger.Debugf("ingress cidrs for %v: %+v", remoteRelationId, cidrs)
	change := &remoteRelationNetworkChange{
		relationTag:         rd.tag,
		localApplicationTag: rd.localApplicationTag,
		networks:            set.NewStrings(cidrs...),
		ingressRequired:     len(cidrs) > 0,
	}
	select {
	case <-rd.catacomb.Dying():
		return rd.catacomb.ErrDying()
	case rd.fw.remoteRelationNetworkChange <- change:
	}
	return nil
}

// updateIngressNetworks processes the changed ingress networks on the relation.
func (rd *remoteRelationData) updateIngressNetworks(remoteRelationId params.RemoteEntityId, cidrs []string) error {
	logger.Debugf("ingress cidrs for %v: %+v", remoteRelationId, cidrs)
	change := &remoteRelationNetworkChange{
		relationTag:         rd.tag,
		localApplicationTag: rd.localApplicationTag,
		networks:            set.NewStrings(cidrs...),
		ingressRequired:     len(cidrs) > 0,
	}
	select {
	case <-rd.catacomb.Dying():
		return rd.catacomb.ErrDying()
	case rd.fw.localRelationsChange <- change:
	}
	return nil
}

// Kill is part of the worker.Worker interface.
func (rd *remoteRelationData) Kill() {
	rd.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (rd *remoteRelationData) Wait() error {
	return rd.catacomb.Wait()
}

// forgetRelation cleans the relation data after the relation is removed.
func (fw *Firewaller) forgetRelation(data *remoteRelationData) error {
	logger.Debugf("forget relation %v", data.tag.Id())
	delete(fw.relationIngress, data.tag)
	change := &remoteRelationNetworkChange{
		relationTag:         data.tag,
		localApplicationTag: data.localApplicationTag,
		networks:            make(set.Strings),
	}
	if err := fw.publishNetworkChanged(change); err != nil {
		return errors.Trace(err)
	}

	// TODO(wallyworld) - we need to unregister with the remote model

	// Unusually, it's fine to ignore this error, because we know the relation data
	// is being tracked in fw.catacomb. But we do still want to wait until the
	// watch loop has stopped before we nuke the last data and return.
	worker.Stop(data)
	logger.Debugf("stopped watching %q", data.tag)
	return nil
}

type remoteRelationPoller struct {
	catacomb       catacomb.Catacomb
	fw             *Firewaller
	relationTag    names.RelationTag
	applicationTag names.ApplicationTag
	relModelUUID   string
	appModelUUID   string
	relationReady  chan remoteRelationInfo
}

// startRelationPoller creates a new worker which waits until a remote
// relation is registered in both models.
func (fw *Firewaller) startRelationPoller(relModelUUID, appModelUUID, relationKey, remoteAppName string, relationReady chan remoteRelationInfo) error {
	poller := &remoteRelationPoller{
		fw:             fw,
		relationTag:    names.NewRelationTag(relationKey),
		applicationTag: names.NewApplicationTag(remoteAppName),
		relationReady:  relationReady,
		relModelUUID:   relModelUUID,
		appModelUUID:   appModelUUID,
	}

	err := catacomb.Invoke(catacomb.Plan{
		Site: &poller.catacomb,
		Work: poller.pollLoop,
	})
	if err != nil {
		return errors.Trace(err)
	}

	// register poller with the firewaller's catacomb.
	return fw.catacomb.Add(poller)
}

// pollLoop waits for a remote relation to be registered.
// It does this by waiting for the relation and app tokens to be created.
func (p *remoteRelationPoller) pollLoop() error {
	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()
		case <-p.fw.pollClock.After(3 * time.Second):
			// Relation is exported with the consuming model UUID.
			relToken, err := p.fw.remoteRelationsApi.GetToken(p.relModelUUID, p.relationTag)
			if err != nil {
				continue
			}
			logger.Debugf("token %v for relation id: %v in model %v", relToken, p.relationTag.Id(), p.relModelUUID)

			// Application is exported with the offering model UUID.
			appToken, err := p.fw.remoteRelationsApi.GetToken(p.appModelUUID, p.applicationTag)
			if err != nil {
				continue
			}
			logger.Debugf("token %v for application id: %v in model %v", appToken, p.applicationTag.Id(), p.appModelUUID)

			// relation and application are ready.
			remoteRelationId := params.RemoteEntityId{ModelUUID: p.relModelUUID, Token: relToken}
			remoteApplicationId := params.RemoteEntityId{ModelUUID: p.appModelUUID, Token: appToken}
			releationInfo := remoteRelationInfo{
				relationId:          remoteRelationId,
				remoteApplicationId: remoteApplicationId,
			}
			select {
			case <-p.catacomb.Dying():
				return p.catacomb.ErrDying()
			case p.relationReady <- releationInfo:
			}
			return nil
		}
	}
}

// Kill is part of the worker.Worker interface.
func (p *remoteRelationPoller) Kill() {
	p.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (p *remoteRelationPoller) Wait() error {
	return p.catacomb.Wait()
}
