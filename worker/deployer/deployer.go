// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/names"
	apideployer "launchpad.net/juju-core/state/api/deployer"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/worker"
)

var logger = loggo.GetLogger("juju.worker.deployer")

// Deployer is responsible for deploying and recalling unit agents, according
// to changes in a set of state units; and for the final removal of its agents'
// units from state when they are no longer needed.
type Deployer struct {
	st       *apideployer.State
	ctx      Context
	deployed set.Strings
}

// Context abstracts away the differences between different unit deployment
// strategies; where a Deployer is responsible for what to deploy, a Context
// is responsible for how to deploy.
type Context interface {
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
}

// NewDeployer returns a Worker that deploys and recalls unit agents
// via ctx, taking a machine id to operate on.
func NewDeployer(st *apideployer.State, ctx Context) worker.Worker {
	d := &Deployer{
		st:  st,
		ctx: ctx,
	}
	return worker.NewStringsWorker(d)
}

func (d *Deployer) SetUp() (watcher.StringsWatcher, error) {
	machineTag := d.ctx.AgentConfig().Tag()
	machine, err := d.st.Machine(machineTag)
	if err != nil {
		return nil, err
	}
	machineUnitsWatcher, err := machine.WatchUnits()
	if err != nil {
		return nil, err
	}

	deployed, err := d.ctx.DeployedUnits()
	if err != nil {
		return nil, err
	}
	for _, unitName := range deployed {
		d.deployed.Add(unitName)
		if err := d.changed(unitName); err != nil {
			return nil, err
		}
	}
	return machineUnitsWatcher, nil
}

func (d *Deployer) Handle(unitNames []string) error {
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
	unitTag := names.UnitTag(unitName)
	// Determine unit life state, and whether we're responsible for it.
	logger.Infof("checking unit %q", unitName)
	var life params.Life
	unit, err := d.st.Unit(unitTag)
	if params.IsCodeNotFoundOrCodeUnauthorized(err) {
		life = params.Dead
	} else if err != nil {
		return err
	} else {
		life = unit.Life()
	}
	// Deployed units must be removed if they're Dead, or if the deployer
	// is no longer responsible for them.
	if d.deployed.Contains(unitName) {
		if life == params.Dead {
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
		if life == params.Alive {
			return d.deploy(unit)
		} else if unit != nil {
			return d.remove(unit)
		}
	}
	return nil
}

// deploy will deploy the supplied unit with the deployer's manager. It will
// panic if it observes inconsistent internal state.
func (d *Deployer) deploy(unit *apideployer.Unit) error {
	unitName := unit.Name()
	if d.deployed.Contains(unit.Name()) {
		panic("must not re-deploy a deployed unit")
	}
	logger.Infof("deploying unit %q", unitName)
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
	logger.Infof("recalling unit %q", unitName)
	if err := d.ctx.RecallUnit(unitName); err != nil {
		return err
	}
	d.deployed.Remove(unitName)
	return nil
}

// remove will remove the supplied unit from state. It will panic if it
// observes inconsistent internal state.
func (d *Deployer) remove(unit *apideployer.Unit) error {
	unitName := unit.Name()
	if d.deployed.Contains(unitName) {
		panic("must not remove a deployed unit")
	} else if unit.Life() == params.Alive {
		panic("must not remove an Alive unit")
	}
	logger.Infof("removing unit %q", unitName)
	return unit.Remove()
}

func (d *Deployer) TearDown() error {
	// Nothing to do here.
	return nil
}
