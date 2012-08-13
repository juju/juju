package main

import (
	"time"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker/firewaller"
	"launchpad.net/juju-core/worker/provisioner"
	"launchpad.net/tomb"

	// register providers
	_ "launchpad.net/juju-core/environs/ec2"
)

var retryDuration = 3 * time.Second

// ProvisioningAgent is a cmd.Command responsible for running a provisioning agent.
type ProvisioningAgent struct {
	tomb        tomb.Tomb
	Conf        AgentConf
	provisioner *provisioner.Provisioner
	firewaller  *firewaller.Firewaller
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

// Stop stops the provisionig agent by stopping the provisioner
// and the firewaller.
func (a *ProvisioningAgent) Stop() error {
	a.tomb.Kill(nil)
	return a.tomb.Wait()
}

// Run run a provisioning agent with a provisioner and a firewaller.
// If either fails, both will be shutdown and restarted.
func (a *ProvisioningAgent) Run(_ *cmd.Context) (err error) {
	defer a.tomb.Done()
	for {
		err = a.runOnce()
		if a.tomb.Err() != tomb.ErrStillAlive {
			// Stop requested by user.
			return err
		}
		time.Sleep(retryDuration)
		log.Printf("restarting provisioner and firewaller after error: %v", err)
	}
	panic("unreachable")
}

// runOnce runs a provisioner and firewaller once.
func (a *ProvisioningAgent) runOnce() (stoperr error) {
	st, err := state.Open(&a.Conf.StateInfo)
	if err != nil {
		return err
	}
	log.Debugf("provisioning: opened state")
	defer func() {
		if e := st.Close(); err != nil {
			stoperr = e
		}
		log.Debugf("provisioning: closed state")
	}()

	a.provisioner, err = provisioner.NewProvisioner(st)
	if err != nil {
		return err
	}
	log.Debugf("provisioning: started provisioner")
	defer func() {
		if e := a.provisioner.Stop(); err != nil {
			stoperr = e
		}
		log.Debugf("provisioning: stopped provisioner")
	}()

	a.firewaller, err = firewaller.NewFirewaller(st)
	if err != nil {
		return err
	}
	log.Debugf("provisioning: started firewaller")
	defer func() {
		if e := a.firewaller.Stop(); err != nil {
			stoperr = e
		}
		log.Debugf("provisioning: stopped firewaller")
	}()

	select {
	case <-a.tomb.Dying():
	case <-a.provisioner.Dying():
	case <-a.firewaller.Dying():
	}

	return
}
