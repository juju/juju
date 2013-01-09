package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker/uniter"
	"launchpad.net/tomb"
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
	if !state.IsUnitName(a.UnitName) {
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
	defer log.Printf("cmd/jujud: unit agent exiting")
	defer a.tomb.Done()
	return RunLoop(a.Conf.Conf, a)
}

// RunOnce runs a unit agent once.
func (a *UnitAgent) RunOnce(st *state.State, e AgentState) error {
	unit := e.(*state.Unit)
	tasks := []task{
		uniter.NewUniter(st, unit.Name(), a.Conf.DataDir),
		NewUpgrader(st, unit, a.Conf.DataDir),
	}
	if unit.IsPrincipal() {
		tasks = append(tasks,
			newDeployer(st, unit.WatchSubordinateUnits(), a.Conf.DataDir))
	}
	return runTasks(a.tomb.Dying(), tasks...)
}

func (a *UnitAgent) Entity(st *state.State) (AgentState, error) {
	return st.Unit(a.UnitName)
}

func (a *UnitAgent) EntityName() string {
	return state.UnitEntityName(a.UnitName)
}

func (a *UnitAgent) Tomb() *tomb.Tomb {
	return &a.tomb
}
