package main

import (
	"time"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker/firewaller"
	"launchpad.net/juju-core/worker/provisioner"

	// register providers
	_ "launchpad.net/juju-core/environs/ec2"
)

var retryDuration = 10 * time.Second

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

// Run runs a provisioning agent with restarting after an error.
func (a *ProvisioningAgent) Run(_ *cmd.Context) error {
	for {
		if err := a.run(); err != nil {
			time.Sleep(retryDuration)
			log.Printf("restarting provisioner and firewaller after error: %v", err)
		}
	}
	panic("unreachable")
}

// run runs a provisioning agent once.
func (a *ProvisioningAgent) run() (err error) {
	st, err := state.Open(&a.Conf.StateInfo)
	if err != nil {
		return err
	}
	defer func() {
		if e := st.Close(); err != nil {
			err = e
		}
	}()

	p, err := provisioner.NewProvisioner(st)
	if err != nil {
		return err
	}
	defer func() {
		if e := p.Stop(); err != nil {
			err = e
		}
	}()

	fw, err := firewaller.NewFirewaller(st)
	if err != nil {
		return err
	}
	defer func() {
		if e := fw.Stop(); err != nil {
			err = e
		}
	}()

	select {
	case <-p.Dying():
	case <-fw.Dying():
	}

	return
}
