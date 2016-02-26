// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/controller/authentication"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
	"github.com/juju/juju/worker/environ"
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
	getMachineWatcher() (watcher.StringsWatcher, error)
	getRetryWatcher() (watcher.NotifyWatcher, error)
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
	catacomb    catacomb.Catacomb
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

// Kill implements worker.Worker.Kill.
func (p *provisioner) Kill() {
	p.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (p *provisioner) Wait() error {
	return p.catacomb.Wait()
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

	envCfg, err := p.st.ModelConfig()
	if err != nil {
		return nil, errors.Annotate(err, "could not retrieve the model config.")
	}

	secureServerConnection := false
	if info, ok := p.agentConfig.StateServingInfo(); ok {
		secureServerConnection = info.CAPrivateKey != ""
	}
	task, err := NewProvisionerTask(
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
	if err != nil {
		return nil, errors.Trace(err)
	}
	return task, nil
}

// NewEnvironProvisioner returns a new Provisioner for an environment.
// When new machines are added to the state, it allocates instances
// from the environment and allocates them to the new machines.
func NewEnvironProvisioner(st *apiprovisioner.State, agentConfig agent.Config) (Provisioner, error) {
	p := &environProvisioner{
		provisioner: provisioner{
			st:          st,
			agentConfig: agentConfig,
			toolsFinder: getToolsFinder(st),
		},
	}
	p.Provisioner = p
	logger.Tracef("Starting environ provisioner for %q", p.agentConfig.Tag())

	err := catacomb.Invoke(catacomb.Plan{
		Site: &p.catacomb,
		Work: p.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return p, nil
}

func (p *environProvisioner) loop() error {
	var modelConfigChanges <-chan struct{}
	modelWatcher, err := p.st.WatchForModelConfigChanges()
	if err != nil {
		return loggedErrorStack(errors.Trace(err))
	}
	if err := p.catacomb.Add(modelWatcher); err != nil {
		return errors.Trace(err)
	}
	modelConfigChanges = modelWatcher.Changes()

	p.environ, err = environ.WaitForEnviron(modelWatcher, p.st, p.catacomb.Dying())
	if err != nil {
		if err == environ.ErrWaitAborted {
			return p.catacomb.ErrDying()
		}
		return loggedErrorStack(errors.Trace(err))
	}
	p.broker = p.environ

	harvestMode := p.environ.Config().ProvisionerHarvestMode()
	task, err := p.getStartTask(harvestMode)
	if err != nil {
		return loggedErrorStack(errors.Trace(err))
	}
	if err := p.catacomb.Add(task); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()
		case _, ok := <-modelConfigChanges:
			if !ok {
				return errors.New("model configuration watcher closed")
			}
			environConfig, err := p.st.ModelConfig()
			if err != nil {
				return errors.Annotate(err, "cannot load model configuration")
			}
			if err := p.setConfig(environConfig); err != nil {
				return errors.Annotate(err, "loaded invalid model configuration")
			}
			task.SetHarvestMode(environConfig.ProvisionerHarvestMode())
		}
	}
}

func (p *environProvisioner) getMachineWatcher() (watcher.StringsWatcher, error) {
	return p.st.WatchModelMachines()
}

func (p *environProvisioner) getRetryWatcher() (watcher.NotifyWatcher, error) {
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
) (Provisioner, error) {

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

	err := catacomb.Invoke(catacomb.Plan{
		Site: &p.catacomb,
		Work: p.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return p, nil
}

func (p *containerProvisioner) loop() error {
	modelWatcher, err := p.st.WatchForModelConfigChanges()
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.catacomb.Add(modelWatcher); err != nil {
		return errors.Trace(err)
	}

	config, err := p.st.ModelConfig()
	if err != nil {
		return err
	}
	harvestMode := config.ProvisionerHarvestMode()

	task, err := p.getStartTask(harvestMode)
	if err != nil {
		return err
	}
	if err := p.catacomb.Add(task); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()
		case _, ok := <-modelWatcher.Changes():
			if !ok {
				return errors.New("model configuratioon watch closed")
			}
			modelConfig, err := p.st.ModelConfig()
			if err != nil {
				return errors.Annotate(err, "cannot load model configuration")
			}
			p.configObserver.notify(modelConfig)
			task.SetHarvestMode(modelConfig.ProvisionerHarvestMode())
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

func (p *containerProvisioner) getMachineWatcher() (watcher.StringsWatcher, error) {
	machine, err := p.getMachine()
	if err != nil {
		return nil, err
	}
	return machine.WatchContainers(p.containerType)
}

func (p *containerProvisioner) getRetryWatcher() (watcher.NotifyWatcher, error) {
	return nil, errors.NotImplementedf("getRetryWatcher")
}
