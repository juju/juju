package main

import (
	"time"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/log"

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

// Run runs a provisioning agent.
func (a *ProvisioningAgent) Run(_ *cmd.Context) error {
	for {
		p, err := NewProvisioner(&a.Conf.StateInfo)
		if err == nil {
			if err = p.Wait(); err == nil {
				// if Wait returns nil then we consider that a signal
				// that the process should exit the retry logic.
				return nil
			}
		}
		log.Printf("restarting provisioner after error: %v", err)
		time.Sleep(retryDuration)
	}
	panic("unreachable")
}
