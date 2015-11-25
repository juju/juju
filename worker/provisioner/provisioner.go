// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.provisioner")

// Ensure our structs implement the required Provisioner interface.
var _ Provisioner = (*environProvisioner)(nil)
var _ Provisioner = (*containerProvisioner)(nil)

var (
	retryStrategyDelay = 10 * time.Second
	retryStrategyCount = 3
)

// Provisioner represents a running provisioner worker.
type Provisioner interface {
	worker.Worker
	Stop() error
	getMachineWatcher() (apiwatcher.StringsWatcher, error)
	getRetryWatcher() (apiwatcher.NotifyWatcher, error)
}

// environProvisioner represents a running provisioning worker for machine nodes
// belonging to an environment.
type environProvisioner struct {
	provisioner
	environ environs.Environ
	configObserver
}

// containerProvisioner represents a running provisioning worker for containers
// hosted on a machine.
type containerProvisioner struct {
	provisioner
	containerType instance.ContainerType
	machine       *apiprovisioner.Machine
	configObserver
}

// provisioner providers common behaviour for a running provisioning worker.
type provisioner struct {
	Provisioner
	st          *apiprovisioner.State
	agentConfig agent.Config
	broker      environs.InstanceBroker
	toolsFinder ToolsFinder
	tomb        tomb.Tomb
}

// RetryStrategy defines the retry behavior when encountering a retryable
// error during provisioning.
type RetryStrategy struct {
	retryDelay time.Duration
	retryCount int
}

// NewRetryStrategy returns a new retry strategy with the specified delay and
// count for use with retryable provisioning errors.
func NewRetryStrategy(delay time.Duration, count int) RetryStrategy {
	return RetryStrategy{
		retryDelay: delay,
		retryCount: count,
	}
}

// configObserver is implemented so that tests can see
// when the environment configuration changes.
type configObserver struct {
	sync.Mutex
	observer chan<- *config.Config
}

// notify notifies the observer of a configuration change.
func (o *configObserver) notify(cfg *config.Config) {
	o.Lock()
	if o.observer != nil {
		o.observer <- cfg
	}
	o.Unlock()
}

// Err returns the reason why the provisioner has stopped or tomb.ErrStillAlive
// when it is still alive.
func (p *provisioner) Err() (reason error) {
	return p.tomb.Err()
}

// Kill implements worker.Worker.Kill.
func (p *provisioner) Kill() {
	p.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (p *provisioner) Wait() error {
	return p.tomb.Wait()
}

// Stop stops the provisioner and returns any error encountered while
// provisioning.
func (p *provisioner) Stop() error {
	p.tomb.Kill(nil)
	return p.tomb.Wait()
}

// getToolsFinder returns a ToolsFinder for the provided State.
// This exists for mocking.
var getToolsFinder = func(st *apiprovisioner.State) ToolsFinder {
	return st
}

// getStartTask creates a new worker for the provisioner,
func (p *provisioner) getStartTask(harvestMode config.HarvestMode) (ProvisionerTask, error) {
	auth, err := authentication.NewAPIAuthenticator(p.st)
	if err != nil {
		return nil, err
	}
	// Start responding to changes in machines, and to any further updates
	// to the environment config.
	machineWatcher, err := p.getMachineWatcher()
	if err != nil {
		return nil, err
	}
	retryWatcher, err := p.getRetryWatcher()
	if err != nil && !errors.IsNotImplemented(err) {
		return nil, err
	}
	tag := p.agentConfig.Tag()
	machineTag, ok := tag.(names.MachineTag)
	if !ok {
		errors.Errorf("expacted names.MachineTag, got %T", tag)
	}

	envCfg, err := p.st.EnvironConfig()
	if err != nil {
		return nil, errors.Annotate(err, "could not retrieve the environment config.")
	}

	secureServerConnection := false
	if info, ok := p.agentConfig.StateServingInfo(); ok {
		secureServerConnection = info.CAPrivateKey != ""
	}
	task := NewProvisionerTask(
		machineTag,
		harvestMode,
		p.st,
		p.toolsFinder,
		machineWatcher,
		retryWatcher,
		p.broker,
		auth,
		envCfg.ImageStream(),
		secureServerConnection,
		RetryStrategy{retryDelay: retryStrategyDelay, retryCount: retryStrategyCount},
	)
	return task, nil
}

// NewEnvironProvisioner returns a new Provisioner for an environment.
// When new machines are added to the state, it allocates instances
// from the environment and allocates them to the new machines.
func NewEnvironProvisioner(st *apiprovisioner.State, agentConfig agent.Config) Provisioner {
	p := &environProvisioner{
		provisioner: provisioner{
			st:          st,
			agentConfig: agentConfig,
			toolsFinder: getToolsFinder(st),
		},
	}
	p.Provisioner = p
	logger.Tracef("Starting environ provisioner for %q", p.agentConfig.Tag())
	go func() {
		defer p.tomb.Done()
		p.tomb.Kill(errors.Cause(p.loop()))
	}()
	return p
}

func (p *environProvisioner) loop() error {
	var environConfigChanges <-chan struct{}
	environWatcher, err := p.st.WatchForEnvironConfigChanges()
	if err != nil {
		return loggedErrorStack(errors.Trace(err))
	}
	environConfigChanges = environWatcher.Changes()
	defer watcher.Stop(environWatcher, &p.tomb)

	p.environ, err = worker.WaitForEnviron(environWatcher, p.st, p.tomb.Dying())
	if err != nil {
		return loggedErrorStack(errors.Trace(err))
	}
	p.broker = p.environ

	harvestMode := p.environ.Config().ProvisionerHarvestMode()
	task, err := p.getStartTask(harvestMode)
	if err != nil {
		return loggedErrorStack(errors.Trace(err))
	}
	defer watcher.Stop(task, &p.tomb)

	for {
		select {
		case <-p.tomb.Dying():
			return tomb.ErrDying
		case <-task.Dying():
			err := task.Err()
			logger.Errorf("environ provisioner died: %v", err)
			return err
		case _, ok := <-environConfigChanges:
			if !ok {
				return watcher.EnsureErr(environWatcher)
			}
			environConfig, err := p.st.EnvironConfig()
			if err != nil {
				logger.Errorf("cannot load environment configuration: %v", err)
				return err
			}
			if err := p.setConfig(environConfig); err != nil {
				logger.Errorf("loaded invalid environment configuration: %v", err)
			}
			task.SetHarvestMode(environConfig.ProvisionerHarvestMode())
		}
	}
}

func (p *environProvisioner) getMachineWatcher() (apiwatcher.StringsWatcher, error) {
	return p.st.WatchEnvironMachines()
}

func (p *environProvisioner) getRetryWatcher() (apiwatcher.NotifyWatcher, error) {
	return p.st.WatchMachineErrorRetry()
}

// setConfig updates the environment configuration and notifies
// the config observer.
func (p *environProvisioner) setConfig(environConfig *config.Config) error {
	if err := p.environ.SetConfig(environConfig); err != nil {
		return err
	}
	p.configObserver.notify(environConfig)
	return nil
}

// NewContainerProvisioner returns a new Provisioner. When new machines
// are added to the state, it allocates instances from the environment
// and allocates them to the new machines.
func NewContainerProvisioner(
	containerType instance.ContainerType,
	st *apiprovisioner.State,
	agentConfig agent.Config,
	broker environs.InstanceBroker,
	toolsFinder ToolsFinder,
) Provisioner {

	p := &containerProvisioner{
		provisioner: provisioner{
			st:          st,
			agentConfig: agentConfig,
			broker:      broker,
			toolsFinder: toolsFinder,
		},
		containerType: containerType,
	}
	p.Provisioner = p
	logger.Tracef("Starting %s provisioner for %q", p.containerType, p.agentConfig.Tag())
	go func() {
		defer p.tomb.Done()
		p.tomb.Kill(p.loop())
	}()
	return p
}

func (p *containerProvisioner) loop() error {
	var environConfigChanges <-chan struct{}
	environWatcher, err := p.st.WatchForEnvironConfigChanges()
	if err != nil {
		return err
	}
	environConfigChanges = environWatcher.Changes()
	defer watcher.Stop(environWatcher, &p.tomb)

	config, err := p.st.EnvironConfig()
	if err != nil {
		return err
	}
	harvestMode := config.ProvisionerHarvestMode()

	task, err := p.getStartTask(harvestMode)
	if err != nil {
		return err
	}
	defer watcher.Stop(task, &p.tomb)

	for {
		select {
		case <-p.tomb.Dying():
			return tomb.ErrDying
		case <-task.Dying():
			err := task.Err()
			logger.Errorf("%s provisioner died: %v", p.containerType, err)
			return err
		case _, ok := <-environConfigChanges:
			if !ok {
				return watcher.EnsureErr(environWatcher)
			}
			environConfig, err := p.st.EnvironConfig()
			if err != nil {
				logger.Errorf("cannot load environment configuration: %v", err)
				return err
			}
			p.configObserver.notify(environConfig)
			task.SetHarvestMode(environConfig.ProvisionerHarvestMode())
		}
	}
}

func (p *containerProvisioner) getMachine() (*apiprovisioner.Machine, error) {
	if p.machine == nil {
		tag := p.agentConfig.Tag()
		machineTag, ok := tag.(names.MachineTag)
		if !ok {
			return nil, errors.Errorf("expected names.MachineTag, got %T", tag)
		}
		var err error
		if p.machine, err = p.st.Machine(machineTag); err != nil {
			logger.Errorf("%s is not in state", machineTag)
			return nil, err
		}
	}
	return p.machine, nil
}

func (p *containerProvisioner) getMachineWatcher() (apiwatcher.StringsWatcher, error) {
	machine, err := p.getMachine()
	if err != nil {
		return nil, err
	}
	return machine.WatchContainers(p.containerType)
}

func (p *containerProvisioner) getRetryWatcher() (apiwatcher.NotifyWatcher, error) {
	return nil, errors.NotImplementedf("getRetryWatcher")
}
