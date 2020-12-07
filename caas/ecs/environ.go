// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs

import (
	"sync"

	// "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/ecs/ecsiface"
	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version"
	core "k8s.io/api/core/v1"    // REMOVE !!!!!
	"k8s.io/client-go/informers" // REMOVE !!!!!

	// "github.com/juju/juju/environs"
	// "github.com/juju/juju/cloud"
	"github.com/juju/juju/caas"
	// "github.com/juju/juju/caas/ecs/constants"
	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	cloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	envcontext "github.com/juju/juju/environs/context"
	// jujustorage "github.com/juju/juju/storage"
)

type environ struct {
	name        string
	clusterName string

	clock jujuclock.Clock
	cloud cloudspec.CloudSpec

	// modelUUID is the UUID of the model this client acts on.
	modelUUID string

	lock           sync.Mutex
	envCfgUnlocked *config.Config

	clientUnlocked ecsiface.ECSAPI
}

// var _ environs.Environ = (*environ)(nil)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/ecs_mock.go github.com/aws/aws-sdk-go/service/ecs/ecsiface ECSAPI
func newEnviron(
	clusterName string,
	clock jujuclock.Clock,
	cloud cloudspec.CloudSpec, envCfg *config.Config,
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
		cloud:          cloud,
		modelUUID:      modelUUID,
		envCfgUnlocked: envCfg,
	}
	if env.clientUnlocked, err = newECSClient(cloud); err != nil {
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
	return "", nil
}

// Version returns cluster version information.
func (env *environ) Version() (ver *version.Number, err error) {
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
	env.cloud = spec
	if env.clientUnlocked, err = newECSClient(spec); err != nil {
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
	// TODO
	return nil
}

// AnnotateUnit annotates the specified pod (name or uid) with a unit tag.
func (env *environ) AnnotateUnit(appName string, mode caas.DeploymentMode, podName string, unit names.UnitTag) error {
	return nil
}

// PrepareForBootstrap prepares for bootstraping a controller.
func (env *environ) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	return nil
}

// Bootstrap deploys controller with mongoDB together into ecs cluster.
func (env *environ) Bootstrap(
	ctx environs.BootstrapContext, callCtx envcontext.ProviderCallContext, args environs.BootstrapParams,
) (*environs.BootstrapResult, error) {
	return nil, nil
}

// Create implements environs.BootstrapEnviron.
func (env *environ) Create(envcontext.ProviderCallContext, environs.CreateParams) error {
	return nil
}

func (env *environ) CurrentModel() string {
	env.lock.Lock()
	defer env.lock.Unlock()
	return env.envCfgUnlocked.Name()
}

// DeleteService deletes the specified service with all related resources.
func (env *environ) DeleteService(appName string) (err error) {
	return nil
}

// Destroy is part of the Broker interface.
func (env *environ) Destroy(callbacks envcontext.ProviderCallContext) (err error) {
	return nil
}

// DestroyController implements the Environ interface.
func (env *environ) DestroyController(ctx envcontext.ProviderCallContext, controllerUUID string) error {
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
	return nil
}

// ExposeService sets up external access to the specified application.
func (env *environ) ExposeService(appName string, resourceTags map[string]string, config application.ConfigAttributes) error {
	return nil
}

// GetAnnotations returns current namespace's annotations.
func (env *environ) GetAnnotations() annotations.Annotation {
	return nil
}

// GetService returns the service for the specified application.
func (env *environ) GetService(appName string, mode caas.DeploymentMode, includeClusterIP bool) (*caas.Service, error) {
	return nil, nil
}

// SharedInformerFactory returns the default k8s SharedInformerFactory used by
// this broker.
func (env *environ) SharedInformerFactory() informers.SharedInformerFactory {
	// REMOVE !!!!!
	return nil
}

// UnexposeService removes external access to the specified service.
func (env *environ) UnexposeService(appName string) error {
	return nil
}

// Units returns all units and any associated filesystems of the specified application.
// Filesystems are mounted via volumes bound to the unit.
func (env *environ) Units(appName string, mode caas.DeploymentMode) ([]caas.Unit, error) {
	return nil, nil
}

// WatchContainerStart returns a watcher which is notified when a container matching containerName regexp
// is starting/restarting. Each string represents the provider id for the unit the container belongs to.
// If containerName regexp matches empty string, then the first workload container
// is used.
func (env *environ) WatchContainerStart(appName string, containerName string) (watcher.StringsWatcher, error) {
	return nil, nil
}

// WatchService returns a watcher which notifies when there
// are changes to the deployment of the specified application.
func (env *environ) WatchService(appName string, mode caas.DeploymentMode) (watcher.NotifyWatcher, error) {
	return nil, nil
}

// WatchUnits returns a watcher which notifies when there
// are changes to units of the specified application.
func (env *environ) WatchUnits(appName string, mode caas.DeploymentMode) (watcher.NotifyWatcher, error) {
	return nil, nil
}

// AdoptResources is called when the model is moved from one
// controller to another using model migration.
func (env *environ) AdoptResources(ctx envcontext.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	return nil
}

// Application returns an Application interface.
func (env *environ) Application(name string, deploymentType caas.DeploymentType) caas.Application {
	return newApplication(
		name, env.clusterName, env.modelUUID, env.CurrentModel(), deploymentType, env.client(), env.clock,
	)
}

// DeleteOperator deletes the specified operator.
func (env *environ) DeleteOperator(appName string) (err error) {
	// REMOVE!!!!!!!
	return nil
}

// EnsureOperator creates or updates an operator pod with the given application
// name, agent path, and operator config.
func (*environ) EnsureOperator(appName, agentPath string, config *caas.OperatorConfig) (err error) {
	// REMOVE!!!
	return nil
}

// Operator returns an Operator with current status and life details.
func (*environ) Operator(appName string) (*caas.Operator, error) {
	// REMOVE!!!
	return nil, nil
}

// OperatorExists indicates if the operator for the specified
// application exists, and whether the operator is terminating.
func (*environ) OperatorExists(appName string) (caas.DeploymentState, error) {
	// REMOVE!!!
	return caas.DeploymentState{}, nil
}

// WatchOperator returns a watcher which notifies when there
// are changes to the operator of the specified application.
func (*environ) WatchOperator(appName string) (watcher.NotifyWatcher, error) {
	// REMOVE!!!
	return nil, nil
}

// EnsureModelOperator implements caas broker's interface. Function ensures that
// a model operator for this broker's namespace exists within Kubernetes.
func (env *environ) EnsureModelOperator(
	modelUUID, agentPath string, config *caas.ModelOperatorConfig,
) error {
	// REMOVE!!!
	return nil
}

// ModelOperator return the model operator config used to create the current
// model operator for this broker
func (*environ) ModelOperator() (*caas.ModelOperatorConfig, error) {
	// REMOVE!!!
	return nil, nil
}

// ModelOperatorExists indicates if the model operator for the given broker
// exists
func (*environ) ModelOperatorExists() (bool, error) {
	// REMOVE!!!
	return false, nil
}

// GetCurrentNamespace returns current namespace name.
func (*environ) GetCurrentNamespace() string {
	// REMOVE!!!
	return ""
}

// GetNamespace returns the namespace for the specified name.
func (*environ) GetNamespace(name string) (*core.Namespace, error) {
	// REMOVE!!!
	return nil, nil
}

// Namespaces returns names of the namespaces on the cluster.
func (*environ) Namespaces() ([]string, error) {
	// REMOVE!!!
	return nil, nil
}

// WatchNamespace returns a watcher which notifies when there
// are changes to current namespace.
func (*environ) WatchNamespace() (watcher.NotifyWatcher, error) {
	// REMOVE!!!
	return nil, nil
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
	return nil
}

func (*environ) Upgrade(agentTag string, vers version.Number) error {
	return nil
}
