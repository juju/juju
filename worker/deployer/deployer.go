// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead pass one through as config to the worker.
type logger interface{}

var _ logger = struct{}{}

// Deployer is responsible for deploying and recalling unit agents, according
// to changes in a set of state units; and for the final removal of its agents'
// units from state when they are no longer needed.
type Deployer struct {
	st       API
	logger   Logger
	ctx      Context
	deployed set.Strings
}

// API is used to define the methods that the deployer makes.
type API interface {
	Machine(names.MachineTag) (Machine, error)
	Unit(names.UnitTag) (Unit, error)
}

// Machine defines the methods that the deployer makes on a machine in
// the model.
type Machine interface {
	WatchUnits() (watcher.StringsWatcher, error)
}

// Unit defines the methods that the deployer makes on a unit in the model.
type Unit interface {
	Life() life.Value
	Name() string
	Remove() error
	SetPassword(password string) error
	SetStatus(unitStatus status.Status, info string, data map[string]interface{}) error
}

// Context abstracts away the differences between different unit deployment
// strategies; where a Deployer is responsible for what to deploy, a Context
// is responsible for how to deploy.
type Context interface {
	worker.Worker

	// DeployUnit causes the agent for the specified unit to be started and run
	// continuously until further notice without further intervention. It will
	// return an error if the agent is already deployed.
	DeployUnit(unitName, initialPassword string) error

	// RecallUnit causes the agent for the specified unit to be stopped, and
	// the agent's data to be destroyed. It will return an error if the agent
	// was not deployed by the manager.
	RecallUnit(unitName string) error

	// DeployedUnits returns the names of all units deployed by the manager.
	DeployedUnits() ([]string, error)

	// AgentConfig returns the agent config for the machine agent that is
	// running the deployer.
	AgentConfig() agent.Config

	Report() map[string]interface{}
}

// NewDeployer returns a Worker that deploys and recalls unit agents
// via ctx, taking a machine id to operate on.
func NewDeployer(st API, logger Logger, ctx Context) (worker.Worker, error) {
	d := &Deployer{
		st:       st,
		logger:   logger,
		ctx:      ctx,
		deployed: make(set.Strings),
	}
	w, err := watcher.NewStringsWorker(watcher.StringsConfig{
		Handler: d,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Report is shown in the engine report.
func (d *Deployer) Report() map[string]interface{} {
	// Get the report from the context.
	return d.ctx.Report()
}

// SetUp is called by the NewStringsWorker to create the watcher that drives the
// worker.
func (d *Deployer) SetUp() (watcher.StringsWatcher, error) {
	d.logger.Tracef("SetUp")
	tag := d.ctx.AgentConfig().Tag()
	machineTag, ok := tag.(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("expected names.MachineTag, got %T", tag)
	}
	d.logger.Tracef("getting Machine %s", machineTag)
	machine, err := d.st.Machine(machineTag)
	if err != nil {
		return nil, err
	}
	d.logger.Tracef("getting units watcher")
	machineUnitsWatcher, err := machine.WatchUnits()
	if err != nil {
		d.logger.Tracef("error: %v", err)
		return nil, err
	}
	d.logger.Tracef("looking for deployed units")

	deployed, err := d.ctx.DeployedUnits()
	if err != nil {
		return nil, err
	}
	d.logger.Tracef("deployed units: %v", deployed)
	for _, unitName := range deployed {
		d.deployed.Add(unitName)
		if err := d.changed(unitName); err != nil {
			return nil, err
		}
	}
	return machineUnitsWatcher, nil
}

// Handle is called for new value in the StringsWatcher.
func (d *Deployer) Handle(_ <-chan struct{}, unitNames []string) error {
	d.logger.Tracef("Handle: %v", unitNames)
	for _, unitName := range unitNames {
		if err := d.changed(unitName); err != nil {
			return err
		}
	}
	return nil
}

// changed ensures that the named unit is deployed, recalled, or removed, as
// indicated by its state.
func (d *Deployer) changed(unitName string) error {
	unitTag := names.NewUnitTag(unitName)
	// Determine unit life state, and whether we're responsible for it.
	d.logger.Infof("checking unit %q", unitName)
	var unitLife life.Value
	unit, err := d.st.Unit(unitTag)
	if params.IsCodeNotFoundOrCodeUnauthorized(err) {
		unitLife = life.Dead
	} else if err != nil {
		return err
	} else {
		unitLife = unit.Life()
	}
	// Deployed units must be removed if they're Dead, or if the deployer
	// is no longer responsible for them.
	if d.deployed.Contains(unitName) {
		if unitLife == life.Dead {
			if err := d.recall(unitName); err != nil {
				return err
			}
		}
	}
	// The only units that should be deployed are those that (1) we are responsible
	// for and (2) are Alive -- if we're responsible for a Dying unit that is not
	// yet deployed, we should remove it immediately rather than undergo the hassle
	// of deploying a unit agent purely so it can set itself to Dead.
	if !d.deployed.Contains(unitName) {
		if unitLife == life.Alive {
			return d.deploy(unit)
		} else if unit != nil {
			return d.remove(unit)
		}
	}
	return nil
}

// deploy will deploy the supplied unit with the deployer's manager. It will
// panic if it observes inconsistent internal state.
func (d *Deployer) deploy(unit Unit) error {
	unitName := unit.Name()
	if d.deployed.Contains(unit.Name()) {
		panic("must not re-deploy a deployed unit")
	}
	if err := unit.SetStatus(status.Waiting, status.MessageInstallingAgent, nil); err != nil {
		return errors.Trace(err)
	}
	d.logger.Infof("deploying unit %q", unitName)
	initialPassword, err := utils.RandomPassword()
	if err != nil {
		return err
	}
	if err := unit.SetPassword(initialPassword); err != nil {
		return fmt.Errorf("cannot set password for unit %q: %v", unitName, err)
	}
	if err := d.ctx.DeployUnit(unitName, initialPassword); err != nil {
		return err
	}
	d.deployed.Add(unitName)
	return nil
}

// recall will recall the named unit with the deployer's manager. It will
// panic if it observes inconsistent internal state.
func (d *Deployer) recall(unitName string) error {
	if !d.deployed.Contains(unitName) {
		panic("must not recall a unit that is not deployed")
	}
	d.logger.Infof("recalling unit %q", unitName)
	if err := d.ctx.RecallUnit(unitName); err != nil {
		return err
	}
	d.deployed.Remove(unitName)
	return nil
}

// remove will remove the supplied unit from state. It will panic if it
// observes inconsistent internal state.
func (d *Deployer) remove(unit Unit) error {
	unitName := unit.Name()
	if d.deployed.Contains(unitName) {
		panic("must not remove a deployed unit")
	} else if unit.Life() == life.Alive {
		panic("must not remove an Alive unit")
	}
	d.logger.Infof("removing unit %q", unitName)
	return unit.Remove()
}

// TearDown stops the embedded context.
func (d *Deployer) TearDown() error {
	d.ctx.Kill()
	return d.ctx.Wait()
}
