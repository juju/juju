package main

import (
	"time"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/juju/cmd"
	"launchpad.net/juju-core/juju/environs"
	"launchpad.net/juju-core/juju/log"
	"launchpad.net/juju-core/juju/state"
	"launchpad.net/tomb"

	// register providers
	_ "launchpad.net/juju-core/juju/environs/dummy"
	_ "launchpad.net/juju-core/juju/environs/ec2"
)

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

// lengthyAttempt defines a strategy to retry 
// every 10 seconds indefinitely.
var lengthyAttempt = environs.AttemptStrategy{
	Total: 365 * 24 * time.Hour,
	Delay: 10 * time.Second,
}

// Run runs a provisioning agent.
func (a *ProvisioningAgent) Run(_ *cmd.Context) error {
	var err error
	for attempt := lengthyAttempt.Start(); attempt.Next(); {
		p, err := NewProvisioner(&a.Conf.StateInfo)
		if err != nil {
			log.Printf("provisioner could not connect to zookeeper: %v", err)
			continue
		}
		err = p.Wait()
		if err == nil {
			// impossible at this point
			log.Printf("provisioner exiting")
			break
		}
		log.Printf("provisioner reported error, retrying: %v", err)
	}
	return err
}

type Provisioner struct {
	st      *state.State
	environ environs.Environ
	tomb    tomb.Tomb

	environWatcher  *state.ConfigWatcher
	machinesWatcher *state.MachinesWatcher
}

// NewProvisioner returns a Provisioner.
func NewProvisioner(info *state.Info) (*Provisioner, error) {
	st, err := state.Open(info)
	if err != nil {
		return nil, err
	}
	p := &Provisioner{
		st: st,
	}
	go p.loop()
	return p, nil
}

func (p *Provisioner) loop() {
	defer p.tomb.Done()
	defer p.st.Close()

	p.environWatcher = p.st.WatchEnvironConfig()
	// TODO(dfc) we need a method like state.IsConnected() here to exit cleanly if
	// there is a connection problem.
	for {
		select {
		case <-p.tomb.Dying():
			return
		case config, ok := <-p.environWatcher.Changes():
			if !ok {
				err := p.environWatcher.Stop()
				if err != nil {
					p.tomb.Kill(err)
				}
				return
			}
			var err error
			p.environ, err = environs.NewEnviron(config.Map())
			if err != nil {
				log.Printf("provisioner loaded invalid environment configuration: %v", err)
				continue
			}
			log.Printf("provisioner loaded new environment configuration")
			p.innerLoop()
		}
	}
}

func (p *Provisioner) innerLoop() {
	p.machinesWatcher = p.st.WatchMachines()
	// TODO(dfc) we need a method like state.IsConnected() here to exit cleanly if
	// there is a connection problem.
	for {
		select {
		case <-p.tomb.Dying():
			return
		case change, ok := <-p.environWatcher.Changes():
			if !ok {
				err := p.environWatcher.Stop()
				if err != nil {
					p.tomb.Kill(err)
				}
				return
			}
			config, err := environs.NewConfig(change.Map())
			if err != nil {
				log.Printf("provisioner loaded invalid environment configuration: %v", err)
				continue
			}
			p.environ.SetConfig(config)
			log.Printf("provisioner loaded new environment configuration")
		case machines, ok := <-p.machinesWatcher.Changes():
			if !ok {
				err := p.machinesWatcher.Stop()
				if err != nil {
					p.tomb.Kill(err)
				}
				return
			}
			p.processMachines(machines)
		}
	}
}

// Wait waits for the Provisioner to exit.
func (p *Provisioner) Wait() error {
	return p.tomb.Wait()
}

// Stop stops the Provisioner and returns any error encountered while
// provisioning.
func (p *Provisioner) Stop() error {
	p.tomb.Kill(nil)
	return p.tomb.Wait()
}

func (p *Provisioner) processMachines(changes *state.MachinesChange) {}
