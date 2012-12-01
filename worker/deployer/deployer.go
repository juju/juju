package deployer

import (
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/tomb"
)

// Context is a representation of the set of units deployed by a given agent.
type Context interface {

	// DeployerName identifies the agent represented by the context.
	DeployerName() string

	// DeployUnit causes the agent for the specified unit to be started and run
	// continuously until further notice without further intervention. It will
	// return an error if the agent is already deployed.
	DeployUnit(name, initialPassword string) error

	// RecallUnit causes the agent for the specified unit to be stopped, and
	// the agent's data to be destroyed. It will return an error if the agent
	// was not deployed to the context.
	RecallUnit(name string) error

	// DeployedUnits returns the names of all units deployed to the context.
	DeployedUnits() ([]string, error)
}

// Deployer is responsible for deploying and recalling unit agents, according
// to changes in a set of state units; and for the final removal of its agents'
// units from state when they are no longer needed.
type Deployer struct {
	tomb     tomb.Tomb
	st       *state.State
	ctx      Context
	deployed map[string]bool
}

// NewDeployer returns a Deployer that deploys and recalls unit agents in
// ctx, according to membership and lifecycle changes notified by w.
func NewDeployer(st *state.State, ctx Context, w *state.UnitsWatcher) *Deployer {
	d := &Deployer{
		st:       st,
		ctx:      ctx,
		deployed: map[string]bool{},
	}
	go func() {
		defer d.tomb.Done()
		defer watcher.Stop(w, &d.tomb)
		d.tomb.Kill(d.loop(w))
	}()
	return d
}

func (d *Deployer) String() string {
	return "deployer for " + d.ctx.DeployerName()
}

func (d *Deployer) Stop() error {
	d.tomb.Kill(nil)
	return d.tomb.Wait()
}

func (d *Deployer) Wait() error {
	return d.tomb.Wait()
}

// changed responds to a reported change in the named unit. This change may
// cause the Deployer to deploy or recall the unit, and potentially set it
// to Dead and remove it from state. The latter operations will only occur
// when the deployer context is known to be responsible for the unit, and the
// unit is known to be inactive and not Alive.
func (d *Deployer) changed(name string) error {
	// Determine unit life state, and whether we're responsible for it.
	log.Printf("worker/deployer: checking unit %q", name)
	var life state.Life
	responsible := false
	unit, err := d.st.Unit(name)
	if state.IsNotFound(err) {
		life = state.Dead
	} else if err != nil {
		return err
	} else {
		life = unit.Life()
		if deployerName, ok := unit.DeployerName(); ok {
			responsible = deployerName == d.ctx.DeployerName()
		}
	}
	if d.deployed[name] {
		if life == state.Dead || !responsible {
			if err := d.recall(name); err != nil {
				return err
			}
		}
	}
	if responsible && !d.deployed[name] {
		if life == state.Alive {
			return d.deploy(unit)
		} else if unit != nil {
			return d.remove(unit)
		}
	}
	return nil
}

func (d *Deployer) deploy(unit *state.Unit) error {
	log.Printf("worker/deployer: deploying agent for unit %q", unit)
	name := unit.Name()
	password, err := trivial.RandomPassword()
	if err != nil {
		return err
	}
	if err := unit.SetPassword(password); err != nil {
		return err
	}
	if err := d.ctx.DeployUnit(name, password); err != nil {
		return err
	}
	d.deployed[name] = true
	return nil
}

func (d *Deployer) recall(name string) error {
	log.Printf("worker/deployer: recalling agent for unit %q", name)
	if err := d.ctx.RecallUnit(name); err != nil {
		return err
	}
	delete(d.deployed, name)
	return nil
}

func (d *Deployer) remove(unit *state.Unit) error {
	log.Printf("worker/deployer: removing unit %q", unit)
	if err := unit.EnsureDead(); err != nil {
		return err
	}
	service, err := unit.Service()
	if err != nil {
		return err
	}
	return service.RemoveUnit(unit)
}

func (d *Deployer) loop(w *state.UnitsWatcher) error {
	deployed, err := d.ctx.DeployedUnits()
	if err != nil {
		return err
	}
	for _, name := range deployed {
		d.deployed[name] = true
		if err := d.changed(name); err != nil {
			return err
		}
	}
	for {
		select {
		case <-d.tomb.Dying():
			return tomb.ErrDying
		case changes, ok := <-w.Changes():
			if !ok {
				return watcher.MustErr(w)
			}
			for _, name := range changes {
				if err := d.changed(name); err != nil {
					return err
				}
			}
		}
	}
	panic("unreachable")
}
