// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/loggo"
	"launchpad.net/tomb"
)

var logger = loggo.GetLogger("juju.deployer")

// Deployer is responsible for deploying and recalling unit agents, according
// to changes in a set of state units; and for the final removal of its agents'
// units from state when they are no longer needed.
type Deployer struct {
	tomb      tomb.Tomb
	st        *state.State
	ctx       Context
	machineId string
	deployed  set.Strings
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
}

// NewDeployer returns a Deployer that deploys and recalls unit agents via
// ctx, taking a machine id to operate on.
func NewDeployer(st *state.State, ctx Context, machineId string) *Deployer {
	d := &Deployer{
		st:        st,
		ctx:       ctx,
		machineId: machineId,
	}
	go func() {
		defer d.tomb.Done()
		d.tomb.Kill(d.loop())
	}()
	return d
}

func (d *Deployer) String() string {
	return "deployer for " + d.machineId
}

func (d *Deployer) Kill() {
	d.tomb.Kill(nil)
}

func (d *Deployer) Stop() error {
	d.tomb.Kill(nil)
	return d.tomb.Wait()
}

func (d *Deployer) Wait() error {
	return d.tomb.Wait()
}

// changed ensures that the named unit is deployed, recalled, or removed, as
// indicated by its state.
func (d *Deployer) changed(unitName string) error {
	// Determine unit life state, and whether we're responsible for it.
	logger.Infof("checking unit %q", unitName)
	var life state.Life
	responsible := false
	unit, err := d.st.Unit(unitName)
	if errors.IsNotFoundError(err) {
		life = state.Dead
	} else if err != nil {
		return err
	} else {
		life = unit.Life()
		if machineId, err := unit.AssignedMachineId(); state.IsNotAssigned(err) {
			logger.Warningf("ignoring unit %q (not assigned)", unitName)
			// Unit is not assigned, so we're not responsible for its deployment.
			responsible = false
		} else if err != nil {
			return err
		} else {
			// We're only responsible for this unit's deployment when we're
			// running inside the machine agent of the machine this unit
			// is assigned to.
			responsible = (machineId == d.machineId)
		}
	}
	// Deployed units must be removed if they're Dead, or if the deployer
	// is no longer responsible for them.
	if d.deployed.Contains(unitName) {
		if life == state.Dead || !responsible {
			if err := d.recall(unitName); err != nil {
				return err
			}
		}
	}
	// The only units that should be deployed are those that (1) we are responsible
	// for and (2) are Alive -- if we're responsible for a Dying unit that is not
	// yet deployed, we should remove it immediately rather than undergo the hassle
	// of deploying a unit agent purely so it can set itself to Dead.
	if responsible && !d.deployed.Contains(unitName) {
		if life == state.Alive {
			return d.deploy(unit)
		} else if unit != nil {
			return d.remove(unit)
		}
	}
	return nil
}

// deploy will deploy the supplied unit with the deployer's manager. It will
// panic if it observes inconsistent internal state.
func (d *Deployer) deploy(unit *state.Unit) error {
	unitName := unit.Name()
	if d.deployed.Contains(unit.Name()) {
		panic("must not re-deploy a deployed unit")
	}
	logger.Infof("deploying unit %q", unit)
	initialPassword, err := utils.RandomPassword()
	if err != nil {
		return err
	}
	if err := unit.SetPassword(initialPassword); err != nil {
		return fmt.Errorf("cannot set password for unit %q: %v", unit, err)
	}
	if err := unit.SetMongoPassword(initialPassword); err != nil {
		return fmt.Errorf("cannot set mongo password for unit %q: %v", unit, err)
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
func (d *Deployer) remove(unit *state.Unit) error {
	if d.deployed.Contains(unit.Name()) {
		panic("must not remove a deployed unit")
	} else if unit.Life() == state.Alive {
		panic("must not remove an Alive unit")
	}
	logger.Infof("removing unit %q", unit)
	if err := unit.EnsureDead(); err != nil {
		return err
	}
	return unit.Remove()
}

func (d *Deployer) loop() error {
	machine, err := d.st.Machine(d.machineId)
	if err != nil {
		return err
	}
	machineUnitsWatcher := machine.WatchUnits()
	defer watcher.Stop(machineUnitsWatcher, &d.tomb)

	deployed, err := d.ctx.DeployedUnits()
	if err != nil {
		return err
	}
	for _, unitName := range deployed {
		d.deployed.Add(unitName)
		if err := d.changed(unitName); err != nil {
			return err
		}
	}
	for {
		select {
		case <-d.tomb.Dying():
			return tomb.ErrDying
		case changes, ok := <-machineUnitsWatcher.Changes():
			if !ok {
				return watcher.MustErr(machineUnitsWatcher)
			}
			for _, unitName := range changes {
				if err := d.changed(unitName); err != nil {
					return err
				}
			}
		}
	}
	panic("unreachable")
}
