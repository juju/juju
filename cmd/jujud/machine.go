package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/agent"
	_ "launchpad.net/juju-core/environs/ec2"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/worker"
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
	a.Conf.addFlags(f)
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
	if err := a.Conf.read(state.MachineEntityName(a.MachineId)); err != nil {
		return err
	}
	defer log.Printf("cmd/jujud: machine agent exiting")
	defer a.tomb.Done()

	apiDone := make(chan error)
	// Pass a copy of the API configuration to maybeRunAPIServer
	// so that it can mutate it independently.
	conf := *a.Conf.Conf
	go func() {
		apiDone <- a.maybeRunAPIServer(&conf)
	}()
	runLoopDone := make(chan error)
	go func() {
		runLoopDone <- RunAgentLoop(a.Conf.Conf, a)
	}()
	var err error
	for apiDone != nil || runLoopDone != nil {
		var err1 error
		select {
		case err1 = <-apiDone:
			apiDone = nil
		case err1 = <-runLoopDone:
			runLoopDone = nil
		}
		a.tomb.Kill(err1)
		if moreImportant(err1, err) {
			err = err1
			log.Printf("%q > %q", err1, err)
		} else {
			log.Printf("%q <= %q", err1, err)
		}
	}
	if err == worker.ErrDead {
		err = nil
	}
	if ug, ok := err.(*UpgradeReadyError); ok {
		if err1 := ug.ChangeAgentTools(); err1 != nil {
			err = err1
			// Return and let upstart deal with the restart.
		}
	}
	return err
}

func (a *MachineAgent) RunOnce(st *state.State, e AgentState) error {
	m := e.(*state.Machine)
	log.Printf("cmd/jujud: jobs for machine agent: %v", m.Jobs())
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
		case state.JobServeAPI:
			continue
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

// maybeStartAPIServer starts the API server if necessary.
func (a *MachineAgent) maybeRunAPIServer(conf *agent.Conf) error {
	return runLoop(func() error {
		return a.maybeRunAPIServerOnce(conf)
	}, a.tomb.Dying())
}

func (a *MachineAgent) maybeRunAPIServerOnce(conf *agent.Conf) error {
	st, entity, err := openState(conf, a)
	if err != nil {
		return err
	}
	defer st.Close()
	m := entity.(*state.Machine)
	runAPI := false
	for _, job := range m.Jobs() {
		if job == state.JobServeAPI {
			runAPI = true
		}
	}
	if !runAPI {
		// If we don't need to run the API, then we just hang
		// around indefinitely until asked to stop.
		<-a.tomb.Dying()
		return nil
	}
	// If the configuration does not have the required information,
	// it is currently not a recoverable error, so we kill the whole
	// agent, potentially enabling human intervention to fix
	// the agent's configuration file. In the future, we may retrieve
	// the state server certificate and key from the state, and
	// this should then change.
	if len(conf.StateServerCert) == 0 || len(conf.StateServerKey) == 0 {
		return &fatalError{"configuration does not have state server cert/key"}
	}
	log.Printf("cmd/jujud: running API server job")
	srv, err := api.NewServer(st, conf.APIInfo.Addr, conf.StateServerCert, conf.StateServerKey)
	if err != nil {
		return err
	}
	select {
	case <-a.tomb.Dying():
	case <-srv.Dead():
	}
	return srv.Stop()
}
