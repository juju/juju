// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs

import (
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs/ecsiface"
	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	cloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	envcontext "github.com/juju/juju/environs/context"
)

type environ struct {
	name        string
	clusterName string

	clock jujuclock.Clock

	// modelUUID is the UUID of the model this client acts on.
	modelUUID      string
	controllerUUID string

	lock           sync.Mutex
	envCfgUnlocked *config.Config
	awsCfgUnlocked *aws.Config

	clientUnlocked ecsiface.ECSAPI
	newECSClient   newECSClientFunc
}

type newECSClientFunc func(*aws.Config) (ecsiface.ECSAPI, error)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/ecs_mock.go github.com/aws/aws-sdk-go/service/ecs/ecsiface ECSAPI
func newEnviron(
	controllerUUID string,
	clusterName string,
	clock jujuclock.Clock,
	envCfg *config.Config,
	awsCfg *aws.Config,
	newECSClient func(*aws.Config) (ecsiface.ECSAPI, error),
) (_ *environ, err error) {
	newCfg, err := providerInstance.newConfig(envCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelUUID := newCfg.UUID()
	if modelUUID == "" {
		return nil, errors.NotValidf("modelUUID is required")
	}

	env := &environ{
		name:           envCfg.Name(),
		clusterName:    clusterName,
		clock:          clock,
		modelUUID:      modelUUID,
		controllerUUID: controllerUUID,
		envCfgUnlocked: envCfg,
		awsCfgUnlocked: awsCfg,
		newECSClient:   newECSClient,
	}
	if env.clientUnlocked, err = newECSClient(awsCfg); err != nil {
		return nil, errors.Trace(err)
	}
	return env, nil
}

func (env *environ) client() ecsiface.ECSAPI {
	env.lock.Lock()
	defer env.lock.Unlock()
	client := env.clientUnlocked
	return client
}

// APIVersion returns the version info for the cluster.
func (env *environ) APIVersion() (string, error) {
	// TODO(ecs)
	return "", nil
}

// Version returns cluster version information.
func (env *environ) Version() (ver *version.Number, err error) {
	// TODO(ecs)
	return nil, nil
}

// Provider is part of the Broker interface.
func (*environ) Provider() caas.ContainerEnvironProvider {
	return providerInstance
}

// SetCloudSpec is specified in the environs.Environ interface.
func (env *environ) SetCloudSpec(spec cloudspec.CloudSpec) (err error) {
	env.lock.Lock()
	defer env.lock.Unlock()

	// env.clusterName = ???

	if env.awsCfgUnlocked, err = cloudSpecToAWSConfig(spec); err != nil {
		return errors.Annotate(err, "validating cloud spec")
	}
	if env.clientUnlocked, err = newECSClient(env.awsCfgUnlocked); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Config returns environ config.
func (env *environ) Config() *config.Config {
	env.lock.Lock()
	defer env.lock.Unlock()
	cfg := env.envCfgUnlocked
	return cfg
}

// SetConfig is specified in the Environ interface.
func (env *environ) SetConfig(cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()
	newCfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return errors.Trace(err)
	}
	env.name = newCfg.Config.Name()
	env.envCfgUnlocked = newCfg.Config
	return nil
}

// CheckCloudCredentials verifies the the cloud credentials provided to the
// broker are functioning.
func (env *environ) CheckCloudCredentials() error {
	// TODO(ecs)
	return nil
}

// AnnotateUnit annotates the specified pod (name or uid) with a unit tag.
func (env *environ) AnnotateUnit(appName string, mode caas.DeploymentMode, podName string, unit names.UnitTag) error {
	// TODO(ecs)
	return nil
}

// PrepareForBootstrap prepares for bootstraping a controller.
func (env *environ) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	// TODO(ecs)
	return nil
}

// Bootstrap deploys controller with mongoDB together into ecs cluster.
func (env *environ) Bootstrap(
	ctx environs.BootstrapContext, callCtx envcontext.ProviderCallContext, args environs.BootstrapParams,
) (*environs.BootstrapResult, error) {
	// TODO(ecs)
	return nil, nil
}

// Create implements environs.BootstrapEnviron.
func (env *environ) Create(envcontext.ProviderCallContext, environs.CreateParams) error {
	return nil
}

func (env *environ) CurrentModel() string {
	env.lock.Lock()
	defer env.lock.Unlock()
	// TODO(ecs): remove from caas.Broker?
	return env.envCfgUnlocked.Name()
}

// DeleteService deletes the specified service with all related resources.
func (env *environ) DeleteService(appName string) (err error) {
	// TODO(ecs)
	return nil
}

// Destroy is part of the Broker interface.
func (env *environ) Destroy(callbacks envcontext.ProviderCallContext) (err error) {
	// TODO(ecs)
	return nil
}

// DestroyController implements the Environ interface.
func (env *environ) DestroyController(ctx envcontext.ProviderCallContext, controllerUUID string) error {
	// TODO(ecs)
	return nil
}

// EnsureService creates or updates a service for pods with the given params.
func (env *environ) EnsureService(
	appName string,
	statusCallback caas.StatusCallbackFunc,
	params *caas.ServiceParams,
	numUnits int,
	config application.ConfigAttributes,
) (err error) {
	// TODO(ecs): remove from caas.Broker?
	return nil
}

// ExposeService sets up external access to the specified application.
func (env *environ) ExposeService(appName string, resourceTags map[string]string, config application.ConfigAttributes) error {
	// TODO(ecs): remove from caas.Broker?
	return nil
}

// GetAnnotations returns current namespace's annotations.
func (env *environ) GetAnnotations() annotations.Annotation {
	// TODO(ecs): remove from caas.Broker?
	return nil
}

// GetService returns the service for the specified application.
func (env *environ) GetService(appName string, mode caas.DeploymentMode, includeClusterIP bool) (*caas.Service, error) {
	// TODO(ecs): remove from caas.Broker?
	return nil, nil
}

// UnexposeService removes external access to the specified service.
func (env *environ) UnexposeService(appName string) error {
	// TODO(ecs): remove from caas.Broker?
	return nil
}

// Units returns all units and any associated filesystems of the specified application.
// Filesystems are mounted via volumes bound to the unit.
func (env *environ) Units(appName string, mode caas.DeploymentMode) ([]caas.Unit, error) {
	// TODO(ecs): remove from caas.Broker?
	return nil, nil
}

// WatchContainerStart returns a watcher which is notified when a container matching containerName regexp
// is starting/restarting. Each string represents the provider id for the unit the container belongs to.
// If containerName regexp matches empty string, then the first workload container
// is used.
func (env *environ) WatchContainerStart(appName string, containerName string) (watcher.StringsWatcher, error) {
	// TODO(ecs): remove from caas.Broker?
	return nil, nil
}

// WatchService returns a watcher which notifies when there
// are changes to the deployment of the specified application.
func (env *environ) WatchService(appName string, mode caas.DeploymentMode) (watcher.NotifyWatcher, error) {
	// TODO(ecs): remove from caas.Broker?
	return nil, nil
}

// WatchUnits returns a watcher which notifies when there
// are changes to units of the specified application.
func (env *environ) WatchUnits(appName string, mode caas.DeploymentMode) (watcher.NotifyWatcher, error) {
	// TODO(ecs): remove from caas.Broker?
	return nil, nil
}

// AdoptResources is called when the model is moved from one
// controller to another using model migration.
func (env *environ) AdoptResources(ctx envcontext.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	// TODO(ecs): remove from caas.Broker?
	return nil
}

// Application returns an Application interface.
func (env *environ) Application(name string, deploymentType caas.DeploymentType) caas.Application {
	return newApplication(
		name, env.clusterName, env.controllerUUID, env.modelUUID, env.CurrentModel(), deploymentType, env.client(), env.clock,
	)
}

// DeleteOperator deletes the specified operator.
func (env *environ) DeleteOperator(appName string) (err error) {
	// TODO(ecs): remove from caas.Broker?
	return nil
}

// EnsureOperator creates or updates an operator pod with the given application
// name, agent path, and operator config.
func (*environ) EnsureOperator(appName, agentPath string, config *caas.OperatorConfig) (err error) {
	// TODO(ecs): remove from caas.Broker?
	return nil
}

// Operator returns an Operator with current status and life details.
func (*environ) Operator(appName string) (*caas.Operator, error) {
	// TODO(ecs): remove from caas.Broker?
	return nil, nil
}

// OperatorExists indicates if the operator for the specified
// application exists, and whether the operator is terminating.
func (*environ) OperatorExists(appName string) (caas.DeploymentState, error) {
	// TODO(ecs): remove from caas.Broker?
	return caas.DeploymentState{}, nil
}

// WatchOperator returns a watcher which notifies when there
// are changes to the operator of the specified application.
func (*environ) WatchOperator(appName string) (watcher.NotifyWatcher, error) {
	// TODO(ecs): remove from caas.Broker?
	return nil, nil
}

// EnsureModelOperator implements caas broker's interface. Function ensures that
// a model operator for this broker's namespace exists within Kubernetes.
func (env *environ) EnsureModelOperator(
	modelUUID, agentPath string, config *caas.ModelOperatorConfig,
) error {
	// TODO(ecs): remove from caas.Broker?
	return nil
}

// ModelOperator return the model operator config used to create the current
// model operator for this broker
func (*environ) ModelOperator() (*caas.ModelOperatorConfig, error) {
	// TODO(ecs): remove from caas.Broker?
	return nil, nil
}

// ModelOperatorExists indicates if the model operator for the given broker
// exists
func (*environ) ModelOperatorExists() (bool, error) {
	// TODO(ecs): remove from caas.Broker?
	return false, nil
}

// PrecheckInstance performs a preflight check on the specified
// series and constraints, ensuring that they are possibly valid for
// creating an instance in this model.
//
// PrecheckInstance is best effort, and not guaranteed to eliminate
// all invalid parameters. If PrecheckInstance returns nil, it is not
// guaranteed that the constraints are valid; if a non-nil error is
// returned, then the constraints are definitely invalid.
func (*environ) PrecheckInstance(ctx envcontext.ProviderCallContext, params environs.PrecheckInstanceParams) error {
	// TODO(ecs): remove from caas.Broker?
	return nil
}

func (*environ) Upgrade(agentTag string, vers version.Number) error {
	// TODO(ecs): remove from caas.Broker?
	return nil
}
