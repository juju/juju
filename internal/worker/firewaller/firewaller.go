// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"
	"sort"
	"time"

	"github.com/EvilSuperstars/go-cidrman"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainrelation "github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/charm"
	internalerrors "github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/rpc/params"
)

type newCrossModelFacadeFunc func(context.Context, *api.Info) (CrossModelFirewallerFacadeCloser, error)

// Config defines the operation of a Worker.
type Config struct {
	ModelUUID                 string
	Mode                      string
	FirewallerAPI             FirewallerAPI
	CrossModelRelationService CrossModelRelationService
	PortsService              PortService
	MachineService            MachineService
	ApplicationService        ApplicationService
	RelationService           RelationService
	EnvironFirewaller         EnvironFirewaller
	EnvironModelFirewaller    EnvironModelFirewaller
	EnvironInstances          EnvironInstances
	EnvironIPV6CIDRSupport    bool

	NewCrossModelFacadeFunc newCrossModelFacadeFunc

	Clock  clock.Clock
	Logger logger.Logger

	// These are used to coordinate gomock tests.

	// WatchMachineNotify is called when the Firewaller starts watching the
	// machine with the given tag (manual machines aren't watched). This
	// should only be used for testing.
	WatchMachineNotify func(machine.Name)
	// FlushModelNotify is called when the Firewaller flushes it's model.
	// This should only be used for testing.
	FlushModelNotify func()
	// SkipFlushModelNotify is called when the Firewaller skips flushing a model.
	// This should only be used for testing.
	SkipFlushModelNotify func()
	// FlushMachineNotify is called when the Firewaller flushes a machine.
	// This should only be used for testing.
	FlushMachineNotify func(machine.Name)
}

// Validate returns an error if cfg cannot drive a Worker.
func (cfg Config) Validate() error {
	if cfg.ModelUUID == "" {
		return errors.NotValidf("empty model uuid")
	}
	if cfg.FirewallerAPI == nil {
		return errors.NotValidf("nil Firewaller Facade")
	}
	if cfg.CrossModelRelationService == nil {
		return errors.NotValidf("nil crossmodelrelation service")
	}
	if cfg.PortsService == nil {
		return errors.NotValidf("nil PortsService")
	}
	if cfg.MachineService == nil {
		return errors.NotValidf("nil MachineService")
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
	return nil
}

// Firewaller watches the state for port ranges opened or closed on
// machines and reflects those changes onto the backing environment.
// Uses Firewaller API V1.
type Firewaller struct {
	catacomb                  catacomb.Catacomb
	firewallerApi             FirewallerAPI
	crossModelRelationService CrossModelRelationService
	portService               PortService
	machineService            MachineService
	applicationService        ApplicationService
	relationService           RelationService
	environFirewaller         EnvironFirewaller
	environModelFirewaller    EnvironModelFirewaller
	environInstances          EnvironInstances

	machinesWatcher      watcher.StringsWatcher
	portsWatcher         watcher.StringsWatcher
	subnetWatcher        watcher.StringsWatcher
	modelFirewallWatcher watcher.NotifyWatcher
	// Watch the remote relations, only emits in the consuming model.
	consumerRelationsWatcher watcher.StringsWatcher
	// Watch the remote relations, only emits in the offering model.
	offererRelationsWatcher watcher.StringsWatcher

	machineds            map[machine.Name]*machineData
	unitsChange          chan *unitsChange
	unitds               map[coreunit.Name]*unitData
	applicationids       map[names.ApplicationTag]*applicationData
	exposedChange        chan *exposedChange
	spaceInfos           network.SpaceInfos
	globalMode           bool
	globalIngressRuleRef map[string]int // map of rule names to count of occurrences

	// Set to true if the environment supports ingress rules containing
	// IPV6 CIDRs.
	envIPV6CIDRSupport bool
	needsToFlushModel  bool

	modelUUID                  string
	newRemoteFirewallerAPIFunc newCrossModelFacadeFunc
	localRelationsChange       chan *remoteRelationNetworkChange
	relationIngress            map[names.RelationTag]*remoteRelationData
	relationWorkerRunner       *worker.Runner
	clk                        clock.Clock
	logger                     logger.Logger

	// Only used for testing
	watchMachineNotify   func(machine.Name)
	flushModelNotify     func()
	skipFlushModelNotify func()
	flushMachineNotify   func(machine.Name)
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

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:   "firewaller",
		Clock:  clk,
		Logger: internalworker.WrapLogger(cfg.Logger),

		// One of the remote relation workers failing should not
		// prevent the others from running.
		IsFatal: func(error) bool { return false },

		// For any failures, try again in 1 minute.
		RestartDelay: time.Minute,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	fw := &Firewaller{
		firewallerApi:              cfg.FirewallerAPI,
		crossModelRelationService:  cfg.CrossModelRelationService,
		portService:                cfg.PortsService,
		machineService:             cfg.MachineService,
		applicationService:         cfg.ApplicationService,
		relationService:            cfg.RelationService,
		environFirewaller:          cfg.EnvironFirewaller,
		environModelFirewaller:     cfg.EnvironModelFirewaller,
		environInstances:           cfg.EnvironInstances,
		envIPV6CIDRSupport:         cfg.EnvironIPV6CIDRSupport,
		newRemoteFirewallerAPIFunc: cfg.NewCrossModelFacadeFunc,
		modelUUID:                  cfg.ModelUUID,
		machineds:                  make(map[machine.Name]*machineData),
		unitsChange:                make(chan *unitsChange),
		unitds:                     make(map[coreunit.Name]*unitData),
		applicationids:             make(map[names.ApplicationTag]*applicationData),
		exposedChange:              make(chan *exposedChange),
		relationIngress:            make(map[names.RelationTag]*remoteRelationData),
		localRelationsChange:       make(chan *remoteRelationNetworkChange),
		clk:                        clk,
		logger:                     cfg.Logger,
		relationWorkerRunner:       runner,
		watchMachineNotify:         cfg.WatchMachineNotify,
		flushModelNotify:           cfg.FlushModelNotify,
		skipFlushModelNotify:       cfg.SkipFlushModelNotify,
		flushMachineNotify:         cfg.FlushMachineNotify,
	}

	switch cfg.Mode {
	case config.FwInstance:
	case config.FwGlobal:
		fw.globalMode = true
		fw.globalIngressRuleRef = make(map[string]int)
	default:
		return nil, errors.Errorf("invalid firewall-mode %q", cfg.Mode)
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "firewaller",
		Site: &fw.catacomb,
		Work: fw.loop,
		Init: []worker.Worker{fw.relationWorkerRunner},
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return fw, nil
}

func (fw *Firewaller) setUp(ctx context.Context) error {
	var err error
	fw.machinesWatcher, err = fw.firewallerApi.WatchModelMachines(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := fw.catacomb.Add(fw.machinesWatcher); err != nil {
		return errors.Trace(err)
	}

	fw.portsWatcher, err = fw.portService.WatchMachineOpenedPorts(ctx)
	if err != nil {
		return errors.Annotatef(err, "failed to start ports watcher")
	}
	if err := fw.catacomb.Add(fw.portsWatcher); err != nil {
		return errors.Trace(err)
	}

	// Remote relations on the consuming model.
	fw.consumerRelationsWatcher, err = fw.crossModelRelationService.WatchConsumerRelations(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := fw.catacomb.Add(fw.consumerRelationsWatcher); err != nil {
		return errors.Trace(err)
	}
	// Remote relations on the offering model.
	fw.offererRelationsWatcher, err = fw.crossModelRelationService.WatchOffererRelations(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := fw.catacomb.Add(fw.offererRelationsWatcher); err != nil {
		return errors.Trace(err)
	}

	fw.subnetWatcher, err = fw.firewallerApi.WatchSubnets(ctx)
	if err != nil {
		return errors.Annotatef(err, "failed to start subnet watcher")
	}
	if err := fw.catacomb.Add(fw.subnetWatcher); err != nil {
		return errors.Trace(err)
	}

	if fw.environModelFirewaller != nil {
		fw.modelFirewallWatcher, err = fw.firewallerApi.WatchModelFirewallRules(ctx)
		if err != nil {
			return errors.Annotatef(err, "failed to start subnet watcher")
		}
		if err := fw.catacomb.Add(fw.modelFirewallWatcher); err != nil {
			return errors.Trace(err)
		}
	}

	if fw.spaceInfos, err = fw.firewallerApi.AllSpaceInfos(ctx); err != nil {
		return errors.Trace(err)
	}

	fw.logger.Debugf(ctx, "started watching opened port ranges for the model")
	return nil
}

func (fw *Firewaller) loop() error {
	ctx, cancel := fw.scopedContext()
	defer cancel()

	if err := fw.setUp(ctx); err != nil {
		return errors.Trace(err)
	}
	var (
		reconciled                    bool
		modelGroupInitiallyConfigured bool
	)
	portsChange := fw.portsWatcher.Changes()

	var modelFirewallChanges watcher.NotifyChannel
	var ensureModelFirewalls <-chan time.Time
	if fw.modelFirewallWatcher != nil {
		modelFirewallChanges = fw.modelFirewallWatcher.Changes()
	}

	for {
		select {
		case <-fw.catacomb.Dying():
			return fw.catacomb.ErrDying()
		case <-ensureModelFirewalls:
			err := fw.flushModel(ctx)
			if errors.Is(err, errors.NotFound) {
				ensureModelFirewalls = fw.clk.After(time.Second)
			} else if err != nil {
				return err
			} else {
				ensureModelFirewalls = nil
			}
		case _, ok := <-modelFirewallChanges:
			if !ok {
				return errors.New("model config watcher closed")
			}
			if ensureModelFirewalls == nil {
				ensureModelFirewalls = fw.clk.After(0)
			}
		case change, ok := <-fw.machinesWatcher.Changes():
			if !ok {
				return errors.New("machines watcher closed")
			}
			for _, machineId := range change {
				if err := fw.machineLifeChanged(ctx, machine.Name(machineId)); err != nil {
					return err
				}
			}
			if !reconciled {
				reconciled = true
				var err error
				if fw.globalMode {
					err = fw.reconcileGlobal(ctx)
				} else {
					err = fw.reconcileInstances(ctx)
				}
				if err != nil {
					return errors.Trace(err)
				}
			}
			// After first machine exists, make sure to trigger the model firewall flush.
			if len(change) > 0 && !modelGroupInitiallyConfigured {
				modelGroupInitiallyConfigured = true
				if ensureModelFirewalls == nil {
					ensureModelFirewalls = fw.clk.After(0)
				}
			}
		case change, ok := <-portsChange:
			if !ok {
				return errors.New("ports watcher closed")
			}
			for _, portsGlobalKey := range change {
				if err := fw.openedPortsChanged(ctx, machine.Name(portsGlobalKey)); err != nil {
					return errors.Trace(err)
				}
			}
		case _, ok := <-fw.subnetWatcher.Changes():
			if !ok {
				return errors.New("subnet watcher closed")
			}

			if err := fw.subnetsChanged(ctx); err != nil {
				return errors.Trace(err)
			}
		case change := <-fw.unitsChange:
			if err := fw.unitsChanged(ctx, change); err != nil {
				return errors.Trace(err)
			}
		case change := <-fw.exposedChange:
			change.applicationd.exposed = change.exposed
			change.applicationd.exposedEndpoints = change.exposedEndpoints
			var unitds []*unitData
			for _, unitd := range change.applicationd.unitds {
				unitds = append(unitds, unitd)
			}
			if err := fw.flushUnits(ctx, unitds); err != nil {
				return errors.Annotate(err, "cannot change firewall ports")
			}

		case change, ok := <-fw.consumerRelationsWatcher.Changes():
			if !ok {
				return errors.New("consumer relations watcher closed")
			}
			for _, relationUUID := range change {
				if err := fw.consumerRelationLifeChanged(ctx, relationUUID); err != nil {
					return err
				}
			}
		case change, ok := <-fw.offererRelationsWatcher.Changes():
			if !ok {
				return errors.New("offerer relations watcher closed")
			}
			for _, relationUUID := range change {
				if err := fw.offererRelationLifeChanged(ctx, relationUUID); err != nil {
					return err
				}
			}
		case change := <-fw.localRelationsChange:
			// We have a notification that the remote (consuming) model
			// has changed egress networks so need to update the local
			// model to allow those networks through the firewall.
			if err := fw.relationIngressChanged(ctx, change); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (fw *Firewaller) subnetsChanged(ctx context.Context) error {
	// Refresh space topology
	var err error
	if fw.spaceInfos, err = fw.firewallerApi.AllSpaceInfos(ctx); err != nil {
		return errors.Trace(err)
	}

	// Select units for which the ingress rules must be refreshed. We only
	// consider applications that expose endpoints to at least one space.
	var unitds []*unitData
	for _, appd := range fw.applicationids {
		var exposedToSpaces bool
		for _, exposeDetails := range appd.exposedEndpoints {
			if exposeDetails.ExposeToSpaceIDs.Size() != 0 {
				exposedToSpaces = true
				break
			}
		}

		if !exposedToSpaces {
			continue // no need to re-eval ingress rules.
		}

		for _, unitd := range appd.unitds {
			unitds = append(unitds, unitd)
		}
	}

	if len(unitds) == 0 {
		return nil // nothing to do
	}

	fw.logger.Debugf(ctx, "updating %d units after changes in subnets", len(unitds))
	if err := fw.flushUnits(ctx, unitds); err != nil {
		return errors.Annotate(err, "cannot update unit ingress rules")
	}
	return nil
}

func (fw *Firewaller) relationIngressChanged(ctx context.Context, change *remoteRelationNetworkChange) error {
	fw.logger.Debugf(ctx, "process remote relation ingress change for %v", change.relationTag)
	relData, ok := fw.relationIngress[change.relationTag]
	if ok {
		relData.networks = change.networks
		relData.ingressRequired = change.ingressRequired
	}
	appData, ok := fw.applicationids[change.localApplicationTag]
	if !ok {
		fw.logger.Debugf(ctx, "ignoring unknown application: %v", change.localApplicationTag)
		return nil
	}
	unitds := []*unitData{}
	for _, unitd := range appData.unitds {
		unitds = append(unitds, unitd)
	}
	if err := fw.flushUnits(ctx, unitds); err != nil {
		return errors.Annotate(err, "cannot change firewall ports")
	}
	return nil
}

// startMachine creates a new data value for tracking details of the
// machine and starts watching the machine for units added or removed.
func (fw *Firewaller) startMachine(ctx context.Context, machineName machine.Name) error {
	machined := &machineData{
		fw:     fw,
		name:   machineName,
		unitds: make(map[coreunit.Name]*unitData),
	}
	m, err := machined.machine(ctx)
	if params.IsCodeNotFound(err) {
		fw.logger.Debugf(ctx, "not watching %q", machineName)
		return nil
	} else if err != nil {
		return errors.Annotate(err, "cannot watch machine units")
	}
	manual, err := m.IsManual(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if manual {
		// Don't track manual machines, we can't change their ports.
		fw.logger.Debugf(ctx, "not watching manual %q", machineName)
		return nil
	}
	unitw, err := fw.applicationService.WatchUnitAddRemoveOnMachine(ctx, machineName)
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
		unitNames, err := transform.SliceOrErr(change, coreunit.NewName)
		if err != nil {
			return err
		}
		fw.machineds[machineName] = machined
		err = fw.unitsChanged(ctx, &unitsChange{machined: machined, units: unitNames})
		if err != nil {
			delete(fw.machineds, machineName)
			return errors.Annotatef(err, "cannot respond to units changes for %q, %q", machineName, fw.modelUUID)
		}
	}

	err = catacomb.Invoke(catacomb.Plan{
		Name: "firewaller-machine",
		Site: &machined.catacomb,
		Work: func() error {
			return machined.watchLoop(unitw)
		},
	})
	if err != nil {
		delete(fw.machineds, machineName)
		return errors.Trace(err)
	}

	// register the machined with the firewaller's catacomb.
	err = fw.catacomb.Add(machined)
	if err != nil {
		return errors.Trace(err)
	}
	fw.logger.Debugf(ctx, "started watching %q", machineName)
	if fw.watchMachineNotify != nil {
		fw.watchMachineNotify(machineName)
	}
	return nil
}

// startUnit creates a new data value for tracking details of the unit
// The provided machineTag must be the tag for the machine the unit was last
// observed to be assigned to.
func (fw *Firewaller) startUnit(ctx context.Context, unit Unit, machineName machine.Name) error {
	application, err := unit.Application()
	if err != nil {
		return err
	}

	applicationTag := application.Tag()
	unitName, err := coreunit.NewName(unit.Name())
	if err != nil {
		return err
	}
	unitd := &unitData{
		fw:   fw,
		unit: unit,
		name: unitName,
	}
	fw.unitds[unitName] = unitd

	unitd.machined = fw.machineds[machineName]
	unitd.machined.unitds[unitName] = unitd
	if fw.applicationids[applicationTag] == nil {
		err := fw.startApplication(ctx, application)
		if err != nil {
			delete(fw.unitds, unitName)
			delete(unitd.machined.unitds, unitName)
			return err
		}
	}
	unitd.applicationd = fw.applicationids[applicationTag]
	unitd.applicationd.unitds[unitName] = unitd

	if err = fw.openedPortsChanged(ctx, machineName); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// startApplication creates a new data value for tracking details of the
// application and starts watching the application for exposure changes.
func (fw *Firewaller) startApplication(ctx context.Context, app Application) error {
	isExposed, err := fw.applicationService.IsApplicationExposed(ctx, app.Name())
	if err != nil {
		return internalerrors.Capture(err)
	}
	exposedEndpoints, err := fw.applicationService.GetExposedEndpoints(ctx, app.Name())
	if err != nil {
		return internalerrors.Capture(err)
	}
	applicationd := &applicationData{
		fw:                 fw,
		applicationTag:     app.Tag(),
		applicationService: fw.applicationService,
		exposed:            isExposed,
		exposedEndpoints:   exposedEndpoints,
		unitds:             make(map[coreunit.Name]*unitData),
	}
	fw.applicationids[app.Tag()] = applicationd

	err = catacomb.Invoke(catacomb.Plan{
		Name: "firewaller-application",
		Site: &applicationd.catacomb,
		Work: func() error {
			return applicationd.watchLoop(isExposed, exposedEndpoints)
		},
	})
	if err != nil {
		return internalerrors.Capture(err)
	}
	if err := fw.catacomb.Add(applicationd); err != nil {
		return internalerrors.Capture(err)
	}
	return nil
}

// reconcileGlobal compares the initially started watcher for machines,
// units and applications with the opened and closed ports globally and
// opens and closes the appropriate ports for the whole environment.
func (fw *Firewaller) reconcileGlobal(ctx context.Context) error {
	var machines []*machineData
	for _, machined := range fw.machineds {
		machines = append(machines, machined)
	}
	want, err := fw.gatherIngressRules(ctx, machines...)
	if err != nil {
		return err
	}
	initialPortRanges, err := fw.environFirewaller.IngressRules(ctx)
	if err != nil {
		return err
	}

	// Check which ports to open or to close.
	toOpen, toClose := initialPortRanges.Diff(want)
	if len(toOpen) > 0 {
		fw.logger.Infof(ctx, "opening global ports %v", toOpen)
		if err := fw.environFirewaller.OpenPorts(ctx, toOpen); err != nil {
			return errors.Annotatef(err, "failed to open global ports %v", toOpen)
		}
	}
	if len(toClose) > 0 {
		fw.logger.Infof(ctx, "closing global ports %v", toClose)
		if err := fw.environFirewaller.ClosePorts(ctx, toClose); err != nil {
			return errors.Annotatef(err, "failed to close global ports %v", toClose)
		}
	}
	return nil
}

// reconcileInstances compares the initially started watcher for machines,
// units and applications with the opened and closed ports of the instances and
// opens and closes the appropriate ports for each instance.
func (fw *Firewaller) reconcileInstances(ctx context.Context) error {
	for _, machined := range fw.machineds {
		m, err := machined.machine(ctx)
		if params.IsCodeNotFound(err) {
			if err := fw.forgetMachine(ctx, machined); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		instanceId, err := m.InstanceId(ctx)
		if errors.Is(err, errors.NotProvisioned) {
			fw.logger.Errorf(ctx, "Machine not yet provisioned: %v", err)
			continue
		}
		if err != nil {
			return err
		}
		ctx := context.Background()
		envInstances, err := fw.environInstances.Instances(ctx, []instance.Id{instanceId})
		if err == environs.ErrNoInstances {
			return nil
		}
		if err != nil {
			return err
		}
		machineName := machined.name

		if len(envInstances) == 0 {
			fw.logger.Errorf(ctx, "Instance not found for machine %v", machineName)
			return nil
		}

		fwInstance, ok := envInstances[0].(instances.InstanceFirewaller)
		if !ok {
			return nil
		}

		initialRules, err := fwInstance.IngressRules(ctx, machineName.String())
		if err != nil {
			return err
		}

		// Check which ports to open or to close.
		toOpen, toClose := initialRules.Diff(machined.ingressRules)
		if len(toOpen) > 0 {
			fw.logger.Infof(ctx, "opening instance port ranges %v for %q",
				toOpen, machineName)
			if err := fwInstance.OpenPorts(ctx, machineName.String(), toOpen); err != nil {
				// TODO(mue) Add local retry logic.
				return errors.Annotatef(err, "failed to open instance ports %v for %q", toOpen, machineName)
			}
		}
		if len(toClose) > 0 {
			fw.logger.Infof(ctx, "closing instance port ranges %v for %q",
				toClose, machineName)
			if err := fwInstance.ClosePorts(ctx, machineName.String(), toClose); err != nil {
				// TODO(mue) Add local retry logic.
				return errors.Annotatef(err, "failed to close instance ports %v for %q", toOpen, machineName)
			}
		}
	}
	return nil
}

// unitsChanged responds to changes to the assigned units.
func (fw *Firewaller) unitsChanged(ctx context.Context, change *unitsChange) error {
	changed := []*unitData{}
	for _, unitName := range change.units {
		machineName, err := fw.applicationService.GetUnitMachineName(ctx, unitName)
		if errors.Is(err, applicationerrors.UnitIsDead) || errors.Is(err, applicationerrors.UnitNotFound) {
			continue
		} else if err != nil && !errors.Is(err, applicationerrors.UnitMachineNotAssigned) {
			return errors.Trace(err)
		}

		unit, err := fw.firewallerApi.Unit(ctx, names.NewUnitTag(unitName.String()))
		if err != nil && !params.IsCodeNotFound(err) {
			return err
		}
		if unitd, known := fw.unitds[unitName]; known {
			knownMachineName := fw.unitds[unitName].machined.name
			if unit == nil || unit.Life() == life.Dead || machineName != knownMachineName {
				fw.forgetUnit(ctx, unitd)
				changed = append(changed, unitd)
				fw.logger.Debugf(ctx, "stopped watching unit %s", unitName)
			}
		} else if unit != nil && unit.Life() != life.Dead && fw.machineds[machineName] != nil {
			err = fw.startUnit(ctx, unit, machineName)
			if params.IsCodeNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			changed = append(changed, fw.unitds[unitName])
			fw.logger.Debugf(ctx, "started watching %q", unitName)
		}
	}
	if err := fw.flushUnits(ctx, changed); err != nil {
		return errors.Annotate(err, "cannot change firewall ports")
	}
	return nil
}

// openedPortsChanged handles port change notifications
func (fw *Firewaller) openedPortsChanged(ctx context.Context, machineName machine.Name) (err error) {
	defer func() {
		if params.IsCodeNotFound(err) {
			err = nil
		}
	}()
	machined, ok := fw.machineds[machineName]
	if !ok {
		// It is common to receive a port change notification before
		// registering the machine, so if a machine is not found in
		// firewaller's list, just skip the change.  Look up will also
		// fail if it's a manual machine.
		fw.logger.Debugf(ctx, "failed to lookup machine %q, skipping port change", machineName)
		return nil
	}

	machineUUID, err := fw.machineService.GetMachineUUID(ctx, machineName)
	if err != nil {
		return err
	}

	openedPortRangesByEndpoint, err := fw.portService.GetMachineOpenedPorts(ctx, machineUUID.String())
	if err != nil {
		return err
	}

	// Check for missing units and defer the handling of this change for
	// the future.
	for unitName := range openedPortRangesByEndpoint {
		if _, ok := machined.unitds[unitName]; !ok {
			// It is common to receive port change notification before
			// registering a unit. Skip handling the port change - it will
			// be handled when the unit is registered.
			fw.logger.Debugf(ctx, "machine %v has units: %+v", machineName, machined.unitds)
			fw.logger.Debugf(ctx, "failed to lookup unit %q, skipping port change", unitName)
			return nil
		}
	}

	if equalGroupedPortRanges(machined.openedPortRangesByEndpoint, openedPortRangesByEndpoint) {
		return nil // no change
	}

	machined.openedPortRangesByEndpoint = openedPortRangesByEndpoint
	return fw.flushMachine(ctx, machined)
}

func equalGroupedPortRanges(a, b map[coreunit.Name]network.GroupedPortRanges) bool {
	if len(a) != len(b) {
		return false
	}
	for unitTag, portRangesA := range a {
		portRangesB, exists := b[unitTag]
		if !exists || !portRangesA.EqualTo(portRangesB) {
			return false
		}
	}
	return true
}

// flushUnits opens and closes ports for the passed unit data.
func (fw *Firewaller) flushUnits(ctx context.Context, unitds []*unitData) error {
	machineds := map[machine.Name]*machineData{}
	for _, unitd := range unitds {
		machineds[unitd.machined.name] = unitd.machined
	}
	for _, machined := range machineds {
		if err := fw.flushMachine(ctx, machined); err != nil {
			return err
		}
	}
	return nil
}

// flushMachine opens and closes ports for the passed machine.
func (fw *Firewaller) flushMachine(ctx context.Context, machined *machineData) error {
	defer func() {
		if fw.flushMachineNotify != nil {
			fw.flushMachineNotify(machined.name)
		}
	}()
	// We may have received a notification to flushModel() in the past but did not have any machines yet.
	// Call flushModel() now.
	if fw.needsToFlushModel {
		if err := fw.flushModel(ctx); err != nil {
			return errors.Trace(err)
		}
	}
	want, err := fw.gatherIngressRules(ctx, machined)
	if err != nil {
		return errors.Trace(err)
	}
	toOpen, toClose := machined.ingressRules.Diff(want)
	machined.ingressRules = want
	if fw.globalMode {
		return fw.flushGlobalPorts(toOpen, toClose)
	}
	return fw.flushInstancePorts(ctx, machined, toOpen, toClose)
}

// gatherIngressRules returns the ingress rules to open and close
// for the specified machines.
func (fw *Firewaller) gatherIngressRules(ctx context.Context, machines ...*machineData) (firewall.IngressRules, error) {
	var want firewall.IngressRules
	for _, machined := range machines {
		for unitTag := range machined.openedPortRangesByEndpoint {
			unitd, known := machined.unitds[unitTag]
			if !known {
				fw.logger.Debugf(ctx, "no ingress rules for unknown %v on %v", unitTag, machined.name)
				continue
			}

			unitRules, err := fw.ingressRulesForMachineUnit(ctx, machined, unitd)
			if err != nil {
				return nil, errors.Trace(err)
			}

			want = append(want, unitRules...)
		}
	}
	if err := want.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	// Substrates that do not support IPV6 CIDRs will complain if we pass
	// an IPV6 CIDR. To work around this issue, we filter out any IPV6
	// CIDRs from the collected ingress rule list.
	if !fw.envIPV6CIDRSupport {
		want = want.RemoveCIDRsMatchingAddressType(network.IPv6Address)
	}

	return want, nil
}

func (fw *Firewaller) ingressRulesForMachineUnit(ctx context.Context, machine *machineData, unit *unitData) (firewall.IngressRules, error) {
	unitPortRanges := machine.openedPortRangesByEndpoint[unit.name]
	if len(unitPortRanges) == 0 {
		return nil, nil // no ports opened by the charm
	}

	var rules firewall.IngressRules
	var err error
	if unit.applicationd.exposed {
		rules = fw.ingressRulesForExposedMachineUnit(ctx, unit, unitPortRanges)
	} else {
		if rules, err = fw.ingressRulesForNonExposedMachineUnit(
			ctx,
			unit.applicationd.applicationTag,
			unitPortRanges,
		); err != nil {
			return nil, errors.Trace(err)
		}
	}

	// De-dup and sort rules before returning them back.
	rules = rules.UniqueRules()
	sort.Slice(rules, func(i, j int) bool { return rules[i].LessThan(rules[j]) })
	fw.logger.Debugf(ctx, "ingress rules for %q: %v", unit.name, rules)
	return rules, nil
}

func (fw *Firewaller) ingressRulesForNonExposedMachineUnit(ctx context.Context,
	appTag names.ApplicationTag,
	openUnitPortRanges network.GroupedPortRanges) (firewall.IngressRules, error) {
	// Not exposed, so add any ingress rules required by remote relations.
	srcCIDRs, err := fw.updateForRemoteRelationIngress(ctx, appTag)
	if err != nil || len(srcCIDRs) == 0 {
		return nil, errors.Trace(err)
	}

	var rules firewall.IngressRules
	for _, portRange := range openUnitPortRanges.UniquePortRanges() {
		rules = append(rules, firewall.NewIngressRule(portRange, srcCIDRs.Values()...))
	}

	return rules, nil
}

func (fw *Firewaller) appendSubnetCIDRsFromExposedSpaces(ctx context.Context, unit *unitData, exposedEndpoint string, exposeDetails *application.ExposedEndpoint) {
	// Collect the operator-provided CIDRs that should be able to
	// access the port ranges opened for this endpoint; then resolve
	// the CIDRs for the spaces specified in the expose details to
	// construct the full source CIDR list for the generated rules.
	for _, spaceID := range exposeDetails.ExposeToSpaceIDs.Values() {
		sp := fw.spaceInfos.GetByID(network.SpaceUUID(spaceID))
		if sp == nil {
			fw.logger.Warningf(ctx, "exposed endpoint references unknown space ID %q", spaceID)
			continue
		}

		if len(sp.Subnets) == 0 {
			if exposedEndpoint == "" {
				fw.logger.Warningf(ctx, "all endpoints of application %q are exposed to space %q which contains no subnets",
					unit.applicationd.applicationTag.Name, sp.Name)
			} else {
				fw.logger.Warningf(ctx, "endpoint %q application %q are exposed to space %q which contains no subnets",
					exposedEndpoint, unit.applicationd.applicationTag.Name, sp.Name)
			}
		}
		for _, subnet := range sp.Subnets {
			if exposeDetails.ExposeToCIDRs == nil {
				exposeDetails.ExposeToCIDRs = set.NewStrings(subnet.CIDR)
			} else {
				exposeDetails.ExposeToCIDRs.Add(subnet.CIDR)
			}
		}
	}
}

func (fw *Firewaller) ingressRulesForExposedMachineUnit(ctx context.Context, unit *unitData, openUnitPortRanges network.GroupedPortRanges) firewall.IngressRules {
	var (
		exposedEndpoints = unit.applicationd.exposedEndpoints
		rules            firewall.IngressRules
	)

	for exposedEndpoint, exposeDetails := range exposedEndpoints {
		fw.appendSubnetCIDRsFromExposedSpaces(ctx, unit, exposedEndpoint, &exposeDetails)

		if exposeDetails.ExposeToCIDRs.Size() == 0 {
			continue // no rules required
		}

		// If this is a named (i.e. not the wildcard) endpoint, look up
		// the port ranges opened for *all* endpoints as well as for
		// that endpoint name specifically, and create ingress rules.
		if exposedEndpoint != "" {
			for _, portRange := range openUnitPortRanges[exposedEndpoint] { // ports opened for this endpoint
				rules = append(rules, firewall.NewIngressRule(portRange, exposeDetails.ExposeToCIDRs.Values()...))
			}
			for _, portRange := range openUnitPortRanges[""] { // ports opened for ALL endpoints
				rules = append(rules, firewall.NewIngressRule(portRange, exposeDetails.ExposeToCIDRs.Values()...))
			}
			continue
		}

		// Create ingress rules for all endpoints except the ones that
		// have their own dedicated entry in the exposed endpoints map.
		for endpointName, portRanges := range openUnitPortRanges {
			// This non-wildcard endpoint has an entry in the exposed
			// endpoints list that override the global expose-all
			// entry so we should skip it.
			if _, hasExposeOverride := exposedEndpoints[endpointName]; hasExposeOverride && endpointName != "" {
				continue
			}

			for _, portRange := range portRanges {
				rules = append(rules, firewall.NewIngressRule(portRange, exposeDetails.ExposeToCIDRs.Values()...))
			}
		}
	}

	return rules
}

// TODO(wallyworld) - consider making this configurable.
const maxAllowedCIDRS = 20

func (fw *Firewaller) updateForRemoteRelationIngress(ctx context.Context, appTag names.ApplicationTag) (set.Strings, error) {
	fw.logger.Debugf(ctx, "finding egress rules for %v", appTag)
	// Now create the rules for any remote relations of which the
	// unit's application is a part.
	cidrs := make(set.Strings)
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
	// If we have too many CIDRs to create a rule for, consolidate.
	// If a firewall rule with a whitelist of CIDRs has been set up,
	// use that, else open to the world.
	if cidrs.Size() > maxAllowedCIDRS {
		// First, try and merge the cidrs.
		merged, err := cidrman.MergeCIDRs(cidrs.Values())
		if err != nil {
			return nil, errors.Trace(err)
		}
		cidrs = set.NewStrings(merged...)
	}

	// If there's still too many after merging, look for any firewall whitelist.
	if cidrs.Size() > maxAllowedCIDRS {
		cfg, err := fw.firewallerApi.ModelConfig(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		whitelistCidrs := cfg.SAASIngressAllow()
		cidrs = set.NewStrings(whitelistCidrs...)
	}

	return cidrs, nil
}

// flushGlobalPorts opens and closes global ports in the environment.
// It keeps a reference count for ports so that only 0-to-1 and 1-to-0 events
// modify the environment.
func (fw *Firewaller) flushGlobalPorts(rawOpen, rawClose firewall.IngressRules) error {
	// Filter which ports are really to open or close.
	var toOpen, toClose firewall.IngressRules
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
	ctx := context.Background()
	// Open and close the ports.
	if len(toOpen) > 0 {
		toOpen.Sort()
		fw.logger.Infof(ctx, "opening port ranges %v in environment", toOpen)
		if err := fw.environFirewaller.OpenPorts(ctx, toOpen); err != nil {
			// TODO(mue) Add local retry logic.
			return errors.Annotatef(err, "failed to open port ranges %v in environment", toOpen)
		}
	}
	if len(toClose) > 0 {
		toClose.Sort()
		fw.logger.Infof(ctx, "closing port ranges %v in environment", toClose)
		if err := fw.environFirewaller.ClosePorts(ctx, toClose); err != nil {
			// TODO(mue) Add local retry logic.
			return errors.Annotatef(err, "failed to close port ranges %v in environment", toOpen)
		}
	}
	return nil
}

func (fw *Firewaller) flushModel(ctx context.Context) error {
	if fw.environModelFirewaller == nil {
		return nil
	}
	// Model specific artefacts shouldn't be created until the model contains at least one machine.
	if len(fw.machineds) == 0 {
		fw.needsToFlushModel = true
		fw.logger.Debugf(ctx, "skipping flushing model because there are no machines for this model")
		if fw.skipFlushModelNotify != nil {
			fw.skipFlushModelNotify()
		}
		return nil
	}
	// Reset the flag because the models are being flushed now.
	fw.needsToFlushModel = false

	want, err := fw.firewallerApi.ModelFirewallRules(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// Substrates that do not support IPV6 CIDRs will complain if we pass
	// an IPV6 CIDR.
	if !fw.envIPV6CIDRSupport {
		want = want.RemoveCIDRsMatchingAddressType(network.IPv6Address)
	}
	curr, err := fw.environModelFirewaller.ModelIngressRules(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	toOpen, toClose := curr.Diff(want)
	if len(toOpen) > 0 {
		toOpen.Sort()
		fw.logger.Infof(ctx, "opening port ranges %v on model firewall", toOpen)
		if err := fw.environModelFirewaller.OpenModelPorts(ctx, toOpen); err != nil {
			// TODO(mue) Add local retry logic.
			return errors.Annotatef(err, "failed to open port ranges %v on model firewall", toOpen)
		}
	}
	if len(toClose) > 0 {
		toClose.Sort()
		fw.logger.Infof(ctx, "closing port ranges %v on model firewall", toClose)
		if err := fw.environModelFirewaller.CloseModelPorts(ctx, toClose); err != nil {
			// TODO(mue) Add local retry logic.
			return errors.Annotatef(err, "failed to close port ranges %v on model firewall", toOpen)
		}
	}
	if fw.flushModelNotify != nil {
		fw.flushModelNotify()
	}
	return nil
}

// flushInstancePorts opens and closes ports global on the machine.
func (fw *Firewaller) flushInstancePorts(ctx context.Context, machined *machineData, toOpen, toClose firewall.IngressRules) (err error) {
	defer func() {
		if params.IsCodeNotFound(err) {
			err = nil
		}
	}()

	// If there's nothing to do, do nothing.
	// This is important because when a machine is first created,
	// it will have no instance id but also no open ports -
	// InstanceId will fail but we don't care.
	fw.logger.Debugf(ctx, "flush instance ports for %v: to open %v, to close %v", machined.name, toOpen, toClose)
	if len(toOpen) == 0 && len(toClose) == 0 {
		return nil
	}
	m, err := machined.machine(ctx)
	if err != nil {
		return err
	}
	machineName := machined.name
	instanceId, err := m.InstanceId(ctx)
	if errors.Is(err, errors.NotProvisioned) {
		// Not provisioned yet, so nothing to do for this instance
		return nil
	}
	if err != nil {
		return err
	}
	envInstances, err := fw.environInstances.Instances(ctx, []instance.Id{instanceId})
	if err != nil {
		return err
	}
	fwInstance, ok := envInstances[0].(instances.InstanceFirewaller)
	if !ok {
		fw.logger.Infof(ctx, "flushInstancePorts called on an instance of type %T which doesn't support firewall.",
			envInstances[0])
		return nil
	}

	// Open and close the ports.
	if len(toOpen) > 0 {
		toOpen.Sort()
		if err := fwInstance.OpenPorts(ctx, machineName.String(), toOpen); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		fw.logger.Infof(ctx, "opened port ranges %v on %q", toOpen, machined.name)
	}
	if len(toClose) > 0 {
		toClose.Sort()
		if err := fwInstance.ClosePorts(ctx, machineName.String(), toClose); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		fw.logger.Infof(ctx, "closed port ranges %v on %q", toClose, machined.name)
	}
	return nil
}

// machineLifeChanged starts watching new machines when the firewaller
// is starting, or when new machines come to life, and stops watching
// machines that are dying.
func (fw *Firewaller) machineLifeChanged(ctx context.Context, name machine.Name) error {
	m, err := fw.firewallerApi.Machine(ctx, names.NewMachineTag(name.String()))
	found := !params.IsCodeNotFound(err)
	if found && err != nil {
		return err
	}
	dead := !found || m.Life() == life.Dead
	machined, known := fw.machineds[name]
	if known && dead {
		return fw.forgetMachine(ctx, machined)
	}
	if !known && !dead {
		err := fw.startMachine(ctx, name)
		if err != nil {
			return err
		}
	}
	return nil
}

// forgetMachine cleans the machine data after the machine is removed.
func (fw *Firewaller) forgetMachine(ctx context.Context, machined *machineData) error {
	for _, unitd := range machined.unitds {
		fw.forgetUnit(ctx, unitd)
	}
	if err := fw.flushMachine(ctx, machined); err != nil {
		return errors.Trace(err)
	}

	// Unusually, it's fine to ignore this error, because we know the machined
	// is being tracked in fw.catacomb. But we do still want to wait until the
	// watch loop has stopped before we nuke the last data and return.
	_ = worker.Stop(machined)
	delete(fw.machineds, machined.name)
	fw.logger.Debugf(ctx, "stopped watching %q", machined.name)
	return nil
}

// forgetUnit cleans the unit data after the unit is removed.
func (fw *Firewaller) forgetUnit(ctx context.Context, unitd *unitData) {
	applicationd := unitd.applicationd
	machined := unitd.machined

	// If it's the last unit in the application, we'll need to stop the applicationd.
	stoppedApplication := false
	if len(applicationd.unitds) == 1 {
		if _, found := applicationd.unitds[unitd.name]; found {
			// Unusually, it's fine to ignore this error, because we know the
			// applicationd is being tracked in fw.catacomb. But we do still want
			// to wait until the watch loop has stopped before we nuke the last
			// data and return.
			_ = worker.Stop(applicationd)
			stoppedApplication = true
		}
	}

	// Clean up after stopping.
	delete(fw.unitds, unitd.name)
	delete(machined.unitds, unitd.name)
	delete(applicationd.unitds, unitd.name)
	fw.logger.Debugf(ctx, "stopped watching %q", unitd.name)
	if stoppedApplication {
		applicationTag := applicationd.applicationTag
		delete(fw.applicationids, applicationTag)
		fw.logger.Debugf(ctx, "stopped watching %q", applicationTag)
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

func (fw *Firewaller) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(fw.catacomb.Context(context.Background()))
}

// unitsChange contains the changed units for one specific machine.
type unitsChange struct {
	machined *machineData
	units    []coreunit.Name
}

// machineData holds machine details and watches units added or removed.
type machineData struct {
	catacomb     catacomb.Catacomb
	fw           *Firewaller
	name         machine.Name
	unitds       map[coreunit.Name]*unitData
	ingressRules firewall.IngressRules
	// ports defined by units on this machine
	openedPortRangesByEndpoint map[coreunit.Name]network.GroupedPortRanges
}

func (md *machineData) machine(ctx context.Context) (Machine, error) {
	return md.fw.firewallerApi.Machine(ctx, names.NewMachineTag(md.name.String()))
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
			unitNames, err := transform.SliceOrErr(change, coreunit.NewName)
			if err != nil {
				return err
			}
			select {
			case <-md.catacomb.Dying():
				return md.catacomb.ErrDying()
			case md.fw.unitsChange <- &unitsChange{machined: md, units: unitNames}:
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
	name         coreunit.Name
	unit         Unit
	applicationd *applicationData
	machined     *machineData
}

// exposedChange contains the changed exposed flag for one specific application.
type exposedChange struct {
	applicationd     *applicationData
	exposed          bool
	exposedEndpoints map[string]application.ExposedEndpoint
}

// applicationData holds application details and watches exposure changes.
type applicationData struct {
	catacomb           catacomb.Catacomb
	fw                 *Firewaller
	applicationTag     names.ApplicationTag
	applicationService ApplicationService
	exposed            bool
	exposedEndpoints   map[string]application.ExposedEndpoint
	unitds             map[coreunit.Name]*unitData
}

// watchLoop watches the application's exposed flag for changes.
func (ad *applicationData) watchLoop(curExposed bool, curExposedEndpoints map[string]application.ExposedEndpoint) error {
	ctx, cancel := ad.scopedContext()
	defer cancel()

	appWatcher, err := ad.applicationService.WatchApplicationExposed(ctx, ad.applicationTag.Name)
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
			newIsExposed, err := ad.applicationService.IsApplicationExposed(ctx, ad.applicationTag.Name)
			if err != nil {
				if errors.Is(err, errors.NotFound) {
					ad.fw.logger.Debugf(ctx, "is exposed application %q, app not found: %w", ad.applicationTag.Name, err)
					return nil
				}
				return internalerrors.Capture(err)
			}
			newExposedEndpoints, err := ad.applicationService.GetExposedEndpoints(ctx, ad.applicationTag.Name)
			if err != nil {
				if errors.Is(err, errors.NotFound) {
					ad.fw.logger.Debugf(ctx, "expose info for application %q, app not found: %w", ad.applicationTag.Name, err)
					return nil
				}
				return internalerrors.Capture(err)
			}
			if curExposed == newIsExposed && equalExposedEndpoints(curExposedEndpoints, newExposedEndpoints) {
				ad.fw.logger.Tracef(ctx, "application %q expose settings unchanged: exposed: %v, exposedEndpoints: %v",
					ad.applicationTag.Name, curExposed, curExposedEndpoints)
				continue
			}
			ad.fw.logger.Tracef(ctx, "application %q expose settings changed: exposed: %v, exposedEndpoints: %v",
				ad.applicationTag.Name, newIsExposed, newExposedEndpoints)

			curExposed, curExposedEndpoints = newIsExposed, newExposedEndpoints
			select {
			case <-ad.catacomb.Dying():
				return ad.catacomb.ErrDying()
			case ad.fw.exposedChange <- &exposedChange{ad, newIsExposed, newExposedEndpoints}:
			}
		}
	}
}

func equalExposedEndpoints(a, b map[string]application.ExposedEndpoint) bool {
	if len(a) != len(b) {
		return false
	}

	for endpoint, exposeDetailsA := range a {
		exposeDetailsB, found := b[endpoint]
		if !found {
			return false
		}

		if !equalStringSets(exposeDetailsA.ExposeToSpaceIDs, exposeDetailsB.ExposeToSpaceIDs) ||
			!equalStringSets(exposeDetailsA.ExposeToCIDRs, exposeDetailsB.ExposeToCIDRs) {
			return false
		}
	}

	return true
}

func equalStringSets(a, b set.Strings) bool {
	if a.Size() != b.Size() {
		return false
	}

	return !a.Difference(b).IsEmpty()
}

// Kill is part of the worker.Worker interface.
func (ad *applicationData) Kill() {
	ad.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (ad *applicationData) Wait() error {
	return ad.catacomb.Wait()
}

func (ad *applicationData) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(ad.catacomb.Context(context.Background()))
}

// consumerRelationLifeChanged manages watching for ingress/egress changes on the
// consuming side. The behavior depends on the local endpoint role:
// - If requirer: watch local egress changes and publish to remote offering model
// - If provider: watch remote egress changes and apply ingress locally
func (fw *Firewaller) consumerRelationLifeChanged(ctx context.Context, relationUUID string) error {
	shouldStart, rel, err := fw.handleRelationLifeChange(ctx, relationUUID, "consumer")
	if err != nil {
		return errors.Trace(err)
	}
	if shouldStart {
		return fw.startConsumerRelation(ctx, rel)
	}
	return nil
}

// offererRelationLifeChanged manages watching for ingress changes on the
// offering side, where we only watch locally for ingress changes.
func (fw *Firewaller) offererRelationLifeChanged(ctx context.Context, relationUUID string) error {
	shouldStart, rel, err := fw.handleRelationLifeChange(ctx, relationUUID, "offerer")
	if err != nil {
		return errors.Trace(err)
	}
	if shouldStart {
		return fw.startOffererRelation(ctx, rel)
	}
	return nil
}

// handleRelationLifeChange handles the common logic for consumer and offerer
// relation life changes.
// It returns a boolean indicating whether to start a new relation watcher,
// along with the relation tag and details needed to start it.
func (fw *Firewaller) handleRelationLifeChange(
	ctx context.Context,
	relationUUID string,
	relationType string,
) (bool, domainrelation.RelationDetails, error) {
	var (
		gone bool
		rel  domainrelation.RelationDetails
		tag  names.RelationTag
	)
	var err error
	rel, err = fw.relationService.GetRelationDetails(ctx, relation.UUID(relationUUID))
	if errors.Is(err, relationerrors.RelationNotFound) {
		gone = true
	} else if err != nil {
		return false, rel, errors.Trace(err)
	}

	gone = gone || rel.Life == life.Dead || rel.Suspended

	tag = names.NewRelationTag(rel.Key.String())
	data, known := fw.relationIngress[tag]
	if known && gone {
		fw.logger.Debugf(ctx, "%s relation %q was known but has died or been suspended", relationType, relationUUID)
		// If relation is suspended, shut off ingress immediately.
		if rel.Suspended {
			change := &remoteRelationNetworkChange{
				relationTag:         tag,
				localApplicationTag: data.localApplicationTag,
				ingressRequired:     false,
			}
			if err := fw.relationIngressChanged(ctx, change); err != nil {
				return false, rel, errors.Trace(err)
			}
		}
		return false, rel, fw.forgetRelation(ctx, data)
	}
	if !known && !gone {
		return true, rel, nil
	}
	return false, rel, nil
}

// startConsumerRelation creates a watcher for egress changes on the consuming
// side. The behavior depends on the local endpoint role:
// - If requirer: watch local egress and publish to remote.
// - If provider: watch remote egress and apply ingress locally.
func (fw *Firewaller) startConsumerRelation(ctx context.Context, rel domainrelation.RelationDetails) error {
	tag := names.NewRelationTag(rel.Key.String())
	fw.logger.Debugf(ctx, "starting consumer relation watcher for %v", tag.Id())

	// For consuming side, we need to identify which endpoint is local and which
	// is remote.
	if len(rel.Endpoints) != 2 {
		return errors.Errorf("relation %v has %d endpoints, expected 2", tag.Id(), len(rel.Endpoints))
	}

	// Try to identify the remote application by querying both endpoints.
	var localEndpoint domainrelation.Endpoint
	var remoteApplicationName string
	var remoteModelUUID string

	for _, ep := range rel.Endpoints {
		remoteApps, err := fw.crossModelRelationService.RemoteApplications(ctx, []string{ep.ApplicationName})
		if err != nil {
			return errors.Trace(err)
		}
		if len(remoteApps) > 0 && remoteApps[0].Result != nil {
			// This is the remote application.
			remoteApplicationName = ep.ApplicationName
			remoteModelUUID = remoteApps[0].Result.ModelUUID
		} else {
			// This is the local application.
			localEndpoint = ep
		}
	}

	if remoteApplicationName == "" {
		return errors.Errorf("could not identify remote application for relation %v", tag.Id())
	}

	data := &remoteRelationData{
		fw:                  fw,
		tag:                 tag,
		localApplicationTag: names.NewApplicationTag(localEndpoint.ApplicationName),
		remoteModelUUID:     remoteModelUUID,
	}
	var watchEgressLoop func() error
	if localEndpoint.Role == charm.RoleRequirer {
		// Local endpoint is requirer, so watch local egress and publish to
		// remote.
		watchEgressLoop = data.watchLocalEgressPublishRemote
	} else {
		// Local endpoint is provider, so watch remote egress and apply locally.
		watchEgressLoop = data.watchRemoteEgressApplyLocal
	}

	// Start the worker which watches for ingress/egress address changes
	if err := fw.relationWorkerRunner.StartWorker(ctx, tag.Id(), func(ctx context.Context) (worker.Worker, error) {
		// This may be a restart after an api error, so ensure any previous
		// worker is killed and the catacomb is reset.
		data.Kill()
		data.catacomb = catacomb.Catacomb{}

		if err := catacomb.Invoke(catacomb.Plan{
			Name: "firewaller-consumer-relation",
			Site: &data.catacomb,
			Work: watchEgressLoop,
		}); err != nil {
			return nil, errors.Trace(err)
		}
		return data, nil
	}); err != nil {
		return errors.Annotate(err, "error starting consumer relation worker")
	}
	fw.relationIngress[tag] = data

	return nil
}

// startOffererRelation creates a watcher for ingress changes on the offering
// side.
// On the offering side, we watch the local model for ingress changes which will
// have been published from the consuming model.
// We only watch if the local endpoint role is provider.
func (fw *Firewaller) startOffererRelation(ctx context.Context, rel domainrelation.RelationDetails) error {
	tag := names.NewRelationTag(rel.Key.String())
	fw.logger.Debugf(ctx, "starting offerer relation watcher for %v", tag.Id())

	// For offering side, get the local application name from the first
	// endpoint.
	if len(rel.Endpoints) == 0 {
		return errors.Errorf("relation %v has no endpoints", tag.Id())
	}
	localEndpoint := rel.Endpoints[0]

	// On the offering side, only watch for ingress changes if the endpoint is a
	// provider.
	if localEndpoint.Role == charm.RoleRequirer {
		fw.logger.Debugf(ctx, "skipping offerer relation %v: local endpoint is requirer", tag.Id())
		return nil
	}

	localApplicationName := localEndpoint.ApplicationName

	data := &remoteRelationData{
		fw:                  fw,
		tag:                 tag,
		localApplicationTag: names.NewApplicationTag(localApplicationName),
	}

	// Start the worker which watches for ingress address changes
	if err := fw.relationWorkerRunner.StartWorker(ctx, tag.Id(), func(ctx context.Context) (worker.Worker, error) {
		// This may be a restart after an api error, so ensure any previous
		// worker is killed and the catacomb is reset.
		data.Kill()
		data.catacomb = catacomb.Catacomb{}

		if err := catacomb.Invoke(catacomb.Plan{
			Name: "firewaller-offerer-relation",
			Site: &data.catacomb,
			Work: data.watchLocalIngress,
		}); err != nil {
			return nil, errors.Trace(err)
		}
		return data, nil
	}); err != nil {
		return errors.Annotate(err, "error starting offerer relation worker")
	}
	fw.relationIngress[tag] = data

	return nil
}

// watchLocalIngress watches for ingress address changes on the offering
// side.
func (rd *remoteRelationData) watchLocalIngress() error {
	ctx, cancel := rd.scopedContext()
	defer cancel()

	fw := rd.fw
	fw.logger.Debugf(ctx, "starting offerer watch loop for relation %v on %v", rd.tag.Id(), rd.localApplicationTag.Id())

	// Watch the local model for ingress changes published from the consuming
	// model.
	ingressAddressWatcher, err := fw.firewallerApi.WatchIngressAddressesForRelation(ctx, rd.tag)
	if err != nil {
		if !params.IsCodeNotFound(err) && !params.IsCodeNotSupported(err) {
			return errors.Trace(err)
		}
		fw.logger.Infof(ctx, "no ingress required for %v", rd.localApplicationTag)
		rd.ingressRequired = false
		return nil
	}
	if err := rd.catacomb.Add(ingressAddressWatcher); err != nil {
		return errors.Trace(err)
	}

	localChanges := make(chan *remoteRelationNetworkChange)

	for {
		select {
		case <-rd.catacomb.Dying():
			return rd.catacomb.ErrDying()
		case cidrs := <-ingressAddressWatcher.Changes():
			fw.logger.Debugf(ctx, "offerer relation ingress addresses for %v changed: %v", rd.tag, cidrs)
			change := &remoteRelationNetworkChange{
				relationTag:         rd.tag,
				localApplicationTag: rd.localApplicationTag,
				networks:            set.NewStrings(cidrs...),
				ingressRequired:     len(cidrs) > 0,
			}
			localChanges <- change
		case change := <-localChanges:
			rd.fw.localRelationsChange <- change
		}
	}
}

// watchLocalEgressPublishRemote watches local egress addresses and publishes them
// to the remote offering model.
func (rd *remoteRelationData) watchLocalEgressPublishRemote() error {
	ctx, cancel := rd.scopedContext()
	defer cancel()

	rd.fw.logger.Debugf(ctx, "watching local egress for %v to publish to remote", rd.tag.Id())

	defer func() {
		if rd.crossModelFirewallerFacade != nil {
			rd.crossModelFirewallerFacade.Close()
		}
	}()

	egressAddressWatcher, err := rd.fw.firewallerApi.WatchEgressAddressesForRelation(ctx, rd.tag)
	if err != nil {
		if !params.IsCodeNotFound(err) && !params.IsCodeNotSupported(err) {
			return errors.Trace(err)
		}
		rd.fw.logger.Infof(ctx, "no egress required for %v", rd.localApplicationTag)
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
			rd.fw.logger.Debugf(ctx, "local egress addresses for %v changed: %v", rd.tag, cidrs)
			if err := rd.publishIngressToRemote(ctx, cidrs); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// watchRemoteEgressApplyLocal watches remote egress addresses and applies them
// as local ingress rules.
func (rd *remoteRelationData) watchRemoteEgressApplyLocal() error {
	ctx, cancel := rd.scopedContext()
	defer cancel()

	rd.fw.logger.Debugf(ctx, "watching remote egress for %v to apply local ingress", rd.tag.Id())

	// Connect to remote model to watch egress
	apiInfo, err := rd.fw.firewallerApi.ControllerAPIInfoForModel(ctx, rd.remoteModelUUID)
	if err != nil {
		return errors.Annotatef(err, "cannot get api info for model %v", rd.remoteModelUUID)
	}
	rd.crossModelFirewallerFacade, err = rd.fw.newRemoteFirewallerAPIFunc(ctx, apiInfo)
	if err != nil {
		return errors.Annotate(err, "cannot open facade to remote model to watch egress addresses")
	}

	mac, err := rd.fw.firewallerApi.MacaroonForRelation(ctx, rd.tag.Id())
	if err != nil {
		return errors.Annotatef(err, "cannot get macaroon for %v", rd.tag.Id())
	}
	arg := params.RemoteEntityArg{
		Token:         rd.relationToken,
		Macaroons:     macaroon.Slice{mac},
		BakeryVersion: bakery.LatestVersion,
	}
	egressAddressWatcher, err := rd.crossModelFirewallerFacade.WatchEgressAddressesForRelation(ctx, arg)
	if err != nil {
		if !params.IsCodeNotFound(err) && !params.IsCodeNotSupported(err) {
			return errors.Trace(err)
		}
		rd.fw.logger.Infof(ctx, "no ingress required for %v", rd.localApplicationTag)
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
			rd.fw.logger.Debugf(ctx, "remote egress addresses for %v changed: %v", rd.tag, cidrs)
			if err := rd.updateIngressNetworks(ctx, cidrs); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// publishIngressToRemote publishes ingress network changes to the remote offering model.
func (rd *remoteRelationData) publishIngressToRemote(ctx context.Context, cidrs []string) error {
	rd.fw.logger.Debugf(ctx, "publishing ingress cidrs for %v: %+v", rd.tag, cidrs)

	apiInfo, err := rd.fw.firewallerApi.ControllerAPIInfoForModel(ctx, rd.remoteModelUUID)
	if err != nil {
		return errors.Annotatef(err, "cannot get api info for model %v", rd.remoteModelUUID)
	}
	mac, err := rd.fw.firewallerApi.MacaroonForRelation(ctx, rd.tag.Id())
	if params.IsCodeNotFound(err) {
		// Relation has gone, nothing to do.
		return nil
	}
	if err != nil {
		return errors.Annotatef(err, "cannot get macaroon for %v", rd.tag.Id())
	}
	remoteModelAPI, err := rd.fw.newRemoteFirewallerAPIFunc(ctx, apiInfo)
	if err != nil {
		return errors.Annotate(err, "cannot open facade to remote model to publish network change")
	}
	defer remoteModelAPI.Close()

	event := params.IngressNetworksChangeEvent{
		RelationToken:   rd.relationToken,
		Networks:        cidrs,
		IngressRequired: len(cidrs) > 0,
		Macaroons:       macaroon.Slice{mac},
		BakeryVersion:   bakery.LatestVersion,
	}
	if err := remoteModelAPI.PublishIngressNetworkChange(ctx, event); err != nil {
		return errors.Trace(err)
	}

	rd.ingressRequired = len(cidrs) > 0
	rd.networks = set.NewStrings(cidrs...)
	return nil
}

type remoteRelationInfo struct {
	relationToken string
}

type remoteRelationData struct {
	catacomb catacomb.Catacomb
	fw       *Firewaller

	tag                 names.RelationTag
	localApplicationTag names.ApplicationTag
	relationToken       string
	remoteModelUUID     string

	crossModelFirewallerFacade CrossModelFirewallerFacadeCloser

	// These values are updated when ingress information on the
	// relation changes in the model.
	ingressRequired bool
	networks        set.Strings
}

func (rd *remoteRelationData) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(rd.catacomb.Context(context.Background()))
}

type remoteRelationNetworkChange struct {
	relationTag         names.RelationTag
	localApplicationTag names.ApplicationTag
	networks            set.Strings
	ingressRequired     bool
}

// updateProviderModel gathers the ingress CIDRs for the relation and notifies
// that a change has occurred.
func (rd *remoteRelationData) updateProviderModel(ctx context.Context, cidrs []string) error {
	rd.fw.logger.Debugf(ctx, "ingress cidrs for %v: %+v", rd.tag, cidrs)
	change := &remoteRelationNetworkChange{
		relationTag:         rd.tag,
		localApplicationTag: rd.localApplicationTag,
		networks:            set.NewStrings(cidrs...),
		ingressRequired:     len(cidrs) > 0,
	}

	apiInfo, err := rd.fw.firewallerApi.ControllerAPIInfoForModel(ctx, rd.remoteModelUUID)
	if err != nil {
		return errors.Annotatef(err, "cannot get api info for model %v", rd.remoteModelUUID)
	}
	mac, err := rd.fw.firewallerApi.MacaroonForRelation(ctx, rd.tag.Id())
	if params.IsCodeNotFound(err) {
		// Relation has gone, nothing to do.
		return nil
	}
	if err != nil {
		return errors.Annotatef(err, "cannot get macaroon for %v", rd.tag.Id())
	}
	remoteModelAPI, err := rd.fw.newRemoteFirewallerAPIFunc(ctx, apiInfo)
	if err != nil {
		return errors.Annotate(err, "cannot open facade to remote model to publish network change")
	}
	defer remoteModelAPI.Close()
	event := params.IngressNetworksChangeEvent{
		RelationToken:   rd.relationToken,
		Networks:        change.networks.Values(),
		IngressRequired: change.ingressRequired,
		Macaroons:       macaroon.Slice{mac},
		BakeryVersion:   bakery.LatestVersion,
	}
	err = remoteModelAPI.PublishIngressNetworkChange(ctx, event)
	if errors.Is(err, errors.NotFound) {
		rd.fw.logger.Debugf(ctx, "relation id not found publishing %+v", event)
		return nil
	}

	// If the requested ingress is not permitted on the offering side,
	// mark the relation as in error. It's not an error that requires a
	// worker restart though.
	if params.IsCodeForbidden(err) {
		return rd.fw.firewallerApi.SetRelationStatus(ctx, rd.tag.Id(), relation.Error, err.Error())
	}
	return errors.Annotate(err, "cannot publish ingress network change")
}

// updateIngressNetworks processes the changed ingress networks on the relation.
func (rd *remoteRelationData) updateIngressNetworks(ctx context.Context, cidrs []string) error {
	rd.fw.logger.Debugf(ctx, "ingress cidrs for %v: %+v", rd.tag, cidrs)
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
func (fw *Firewaller) forgetRelation(ctx context.Context, data *remoteRelationData) error {
	fw.logger.Debugf(ctx, "forget relation %v", data.tag.Id())
	delete(fw.relationIngress, data.tag)
	// There's not much we can do if there's an error stopping the remote
	// relation worker, so just log it.
	if err := fw.relationWorkerRunner.StopAndRemoveWorker(data.tag.Id(), fw.catacomb.Dying()); err != nil {
		fw.logger.Errorf(ctx, "error stopping remote relation worker for %s: %v", data.tag, err)
	}
	fw.logger.Debugf(ctx, "stopped watching %q", data.tag)
	return nil
}
