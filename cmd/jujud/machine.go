package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	_ "launchpad.net/juju-core/environs/ec2"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/worker/firewaller"
	"launchpad.net/juju-core/environs/agent"
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

	apiDone, err := a.maybeStartAPIServer()
	if err != nil {
		if err == worker.ErrDead {
			return nil
		}
		return err
	}
	runLoopDone := make(chan error, 1)
	go func() {
		runLoopDone <- RunLoop(a.Conf.Conf, a)
	}()
	for apiDone != nil || runLoopDone != nil {
		var err error
		select{
		case err = <-apiDone:
			apiDone = nil
		case err = <-runLoopDone:
			runLoopDone = nil
		}
		a.tomb.Kill(err)
	}
	return a.tomb.Err()
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

// maybeStartAPIServer starts the API server if it needs to run.
// If it does not need to run, it returns a nil done channel.
func (a *MachineAgent) maybeStartAPIServer() (apiDone <-chan error, err error) {
	// Note: the initial state connection runs synchronously because
	// things will go wrong if we're concurrently modifying
	// the password.

	// First determine if we should run an API server
	// by opening the state and looking at the machine's jobs.
	var st *state.State
	var m *state.Machine
	for {
		st0, entity, err := openState(a.Conf.Conf, a)
		if err == worker.ErrDead {
			return nil, err
		}
		if err == nil {
			st = st0
			m = entity.(*state.Machine)
			break
		}
		log.Printf("cmd/jujud: %v", err)
		if !isleep(retryDelay, a.tomb.Dying()) {
			return nil, tomb.ErrDying
		}
	}
	runAPI := false
	for _, job := range m.Jobs() {
		if job == state.JobServeAPI {
			runAPI = true
		}
	}
	if !runAPI {
		st.Close()
		return nil, nil
	}
	// Use a copy of the configuration so that we're independent.
	conf := *a.Conf.Conf
	if len(conf.StateServerCert) == 0 || len(conf.StateServerKey) == 0 {
		return nil, fmt.Errorf("configuration does not have state server cert/key")
	}
	if conf.APIInfo.Addr == "" {
		return nil, fmt.Errorf("configuration does not have API server address")
	}
	log.Printf("cmd/jujud: running API server job")
	done := make(chan error, 1)
	go func() {
		done <- a.apiServer(st, &conf)
	}()
	return done, nil
}

func (a *MachineAgent) apiServer(st *state.State, conf *agent.Conf) error {
	defer func() {
		if st != nil {
			st.Close()
		}
	}()
	for {
		srv, err := api.NewServer(st, conf.APIInfo.Addr, conf.StateServerCert, conf.StateServerKey)
		if err != nil {
			st.Close()
			return err
		}
		select{
		case <-a.tomb.Dying():
			return srv.Stop()
		case <-srv.Dead():
			log.Printf("cmd/jujud: api server died: %v", srv.Stop())
		}
		if !isleep(retryDelay, a.tomb.Dying()) {
			return tomb.ErrDying
		}
	}
	panic("unreachable")
}
