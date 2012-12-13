package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	_ "launchpad.net/juju-core/environs/ec2"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/firewaller"
	"launchpad.net/juju-core/worker/machiner"
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
	for a.tomb.Err() == tomb.ErrStillAlive {
		log.Printf("cmd/jujud: machine agent starting")
		err := a.runOnce()
		if ug, ok := err.(*UpgradeReadyError); ok {
			if err = ug.ChangeAgentTools(); err == nil {
				// Return and let upstart deal with the restart.
				return ug
			}
		}
		if err == worker.ErrDead {
			log.Printf("cmd/jujud: machine is dead")
			return nil
		}
		if err == nil {
			log.Printf("cmd/jujud: workers died with no error")
		} else {
			log.Printf("cmd/jujud: %v", err)
		}
		select {
		case <-a.tomb.Dying():
			a.tomb.Kill(err)
		case <-time.After(retryDelay):
			log.Printf("cmd/jujud: rerunning machiner")
		}
	}
	return a.tomb.Err()
}

if we've been given a mongodb address, we try
to connect to it to see if our machine is supposed
to run a state worker.

if we fail because we're not authorised, then
we fail altogether, i suppose (no use in getting
the list of workers if we can't run the state server)
but... should we fail altogether if we can still run
some of the other tasks?

func (a *MachineAgent) stateServer() {
	if len(a.MongoAddrs) == 0 {
		return
	}
	st, password, err := openState(state.MachineEntityName(a.MachineId), &a.Conf)
	
	what do we do about the new password?

	get machine
	if workers do not include ServerWorker {
		return
	}
	for {
		go runServer(0
		select {
		case <-serverquit:
			open mongo state
		case <-upgraded:
			srv.Stop()
			done
		}
	}
}

TODO:

get cert and key from data dir
change agent.go to get CA cert from data dir

func (a *MachineAgent) runOnce() error {
	// TODO (when API state is universal): try to open mongo state
	// first, set password with that, then run state server if
	// necessary; then open api and set password with that if
	// necessary.
	st, password, err := openState(state.MachineEntityName(a.MachineId), &a.Conf)
	if err != nil {
		return err
	}
	defer st.Close()
	m, err := st.Machine(a.MachineId)
	if state.IsNotFound(err) || err == nil && m.Life() == state.Dead {
		return worker.ErrDead
	}
	if err != nil {
		return err
	}
	if password != "" {
		if err := m.SetPassword(password); err != nil {
			return err
		}
	}
	log.Printf("cmd/jujud: requested workers for machine agent: ", m.Workers())
	var tasks []task
	// The API server provides a service that may be required
	// to open the API client, so we start it first if it's required.
	for _, w := range m.Workers() {
		if w == state.ServerWorker {
			srv, err := api.NewServer(st, apiAddr, cert, key)
			if err != nil {
				return err
			}
			tasks = append(tasks, t)
		}
	}
	apiSt, err := api.Open(a.APIInfo)
	if err != nil {
		stopc := make(chan struct{})
		close(stopc)
		if err := runTasks(stopc, tasks); err != nil {
			// The API server error is probably more interesting
			// than the API client connection failure.
			return err
		}
		return err
	}
	defer apiSt.Close()
	tasks = append(tasks, NewUpgrader(st, m, a.Conf.DataDir))
	for _, w := range m.Workers() {
		var t task
		switch w {
		case state.MachinerWorker:
			t = machiner.NewMachiner(m, &a.Conf.StateInfo, a.Conf.DataDir)
		case state.ProvisionerWorker:
			t = provisioner.NewProvisioner(st)
		case state.FirewallerWorker:
			t = firewaller.NewFirewaller(st)
		case state.ServerWorker:
			continue
		}
		if t == nil {
			log.Printf("cmd/jujud: ignoring unknown worker %q", w)
			continue
		}
		tasks = append(tasks, t)
	}
	return runTasks(a.tomb.Dying(), tasks...)
}
