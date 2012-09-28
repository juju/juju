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
	MachineId int
}

// Info returns usage information for the command.
func (a *MachineAgent) Info() *cmd.Info {
	return &cmd.Info{"machine", "", "run a juju machine agent", ""}
}

// Init initializes the command for running.
func (a *MachineAgent) Init(f *gnuflag.FlagSet, args []string) error {
	a.Conf.addFlags(f)
	f.IntVar(&a.MachineId, "machine-id", -1, "id of the machine to run")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if a.MachineId < 0 {
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
	defer log.Printf("machine agent exiting")
	defer a.tomb.Done()
	for a.tomb.Err() == tomb.ErrStillAlive {
		log.Printf("machine agent starting")
		err := a.runOnce()
		if ug, ok := err.(*UpgradeReadyError); ok {
			if err = ug.ChangeAgentTools(); err == nil {
				// Return and let upstart deal with the restart.
				return ug
			}
		}
		if err == worker.ErrDead {
			log.Printf("uniter: machine is dead")
			return nil
		}
		if err == nil {
			log.Printf("machiner: workers died with no error")
		} else {
			log.Printf("machiner: %v", err)
		}
		select {
		case <-a.tomb.Dying():
			a.tomb.Kill(err)
		case <-time.After(retryDelay):
			log.Printf("rerunning machiner")
		}
	}
	return a.tomb.Err()
}

func (a *MachineAgent) runOnce() error {
	st, err := state.Open(&a.Conf.StateInfo)
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
	log.Printf("machine agent running tasks: %v", m.Workers())
	tasks := []task{NewUpgrader(st, m, a.Conf.DataDir)}
	for _, w := range m.Workers() {
		var t task
		switch w {
		case state.MachinerWorker:
			t = machiner.NewMachiner(m, &a.Conf.StateInfo, a.Conf.DataDir)
		case state.ProvisionerWorker:
			t = provisioner.NewProvisioner(st)
		case state.FirewallerWorker:
			t = firewaller.NewFirewaller(st)
		}
		if t == nil {
			log.Printf("ignoring unknown worker task %q", w)
			continue
		}
		tasks = append(tasks, t)
	}
	log.Printf("final tasks: %#v", tasks)
	return runTasks(a.tomb.Dying(), tasks...)
}
