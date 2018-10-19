// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/controller/authentication"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/worker/common"
)

var logger = loggo.GetLogger("juju.provisioner")

// Ensure our structs implement the required Provisioner interface.
var _ Provisioner = (*environProvisioner)(nil)
var _ Provisioner = (*containerProvisioner)(nil)

var (
	retryStrategyDelay = 10 * time.Second
	retryStrategyCount = 10
)

// Provisioner represents a running provisioner worker.
type Provisioner interface {
	worker.Worker
	getMachineWatcher() (watcher.StringsWatcher, error)
	getRetryWatcher() (watcher.NotifyWatcher, error)
	getProfileWatcher() (watcher.StringsWatcher, error)
}

// environProvisioner represents a running provisioning worker for machine nodes
// belonging to an environment.
type environProvisioner struct {
	provisioner
	environ        environs.Environ
	configObserver configObserver
}

// containerProvisioner represents a running provisioning worker for containers
// hosted on a machine.
type containerProvisioner struct {
	provisioner
	containerType  instance.ContainerType
	machine        apiprovisioner.MachineProvisioner
	configObserver configObserver
}

// provisioner providers common behaviour for a running provisioning worker.
type provisioner struct {
	Provisioner
	st                      *apiprovisioner.State
	agentConfig             agent.Config
	broker                  environs.InstanceBroker
	distributionGroupFinder DistributionGroupFinder
	toolsFinder             ToolsFinder
	catacomb                catacomb.Catacomb
	callContext             context.ProviderCallContext
}

// RetryStrategy defines the retry behavior when encountering a retryable
// error during provisioning.
//
// TODO(katco): 2016-08-09: lp:1611427
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

// configObserver is implemented so that tests can see when the environment
// configuration changes.
// The catacomb is set in export_test to the provider's member.
// This is used to prevent notify from blocking a provisioner that has had its
// Kill method invoked.
type configObserver struct {
	sync.Mutex
	observer chan<- *config.Config
	catacomb *catacomb.Catacomb
}

// notify notifies the observer of a configuration change.
func (o *configObserver) notify(cfg *config.Config) {
	o.Lock()
	if o.observer != nil {
		select {
		case o.observer <- cfg:
		case <-o.catacomb.Dying():
		}
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

// getDistributionGroupFinder returns a DistributionGroupFinder
// for the provided State. This exists for mocking.
var getDistributionGroupFinder = func(st *apiprovisioner.State) DistributionGroupFinder {
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
	profileWatcher, err := p.getProfileWatcher()
	if err != nil {
		return nil, err
	}
	tag := p.agentConfig.Tag()
	machineTag, ok := tag.(names.MachineTag)
	if !ok {
		errors.Errorf("expected names.MachineTag, got %T", tag)
	}

	modelCfg, err := p.st.ModelConfig()
	if err != nil {
		return nil, errors.Annotate(err, "could not retrieve the model config.")
	}

	controllerCfg, err := p.st.ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "could not retrieve the controller config.")
	}

	task, err := NewProvisionerTask(
		controllerCfg.ControllerUUID(),
		machineTag,
		harvestMode,
		p.st,
		p.distributionGroupFinder,
		p.toolsFinder,
		machineWatcher,
		retryWatcher,
		profileWatcher,
		p.broker,
		auth,
		modelCfg.ImageStream(),
		RetryStrategy{retryDelay: retryStrategyDelay, retryCount: retryStrategyCount},
		p.callContext,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return task, nil
}

// NewEnvironProvisioner returns a new Provisioner for an environment.
// When new machines are added to the state, it allocates instances
// from the environment and allocates them to the new machines.
func NewEnvironProvisioner(st *apiprovisioner.State,
	agentConfig agent.Config,
	environ environs.Environ,
	credentialAPI common.CredentialAPI,
) (Provisioner, error) {
	p := &environProvisioner{
		provisioner: provisioner{
			st:                      st,
			agentConfig:             agentConfig,
			toolsFinder:             getToolsFinder(st),
			distributionGroupFinder: getDistributionGroupFinder(st),
			callContext:             common.NewCloudCallContext(credentialAPI),
		},
		environ: environ,
	}
	p.Provisioner = p
	p.broker = environ
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
	// TODO(mjs channeling axw) - It would be better if there were
	// APIs to watch and fetch provisioner specific config instead of
	// watcher for all changes to model config. This would avoid the
	// need for a full model config.
	var modelConfigChanges <-chan struct{}
	modelWatcher, err := p.st.WatchForModelConfigChanges()
	if err != nil {
		return loggedErrorStack(errors.Trace(err))
	}
	if err := p.catacomb.Add(modelWatcher); err != nil {
		return errors.Trace(err)
	}
	modelConfigChanges = modelWatcher.Changes()

	modelConfig := p.environ.Config()
	p.configObserver.notify(modelConfig)
	harvestMode := modelConfig.ProvisionerHarvestMode()
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
			modelConfig, err := p.st.ModelConfig()
			if err != nil {
				return errors.Annotate(err, "cannot load model configuration")
			}
			if err := p.setConfig(modelConfig); err != nil {
				return errors.Annotate(err, "loaded invalid model configuration")
			}
			task.SetHarvestMode(modelConfig.ProvisionerHarvestMode())
		}
	}
}

func (p *environProvisioner) getMachineWatcher() (watcher.StringsWatcher, error) {
	return p.st.WatchModelMachines()
}

func (p *environProvisioner) getRetryWatcher() (watcher.NotifyWatcher, error) {
	return p.st.WatchMachineErrorRetry()
}

func (p *environProvisioner) getProfileWatcher() (watcher.StringsWatcher, error) {
	return p.st.WatchModelMachinesCharmProfiles()
}

// setConfig updates the environment configuration and notifies
// the config observer.
func (p *environProvisioner) setConfig(modelConfig *config.Config) error {
	if err := p.environ.SetConfig(modelConfig); err != nil {
		return errors.Trace(err)
	}
	p.configObserver.notify(modelConfig)
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
	distributionGroupFinder DistributionGroupFinder,
	credentialAPI common.CredentialAPI,
) (Provisioner, error) {
	p := &containerProvisioner{
		provisioner: provisioner{
			st:                      st,
			agentConfig:             agentConfig,
			broker:                  broker,
			toolsFinder:             toolsFinder,
			distributionGroupFinder: distributionGroupFinder,
			callContext:             common.NewCloudCallContext(credentialAPI),
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

	modelConfig, err := p.st.ModelConfig()
	if err != nil {
		return errors.Trace(err)
	}
	p.configObserver.notify(modelConfig)
	harvestMode := modelConfig.ProvisionerHarvestMode()

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
		case _, ok := <-modelWatcher.Changes():
			if !ok {
				return errors.New("model configuration watch closed")
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

func (p *containerProvisioner) getMachine() (apiprovisioner.MachineProvisioner, error) {
	if p.machine == nil {
		tag := p.agentConfig.Tag()
		machineTag, ok := tag.(names.MachineTag)
		if !ok {
			return nil, errors.Errorf("expected names.MachineTag, got %T", tag)
		}
		result, err := p.st.Machines(machineTag)
		if err != nil {
			logger.Errorf("error retrieving %s from state", machineTag)
			return nil, err
		}
		if result[0].Err != nil {
			logger.Errorf("%s is not in state", machineTag)
			return nil, err
		}
		p.machine = result[0].Machine
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

func (p *containerProvisioner) getProfileWatcher() (watcher.StringsWatcher, error) {
	if p.containerType != instance.LXD {
		// TODO (hml) lxd-profile 17-oct-2018
		// what is the correct way to handle this, only start for LXD containers
	}
	machine, err := p.getMachine()
	if err != nil {
		return nil, err
	}
	return machine.WatchContainersCharmProfiles(p.containerType)
}
