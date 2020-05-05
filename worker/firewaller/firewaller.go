// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"io"
	"strings"
	"time"

	"github.com/EvilSuperstars/go-cidrman"
	"github.com/juju/charm/v7"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/api/remoterelations"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/network"
	"github.com/juju/juju/worker/common"
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
	ControllerAPIInfoForModel(modelUUID string) (*api.Info, error)
	MacaroonForRelation(relationKey string) (*macaroon.Macaroon, error)
	SetRelationStatus(relationKey string, status relation.Status, message string) error
	FirewallRules(applicationNames ...string) ([]params.FirewallRule, error)
}

// CrossModelFirewallerFacade exposes firewaller functionality on the
// remote offering model to a worker.
type CrossModelFirewallerFacade interface {
	PublishIngressNetworkChange(params.IngressNetworksChangeEvent) error
	WatchEgressAddressesForRelation(details params.RemoteEntityArg) (watcher.StringsWatcher, error)
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
	Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error)
}

type newCrossModelFacadeFunc func(*api.Info) (CrossModelFirewallerFacadeCloser, error)

// Config defines the operation of a Worker.
type Config struct {
	ModelUUID          string
	Mode               string
	FirewallerAPI      FirewallerAPI
	RemoteRelationsApi *remoterelations.Client
	EnvironFirewaller  EnvironFirewaller
	EnvironInstances   EnvironInstances

	NewCrossModelFacadeFunc newCrossModelFacadeFunc

	Clock  clock.Clock
	Logger Logger

	CredentialAPI common.CredentialAPI
}

// Validate returns an error if cfg cannot drive a Worker.
func (cfg Config) Validate() error {
	if cfg.ModelUUID == "" {
		return errors.NotValidf("empty model uuid")
	}
	if cfg.FirewallerAPI == nil {
		return errors.NotValidf("nil Firewaller Facade")
	}
	if cfg.RemoteRelationsApi == nil {
		return errors.NotValidf("nil RemoteRelations Facade")
	}
	if cfg.Mode == config.FwGlobal && cfg.EnvironFirewaller == nil {
		return errors.NotValidf("nil EnvironFirewaller")
	}
	if cfg.EnvironInstances == nil {
		return errors.NotValidf("nil EnvironInstances")
	}
	if cfg.NewCrossModelFacadeFunc == nil {
		return errors.NotValidf("nil Cross Model Facade func")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.CredentialAPI == nil {
		return errors.NotValidf("nil Credential Facade")
	}
	return nil
}

type portRanges map[corenetwork.PortRange]bool

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

	modelUUID                  string
	newRemoteFirewallerAPIFunc newCrossModelFacadeFunc
	remoteRelationsWatcher     watcher.StringsWatcher
	localRelationsChange       chan *remoteRelationNetworkChange
	relationIngress            map[names.RelationTag]*remoteRelationData
	relationWorkerRunner       *worker.Runner
	pollClock                  clock.Clock
	logger                     Logger

	cloudCallContext context.ProviderCallContext
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
		firewallerApi:              cfg.FirewallerAPI,
		remoteRelationsApi:         cfg.RemoteRelationsApi,
		environFirewaller:          cfg.EnvironFirewaller,
		environInstances:           cfg.EnvironInstances,
		newRemoteFirewallerAPIFunc: cfg.NewCrossModelFacadeFunc,
		modelUUID:                  cfg.ModelUUID,
		machineds:                  make(map[names.MachineTag]*machineData),
		unitsChange:                make(chan *unitsChange),
		unitds:                     make(map[names.UnitTag]*unitData),
		applicationids:             make(map[names.ApplicationTag]*applicationData),
		exposedChange:              make(chan *exposedChange),
		relationIngress:            make(map[names.RelationTag]*remoteRelationData),
		localRelationsChange:       make(chan *remoteRelationNetworkChange),
		pollClock:                  clk,
		logger:                     cfg.Logger,
		relationWorkerRunner: worker.NewRunner(worker.RunnerParams{
			Clock: clk,

			// One of the remote relation workers failing should not
			// prevent the others from running.
			IsFatal: func(error) bool { return false },

			// For any failures, try again in 1 minute.
			RestartDelay: time.Minute,
		}),
		cloudCallContext: common.NewCloudCallContext(cfg.CredentialAPI, nil),
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
		Init: []worker.Worker{fw.relationWorkerRunner},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return fw, nil
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

	fw.remoteRelationsWatcher, err = fw.remoteRelationsApi.WatchRemoteRelations()
	if err != nil {
		return errors.Trace(err)
	}
	if err := fw.catacomb.Add(fw.remoteRelationsWatcher); err != nil {
		return errors.Trace(err)
	}

	fw.logger.Debugf("started watching opened port ranges for the model")
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

func (fw *Firewaller) relationIngressChanged(change *remoteRelationNetworkChange) error {
	fw.logger.Debugf("process remote relation ingress change for %v", change.relationTag)
	relData, ok := fw.relationIngress[change.relationTag]
	if ok {
		relData.networks = change.networks
		relData.ingressRequired = change.ingressRequired
	}
	appData, ok := fw.applicationids[change.localApplicationTag]
	if !ok {
		fw.logger.Debugf("ignoring unknown application: %v", change.localApplicationTag)
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
		fw.logger.Debugf("not watching %q", tag)
		return nil
	} else if err != nil {
		return errors.Annotate(err, "cannot watch machine units")
	}
	manual, err := m.IsManual()
	if err != nil {
		return errors.Trace(err)
	}
	if manual {
		// Don't track manual machines, we can't change their ports.
		fw.logger.Debugf("not watching manual %q", tag)
		return nil
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
	err = fw.catacomb.Add(machined)
	if err == nil {
		fw.logger.Debugf("started watching %q", tag)
	}
	return err
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
	if err != nil {
		return err
	}
	initialPortRanges, err := fw.environFirewaller.IngressRules(fw.cloudCallContext)
	if err != nil {
		return err
	}

	// Check which ports to open or to close.
	toOpen, toClose := diffRanges(initialPortRanges, want)
	if len(toOpen) > 0 {
		fw.logger.Infof("opening global ports %v", toOpen)
		if err := fw.environFirewaller.OpenPorts(fw.cloudCallContext, toOpen); err != nil {
			return err
		}
	}
	if len(toClose) > 0 {
		fw.logger.Infof("closing global ports %v", toClose)
		if err := fw.environFirewaller.ClosePorts(fw.cloudCallContext, toClose); err != nil {
			return err
		}
	}
	return nil
}

// reconcileInstances compares the initially started watcher for machines,
// units and applications with the opened and closed ports of the instances and
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
			fw.logger.Errorf("Machine not yet provisioned: %v", err)
			continue
		}
		if err != nil {
			return err
		}
		envInstances, err := fw.environInstances.Instances(fw.cloudCallContext, []instance.Id{instanceId})
		if err == environs.ErrNoInstances {
			return nil
		}
		if err != nil {
			return err
		}
		machineId := machined.tag.Id()

		fwInstance, ok := envInstances[0].(instances.InstanceFirewaller)
		if !ok {
			return nil
		}

		initialRules, err := fwInstance.IngressRules(fw.cloudCallContext, machineId)
		if err != nil {
			return err
		}

		// Check which ports to open or to close.
		toOpen, toClose := diffRanges(initialRules, machined.ingressRules)
		if len(toOpen) > 0 {
			fw.logger.Infof("opening instance port ranges %v for %q",
				toOpen, machined.tag)
			if err := fwInstance.OpenPorts(fw.cloudCallContext, machineId, toOpen); err != nil {
				// TODO(mue) Add local retry logic.
				return err
			}
		}
		if len(toClose) > 0 {
			fw.logger.Infof("closing instance port ranges %v for %q",
				toClose, machined.tag)
			if err := fwInstance.ClosePorts(fw.cloudCallContext, machineId, toClose); err != nil {
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
			if unit == nil || unit.Life() == life.Dead || machineTag != knownMachineTag {
				fw.forgetUnit(unitd)
				changed = append(changed, unitd)
				fw.logger.Debugf("stopped watching unit %s", name)
			}
		} else if unit != nil && unit.Life() != life.Dead && fw.machineds[machineTag] != nil {
			err = fw.startUnit(unit, machineTag)
			if params.IsCodeNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			changed = append(changed, fw.unitds[unitTag])
			fw.logger.Debugf("started watching %q", unitTag)
		}
	}
	if err := fw.flushUnits(changed); err != nil {
		return errors.Annotate(err, "cannot change firewall ports")
	}
	return nil
}

// openedPortsChanged handles port change notifications
func (fw *Firewaller) openedPortsChanged(machineTag names.MachineTag, subnetTag names.SubnetTag) (err error) {
	defer func() {
		if params.IsCodeNotFound(err) {
			err = nil
		}
	}()
	machined, ok := fw.machineds[machineTag]
	if !ok {
		// It is common to receive a port change notification before
		// registering the machine, so if a machine is not found in
		// firewaller's list, just skip the change.  Look up will also
		// fail if it's a manual machine.
		fw.logger.Debugf("failed to lookup %q, skipping port change", machineTag)
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
			fw.logger.Debugf("failed to lookup %q, skipping port change", unitTag)
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
				fw.logger.Debugf("no ingress rules for unknown %v on %v", unitTag, machined.tag)
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
				fw.logger.Debugf("CIDRS for %v: %v", unitTag, cidrs.Values())
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

// TODO(wallyworld) - consider making this configurable.
const maxAllowedCIDRS = 20

func (fw *Firewaller) updateForRemoteRelationIngress(appTag names.ApplicationTag, cidrs set.Strings) error {
	fw.logger.Debugf("finding egress rules for %v", appTag)
	// Now create the rules for any remote relations of which the
	// unit's application is a part.
	newCidrs := make(set.Strings)
	for _, data := range fw.relationIngress {
		if data.localApplicationTag != appTag {
			continue
		}
		if !data.ingressRequired {
			continue
		}
		for _, cidr := range data.networks.Values() {
			newCidrs.Add(cidr)
		}
	}
	// If we have too many CIDRs to create a rule for, consolidate.
	// If a firewall rule with a whitelist of CIDRs has been set up,
	// use that, else open to the world.
	if newCidrs.Size() > maxAllowedCIDRS {
		// First, try and merge the cidrs.
		merged, err := cidrman.MergeCIDRs(newCidrs.Values())
		if err != nil {
			return errors.Trace(err)
		}
		newCidrs = set.NewStrings(merged...)
	}

	// If there's still too many after merging, look for any firewall whitelist.
	if newCidrs.Size() > maxAllowedCIDRS {
		newCidrs = make(set.Strings)
		rules, err := fw.firewallerApi.FirewallRules("juju-application-offer")
		if err != nil {
			return errors.Trace(err)
		}
		if len(rules) > 0 {
			rule := rules[0]
			if len(rule.WhitelistCIDRS) > 0 {
				for _, cidr := range rule.WhitelistCIDRS {
					newCidrs.Add(cidr)
				}
			}
		}
		// No relevant firewall rule exists, so go public.
		if newCidrs.Size() == 0 {
			newCidrs.Add("0.0.0.0/0")
		}
	}
	for _, cidr := range newCidrs.Values() {
		cidrs.Add(cidr)
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
		if err := fw.environFirewaller.OpenPorts(fw.cloudCallContext, toOpen); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		network.SortIngressRules(toOpen)
		fw.logger.Infof("opened port ranges %v in environment", toOpen)
	}
	if len(toClose) > 0 {
		if err := fw.environFirewaller.ClosePorts(fw.cloudCallContext, toClose); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		network.SortIngressRules(toClose)
		fw.logger.Infof("closed port ranges %v in environment", toClose)
	}
	return nil
}

// flushInstancePorts opens and closes ports global on the machine.
func (fw *Firewaller) flushInstancePorts(machined *machineData, toOpen, toClose []network.IngressRule) (err error) {
	defer func() {
		if params.IsCodeNotFound(err) {
			err = nil
		}
	}()

	// If there's nothing to do, do nothing.
	// This is important because when a machine is first created,
	// it will have no instance id but also no open ports -
	// InstanceId will fail but we don't care.
	fw.logger.Debugf("flush instance ports: to open %v, to close %v", toOpen, toClose)
	if len(toOpen) == 0 && len(toClose) == 0 {
		return nil
	}
	m, err := machined.machine()
	if err != nil {
		return err
	}
	machineId := machined.tag.Id()
	instanceId, err := m.InstanceId()
	if params.IsCodeNotProvisioned(err) {
		// Not provisioned yet, so nothing to do for this instance
		return nil
	}
	if err != nil {
		return err
	}
	envInstances, err := fw.environInstances.Instances(fw.cloudCallContext, []instance.Id{instanceId})
	if err != nil {
		return err
	}
	fwInstance, ok := envInstances[0].(instances.InstanceFirewaller)
	if !ok {
		fw.logger.Infof("flushInstancePorts called on an instance of type %T which doesn't support firewall.", envInstances[0])
		return nil
	}

	// Open and close the ports.
	if len(toOpen) > 0 {
		if err := fwInstance.OpenPorts(fw.cloudCallContext, machineId, toOpen); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		network.SortIngressRules(toOpen)
		fw.logger.Infof("opened port ranges %v on %q", toOpen, machined.tag)
	}
	if len(toClose) > 0 {
		if err := fwInstance.ClosePorts(fw.cloudCallContext, machineId, toClose); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		network.SortIngressRules(toClose)
		fw.logger.Infof("closed port ranges %v on %q", toClose, machined.tag)
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
	dead := !found || m.Life() == life.Dead
	machined, known := fw.machineds[tag]
	if known && dead {
		return fw.forgetMachine(machined)
	}
	if !known && !dead {
		err := fw.startMachine(tag)
		if err != nil {
			return err
		}
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
	_ = worker.Stop(machined)
	delete(fw.machineds, machined.tag)
	fw.logger.Debugf("stopped watching %q", machined.tag)
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
			_ = worker.Stop(applicationd)
			stoppedApplication = true
		}
	}

	// Clean up after stopping.
	delete(fw.unitds, unitd.tag)
	delete(machined.unitds, unitd.tag)
	delete(applicationd.unitds, unitd.tag)
	fw.logger.Debugf("stopped watching %q", unitd.tag)
	if stoppedApplication {
		applicationTag := applicationd.application.Tag()
		delete(fw.applicationids, applicationTag)
		fw.logger.Debugf("stopped watching %q", applicationTag)
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
			change, err := ad.application.IsExposed()
			if err != nil {
				if errors.IsNotFound(err) {
					ad.fw.logger.Debugf("application(%q).IsExposed() returned NotFound: %v", ad.application.Name(), err)
					return nil
				}
				return errors.Trace(err)
			}
			if change == exposed {
				ad.fw.logger.Tracef("application(%q).IsExposed() == %v (unchanged)", ad.application.Name(), exposed)
				continue
			}
			ad.fw.logger.Tracef("application(%q).IsExposed() changed %v => %v", ad.application.Name(), exposed, change)

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
	portCidrs := func(rules []network.IngressRule) map[corenetwork.PortRange]set.Strings {
		result := make(map[corenetwork.PortRange]set.Strings)
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

	gone := notfound || rel.Life == life.Dead || rel.Suspended
	data, known := fw.relationIngress[tag]
	if known && gone {
		fw.logger.Debugf("relation %v was known but has died or been suspended", tag.Id())
		// If relation is suspended, shut off ingress immediately.
		// Units will also eventually leave scope which would cause
		// ingress to be shut off, but best to do it up front.
		if rel != nil && rel.Suspended {
			change := &remoteRelationNetworkChange{
				relationTag:         tag,
				localApplicationTag: data.localApplicationTag,
				ingressRequired:     false,
			}
			if err := fw.relationIngressChanged(change); err != nil {
				return errors.Trace(err)
			}
		}
		return fw.forgetRelation(data)
	}
	if !known && !gone {
		err := fw.startRelation(rel, rel.Endpoint.Role)
		if err != nil {
			return err
		}
	}
	return nil
}

type remoteRelationInfo struct {
	relationToken    string
	applicationToken string
}

type remoteRelationData struct {
	catacomb      catacomb.Catacomb
	fw            *Firewaller
	relationReady chan remoteRelationInfo

	tag                 names.RelationTag
	localApplicationTag names.ApplicationTag
	relationToken       string
	applicationToken    string
	remoteModelUUID     string
	endpointRole        charm.RelationRole
	isOffer             bool

	crossModelFirewallerFacade CrossModelFirewallerFacadeCloser

	// These values are updated when ingress information on the
	// relation changes in the model.
	ingressRequired bool
	networks        set.Strings
}

// startRelation creates a new data value for tracking details of the
// relation and starts watching the related models for subnets added or removed.
func (fw *Firewaller) startRelation(rel *params.RemoteRelation, role charm.RelationRole) error {
	remoteApps, err := fw.remoteRelationsApi.RemoteApplications([]string{rel.RemoteApplicationName})
	if err != nil {
		return errors.Trace(err)
	}
	remoteAppResult := remoteApps[0]
	if remoteAppResult.Error != nil {
		return errors.Trace(err)
	}

	tag := names.NewRelationTag(rel.Key)
	data := &remoteRelationData{
		fw:                  fw,
		tag:                 tag,
		remoteModelUUID:     rel.SourceModelUUID,
		localApplicationTag: names.NewApplicationTag(rel.ApplicationName),
		endpointRole:        role,
		relationReady:       make(chan remoteRelationInfo),
	}

	// Start the worker which will watch the remote relation for things like new networks.
	if err := fw.relationWorkerRunner.StartWorker(tag.Id(), func() (worker.Worker, error) {
		if err := catacomb.Invoke(catacomb.Plan{
			Site: &data.catacomb,
			Work: data.watchLoop,
		}); err != nil {
			return nil, errors.Trace(err)
		}
		return data, nil
	}); err != nil {
		return errors.Annotate(err, "error starting remote relation worker")
	}
	fw.relationIngress[tag] = data

	data.isOffer = remoteAppResult.Result.IsConsumerProxy
	return fw.startRelationPoller(rel.Key, rel.RemoteApplicationName, data.relationReady)
}

// watchLoop watches the relation for networks added or removed.
func (rd *remoteRelationData) watchLoop() error {
	defer func() {
		if rd.crossModelFirewallerFacade != nil {
			rd.crossModelFirewallerFacade.Close()
		}
	}()

	// First, wait for relation to become ready.
	for rd.relationToken == "" {
		select {
		case <-rd.catacomb.Dying():
			return rd.catacomb.ErrDying()
		case remoteRelationInfo := <-rd.relationReady:
			rd.relationToken = remoteRelationInfo.relationToken
			rd.applicationToken = remoteRelationInfo.applicationToken
			rd.fw.logger.Debugf(
				"relation %v for remote app %v in model %v is ready",
				rd.relationToken, rd.applicationToken, rd.remoteModelUUID)
		}
	}

	if rd.endpointRole == charm.RoleRequirer {
		return rd.requirerEndpointLoop()
	}
	return rd.providerEndpointLoop()
}

func (rd *remoteRelationData) requirerEndpointLoop() error {
	// If the requirer end of the relation is on the offering model,
	// there's nothing to do here because the provider end on the
	// consuming model will be watching for changes.
	// TODO(wallyworld) - this will change if we want to allow bidirectional traffic.
	if rd.isOffer {
		return nil
	}

	rd.fw.logger.Debugf("starting requirer endpoint loop for %v on %v ", rd.tag.Id(), rd.localApplicationTag.Id())
	// Now watch for updates to egress addresses so we can inform the offering
	// model what firewall ingress to allow.
	egressAddressWatcher, err := rd.fw.firewallerApi.WatchEgressAddressesForRelation(rd.tag)
	if err != nil {
		if !params.IsCodeNotFound(err) && !params.IsCodeNotSupported(err) {
			return errors.Trace(err)
		}
		rd.fw.logger.Infof("no egress required for %v", rd.localApplicationTag)
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
			rd.fw.logger.Debugf("relation egress addresses for %v changed in model %v: %v", rd.tag, rd.fw.modelUUID, cidrs)
			if err := rd.updateProviderModel(cidrs); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (rd *remoteRelationData) providerEndpointLoop() error {
	rd.fw.logger.Debugf("starting provider endpoint loop for %v on %v ", rd.tag.Id(), rd.localApplicationTag.Id())
	// Watch for ingress changes requested by the consuming model.
	ingressAddressWatcher, err := rd.ingressAddressWatcher()
	if err != nil {
		if !params.IsCodeNotFound(err) && !params.IsCodeNotSupported(err) {
			return errors.Trace(err)
		}
		rd.fw.logger.Infof("no ingress required for %v", rd.localApplicationTag)
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
			rd.fw.logger.Debugf("relation ingress addresses for %v changed in model %v: %v", rd.tag, rd.fw.modelUUID, cidrs)
			if err := rd.updateIngressNetworks(cidrs); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (rd *remoteRelationData) ingressAddressWatcher() (watcher.StringsWatcher, error) {
	if rd.isOffer {
		// On the offering side we watch the local model for ingress changes
		// which will have been published from the consuming model.
		return rd.fw.firewallerApi.WatchIngressAddressesForRelation(rd.tag)
	} else {
		// On the consuming side, if this is the provider end of the relation,
		// we watch the remote model's egress changes to get our ingress changes.
		apiInfo, err := rd.fw.firewallerApi.ControllerAPIInfoForModel(rd.remoteModelUUID)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot get api info for model %v", rd.remoteModelUUID)
		}
		rd.crossModelFirewallerFacade, err = rd.fw.newRemoteFirewallerAPIFunc(apiInfo)
		if err != nil {
			return nil, errors.Annotate(err, "cannot open facade to remote model to watch ingress addresses")
		}

		mac, err := rd.fw.firewallerApi.MacaroonForRelation(rd.tag.Id())
		if err != nil {
			return nil, errors.Annotatef(err, "cannot get macaroon for %v", rd.tag.Id())
		}
		arg := params.RemoteEntityArg{
			Token:     rd.relationToken,
			Macaroons: macaroon.Slice{mac},
		}
		return rd.crossModelFirewallerFacade.WatchEgressAddressesForRelation(arg)
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
func (rd *remoteRelationData) updateProviderModel(cidrs []string) error {
	rd.fw.logger.Debugf("ingress cidrs for %v: %+v", rd.tag, cidrs)
	change := &remoteRelationNetworkChange{
		relationTag:         rd.tag,
		localApplicationTag: rd.localApplicationTag,
		networks:            set.NewStrings(cidrs...),
		ingressRequired:     len(cidrs) > 0,
	}

	apiInfo, err := rd.fw.firewallerApi.ControllerAPIInfoForModel(rd.remoteModelUUID)
	if err != nil {
		return errors.Annotatef(err, "cannot get api info for model %v", rd.remoteModelUUID)
	}
	mac, err := rd.fw.firewallerApi.MacaroonForRelation(rd.tag.Id())
	if params.IsCodeNotFound(err) {
		// Relation has gone, nothing to do.
		return nil
	}
	if err != nil {
		return errors.Annotatef(err, "cannot get macaroon for %v", rd.tag.Id())
	}
	remoteModelAPI, err := rd.fw.newRemoteFirewallerAPIFunc(apiInfo)
	if err != nil {
		return errors.Annotate(err, "cannot open facade to remote model to publish network change")
	}
	defer remoteModelAPI.Close()
	event := params.IngressNetworksChangeEvent{
		RelationToken:    rd.relationToken,
		ApplicationToken: rd.applicationToken,
		Networks:         change.networks.Values(),
		IngressRequired:  change.ingressRequired,
		Macaroons:        macaroon.Slice{mac},
		BakeryVersion:    bakery.LatestVersion,
	}
	err = remoteModelAPI.PublishIngressNetworkChange(event)
	if errors.IsNotFound(err) {
		rd.fw.logger.Debugf("relation id not found publishing %+v", event)
		return nil
	}

	// If the requested ingress is not permitted on the offering side,
	// mark the relation as in error. It's not an error that requires a
	// worker restart though.
	if params.IsCodeForbidden(err) {
		return rd.fw.firewallerApi.SetRelationStatus(rd.tag.Id(), relation.Error, err.Error())
	}
	return errors.Annotate(err, "cannot publish ingress network change")
}

// updateIngressNetworks processes the changed ingress networks on the relation.
func (rd *remoteRelationData) updateIngressNetworks(cidrs []string) error {
	rd.fw.logger.Debugf("ingress cidrs for %v: %+v", rd.tag, cidrs)
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
	fw.logger.Debugf("forget relation %v", data.tag.Id())
	delete(fw.relationIngress, data.tag)
	// There's not much we can do if there's an error stopping the remote
	// relation worker, so just log it.
	if err := fw.relationWorkerRunner.StopWorker(data.tag.Id()); err != nil {
		fw.logger.Errorf("error stopping remote relation worker for %s: %v", data.tag, err)
	}
	fw.logger.Debugf("stopped watching %q", data.tag)
	return nil
}

type remoteRelationPoller struct {
	catacomb       catacomb.Catacomb
	fw             *Firewaller
	relationTag    names.RelationTag
	applicationTag names.ApplicationTag
	relationReady  chan remoteRelationInfo
}

// startRelationPoller creates a new worker which waits until a remote
// relation is registered in both models.
func (fw *Firewaller) startRelationPoller(relationKey, remoteAppName string, relationReady chan remoteRelationInfo) error {
	poller := &remoteRelationPoller{
		fw:             fw,
		relationTag:    names.NewRelationTag(relationKey),
		applicationTag: names.NewApplicationTag(remoteAppName),
		relationReady:  relationReady,
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
	p.fw.logger.Debugf("polling for relation %v on %v to be ready", p.relationTag, p.applicationTag)
	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()
		case <-p.fw.pollClock.After(3 * time.Second):
			// Relation is exported with the consuming model UUID.
			relToken, err := p.fw.remoteRelationsApi.GetToken(p.relationTag)
			if err != nil {
				continue
			}
			p.fw.logger.Debugf("token %v for relation id: %v in model %v", relToken, p.relationTag.Id(), p.fw.modelUUID)

			// Application is exported with the offering model UUID.
			appToken, err := p.fw.remoteRelationsApi.GetToken(p.applicationTag)
			if err != nil {
				continue
			}
			p.fw.logger.Debugf("token %v for application id: %v", appToken, p.applicationTag.Id())

			// relation and application are ready.
			relationInfo := remoteRelationInfo{
				relationToken:    relToken,
				applicationToken: appToken,
			}
			select {
			case <-p.catacomb.Dying():
				return p.catacomb.ErrDying()
			case p.relationReady <- relationInfo:
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
