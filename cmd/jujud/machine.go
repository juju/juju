package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	_ "launchpad.net/juju-core/environs/ec2"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker/firewaller"
	"launchpad.net/juju-core/worker/provisioner"
	"launchpad.net/tomb"
	"time"
)

var retryDelay = 3 * time.Second

// MachineAgent is a cmd.Command responsible for running a machine agent.
type MachineAgent struct {
	tomb      tomb.Tomb
	Conf      AgentConf
	MachineId string
}

// Info returns usage information for the command.
func (a *MachineAgent) Info() *cmd.Info {
	return &cmd.Info{"machine", "", "run a juju machine agent", ""}
}

// Init initializes the command for running.
func (a *MachineAgent) Init(f *gnuflag.FlagSet, args []string) error {
	a.Conf.addFlags(f, flagAll)
	f.StringVar(&a.MachineId, "machine-id", "", "id of the machine to run")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if !state.IsMachineId(a.MachineId) {
		return fmt.Errorf("--machine-id option must be set, and expects a non-negative integer")
	}
	return a.Conf.checkArgs(f.Args())
}

// Stop stops the machine agent.
func (a *MachineAgent) Stop() error {
	a.tomb.Kill(nil)
	return a.tomb.Wait()
}

// Run runs a machine agent.
func (a *MachineAgent) Run(_ *cmd.Context) error {
	defer log.Printf("cmd/jujud: machine agent exiting")
	defer a.tomb.Done()
	return RunLoop(&a.Conf, a)
}

func (a *MachineAgent) RunOnce(st *state.State, e AgentState) error {
	m := e.(*state.Machine)
	log.Printf("cmd/jujud: running jobs for machine agent: %v", m.Jobs())
	tasks := []task{NewUpgrader(st, m, a.Conf.DataDir)}
	for _, j := range m.Jobs() {
		switch j {
		case state.JobHostUnits:
			tasks = append(tasks,
				newDeployer(st, m.WatchPrincipalUnits(), a.Conf.DataDir))
		case state.JobManageEnviron:
			tasks = append(tasks,
				provisioner.NewProvisioner(st),
				firewaller.NewFirewaller(st))
		default:
			log.Printf("cmd/jujud: ignoring unknown job %q", j)
		}
	}
	return runTasks(a.tomb.Dying(), tasks...)
}

func (a *MachineAgent) Entity(st *state.State) (AgentState, error) {
	return st.Machine(a.MachineId)
}

func (a *MachineAgent) EntityName() string {
	return state.MachineEntityName(a.MachineId)
}

func (a *MachineAgent) Tomb() *tomb.Tomb {
	return &a.tomb
}
