package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker/uniter"
	"launchpad.net/tomb"
	"time"
)

// UnitAgent is a cmd.Command responsible for running a unit agent.
type UnitAgent struct {
	tomb     tomb.Tomb
	Conf     AgentConf
	UnitName string
}

// Info returns usage information for the command.
func (a *UnitAgent) Info() *cmd.Info {
	return &cmd.Info{"unit", "", "run a juju unit agent", ""}
}

// Init initializes the command for running.
func (a *UnitAgent) Init(f *gnuflag.FlagSet, args []string) error {
	a.Conf.addFlags(f)
	f.StringVar(&a.UnitName, "unit-name", "", "name of the unit to run")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if a.UnitName == "" {
		return requiredError("unit-name")
	}
	if !juju.ValidUnit.MatchString(a.UnitName) {
		return fmt.Errorf(`--unit-name option expects "<service>/<n>" argument`)
	}
	return a.Conf.checkArgs(f.Args())
}

// Stop stops the unit agent.
func (a *UnitAgent) Stop() error {
	a.tomb.Kill(nil)
	return a.tomb.Wait()
}

// Run runs a unit agent.
func (a *UnitAgent) Run(ctx *cmd.Context) error {
	defer a.tomb.Done()
	for a.tomb.Err() == tomb.ErrStillAlive {
		err := a.runOnce()
		log.Printf("uniter error: %v", err)
		select {
		case <-a.tomb.Dying():
			a.tomb.Kill(err)
		case <-time.After(retryDelay):
			log.Printf("rerunning uniter")
		}
	}
	return a.tomb.Err()
}

// runOnce runs a uniter once.
func (a *UnitAgent) runOnce() error {
	st, err := state.Open(&a.Conf.StateInfo)
	if err != nil {
		return err
	}
	defer st.Close()
	u, err := uniter.NewUniter(st, a.UnitName, a.Conf.DataDir)
	if err != nil {
		return err
	}
	return runTasks(a.tomb.Dying(), u)
}
