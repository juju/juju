package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/state"
	"log"
)

type machine struct {
	id string
}

func (m *machine) loop() {
	for {
		
	}
}

// ProvisioningAgent is a cmd.Command responsible for running a provisioning agent.
type ProvisioningAgent struct {
	Conf AgentConf
}

// Info returns usage information for the command.
func (a *ProvisioningAgent) Info() *cmd.Info {
	return &cmd.Info{"provisioning", "", "run a juju provisioning agent", ""}
}

// Init initializes the command for running.
func (a *ProvisioningAgent) Init(f *gnuflag.FlagSet, args []string) error {
	a.Conf.addFlags(f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return a.Conf.checkArgs(f.Args())
}

func (a *ProvisioningAgent) openState() (*state.State, error) {
	return state.Open(&a.Conf.StateInfo)
}

// Run runs a provisioning agent.
func (a *ProvisioningAgent) Run(_ *cmd.Context) error {
	state, err := a.openState()
	if err != nil {
		return err
	}
	w := state.WatchMachine()
	defer w.Stop()

	for {
		event := <- w.Changes
		switch {
		case len(event.Added) > 0:
			log.Printf("Machines added: %v", event.Added)
		case len(event.Deleted) > 0:
			log.Printf("Machines deleted: %v", event.Deleted)
		}
	}
	return nil
}
