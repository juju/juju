// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	"github.com/prometheus/client_golang/prometheus"
	gossh "golang.org/x/crypto/ssh"

	corecontroller "github.com/juju/juju/core/controller"
	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/virtualhostname"
	controllersshservice "github.com/juju/juju/domain/ssh/service/controller"
	modelsshservice "github.com/juju/juju/domain/ssh/service/model"
	"github.com/juju/juju/internal/jwtparser"
	k8sexec "github.com/juju/juju/internal/provider/kubernetes/exec"
	"github.com/juju/juju/internal/services"
	internalTunneler "github.com/juju/juju/internal/sshtunneler"
	"github.com/juju/juju/internal/worker/common"
	workerTunneler "github.com/juju/juju/internal/worker/sshtunneler"
)

// machineConnectionTimeout is the maximum time to wait for a machine
// to establish a reverse tunnel connection back to the controller.
// This maye be useful to make configurable in the future.
const machineConnectionTimeout = 60 * time.Second

// GetControllerConfigServiceFunc is a helper function that gets
// a controller config service from the manifold.
type GetControllerConfigServiceFunc = func(getter dependency.Getter, name string) (ControllerConfigService, error)

// GetDomainServicesGetterFunc is a helper function that gets the model domain
// services getter from the manifold.
type GetDomainServicesGetterFunc = func(getter dependency.Getter, name string) (services.DomainServicesGetter, error)

// GetSSHServiceFunc is a helper function that gets the model SSH service from
// the manifold.
type GetSSHServiceFunc = func(context.Context, services.DomainServicesGetter, model.UUID) (*modelsshservice.WatchableService, error)

// GetControllerConfigService is a helper function that gets a service from the
// manifold.
func GetControllerConfigService(getter dependency.Getter, name string) (ControllerConfigService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.ControllerDomainServices) ControllerConfigService {
		return factory.ControllerConfig()
	})
}

// GetControllerSSHHostKeyService gets the controller SSH host key service from
// the controller domain services dependency.
func GetControllerSSHService(getter dependency.Getter, name string) (*controllersshservice.Service, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.ControllerDomainServices) *controllersshservice.Service {
		return factory.SSHServerHostKey()
	})
}

// GetDomainServicesGetter gets the model domain services getter from the
// domain services worker dependency.
func GetDomainServicesGetter(getter dependency.Getter, name string) (services.DomainServicesGetter, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.DomainServicesGetter) services.DomainServicesGetter {
		return factory
	})

}

// GetSSHService gets the model SSH service from the current model domain
// services dependency.
func GetSSHService(ctx context.Context, domainServicesGetter services.DomainServicesGetter, modelUUID model.UUID) (*modelsshservice.WatchableService, error) {
	domainServices, err := domainServicesGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return domainServices.SSH(), nil
}

// ManifoldConfig holds the information necessary to run an embedded SSH server
// worker in a dependency.Engine.
type ManifoldConfig struct {
	// DomainServicesName is the name of the domain services worker.
	DomainServicesName string
	// SSHTunnelerName is the name of the SSH tunneler worker.
	SSHTunnelerName string
	// JWTParserName is the name of the JWT parser worker.
	JWTParserName string
	// ControllerID is the ID of the controller node.
	ControllerID string
	// ControllerUUID is the UUID of the controller entity.
	ControllerUUID string
	// NewServerWrapperWorker is the function that creates the embedded SSH server worker.
	NewServerWrapperWorker func(ServerWrapperWorkerConfig) (worker.Worker, error)
	// NewServerWorker is the function that creates a worker that has a catacomb
	// to run the server and other worker dependencies.
	NewServerWorker func(ServerWorkerConfig) (worker.Worker, error)
	// GetControllerConfigService is used to get a service from the manifold.
	GetControllerConfigService GetControllerConfigServiceFunc
	// GetControllerSSHService is used to get the concrete controller SSH service
	// from the manifold.
	GetControllerSSHService func(getter dependency.Getter, name string) (*controllersshservice.Service, error)
	// GetDomainServicesGetter is used to get the model domain services getter
	// from the manifold.
	GetDomainServicesGetter GetDomainServicesGetterFunc
	// GetSSHService is used to get the SSH service from the manifold.
	GetSSHService GetSSHServiceFunc
	// Logger is the logger to use for the worker.
	Logger logger.Logger
	// PrometheusRegisterer registers SSH server metrics.
	PrometheusRegisterer prometheus.Registerer
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.SSHTunnelerName == "" {
		return errors.NotValidf("empty SSHTunnelerName")
	}
	if config.JWTParserName == "" {
		return errors.NotValidf("empty JWTParserName")
	}
	if config.ControllerID == "" {
		return errors.NotValidf("empty ControllerID")
	}
	if config.ControllerUUID == "" {
		return errors.NotValidf("empty ControllerUUID")
	}
	if config.NewServerWrapperWorker == nil {
		return errors.NotValidf("nil NewServerWrapperWorker")
	}
	if config.NewServerWorker == nil {
		return errors.NotValidf("nil NewServerWorker")
	}
	if config.GetControllerConfigService == nil {
		return errors.NotValidf("nil GetControllerConfigService")
	}
	if config.GetControllerSSHService == nil {
		return errors.NotValidf("nil GetControllerSSHService")
	}
	if config.GetDomainServicesGetter == nil {
		return errors.NotValidf("nil GetDomainServicesGetter")
	}
	if config.GetSSHService == nil {
		return errors.NotValidf("nil GetSSHService")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an embedded SSH server
// worker. The manifold has no outputs.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.DomainServicesName, config.SSHTunnelerName, config.JWTParserName},
		Start:  config.startWrapperWorker,
	}
}

// startWrapperWorker starts the SSH server worker wrapper passing the necessary dependencies.
func (config ManifoldConfig) startWrapperWorker(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	controllerUUID, err := corecontroller.ParseUUID(config.ControllerUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerConfigService, err := config.GetControllerConfigService(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerSSHService, err := config.GetControllerSSHService(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	domainServicesGetter, err := config.GetDomainServicesGetter(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var tunnelTracker workerTunneler.TunnelTracker
	if err := getter.Get(config.SSHTunnelerName, &tunnelTracker); err != nil {
		return nil, errors.Trace(err)
	}
	var jwtParser *jwtparser.Parser
	if err := getter.Get(config.JWTParserName, &jwtParser); err != nil {
		return nil, errors.Trace(err)
	}

	metricsCollector := NewMetricsCollector()
	if err := config.PrometheusRegisterer.Register(metricsCollector); err != nil {
		return nil, errors.Trace(err)
	}

	sshService := sshService{
		controllerSSHService: controllerSSHService,
		domainServicesGetter: domainServicesGetter,
		getSSHService:        config.GetSSHService,
		controllerUUID:       controllerUUID,
	}
	proxyFactory := proxyFactory{
		k8sResolver: sshService,
		logger:      config.Logger,
		connector: tunnelConnector{
			tunnelTracker: tunnelTracker,
			controllerID:  config.ControllerID,
			resolver:      sshService,
		},
		getExecutor: k8sexec.NewInCluster,
		metrics:     metricsCollector,
	}

	w, err := config.NewServerWrapperWorker(ServerWrapperWorkerConfig{
		ControllerConfigService: controllerConfigService,
		SSHService:              sshService,
		NewServerWorker:         config.NewServerWorker,
		Logger:                  config.Logger,
		Authenticator: authenticator{
			logger:        config.Logger,
			jwtParser:     jwtParser,
			tunnelTracker: tunnelTracker,
			publicKeys:    sshService,
		},
		Authorizer: authorizer{
			access: sshService,
			logger: config.Logger,
		},
		ProxyFactory:  proxyFactory,
		TunnelTracker: tunnelTracker,
		Metrics:       metricsCollector,
	})
	if err != nil {
		_ = config.PrometheusRegisterer.Unregister(metricsCollector)
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() {
		_ = config.PrometheusRegisterer.Unregister(metricsCollector)
	}), nil
}

// sshService wraps our ssh domain services to enable two things:
//  1. Direct controller model access via the ControllerSSHHostKeyService interface.
//  2. Model-scoped access to the SSHService interface with underlying calls to
//     "ServicesForModel".
//     The SSH server doesn't take the apiserver approach where the model uuid is populated
//     by the time we reach the service, and instead, we must call the methods WITH the UUID received
//     from the virtual host name.
type sshService struct {
	controllerSSHService *controllersshservice.Service
	domainServicesGetter services.DomainServicesGetter
	getSSHService        GetSSHServiceFunc
	controllerUUID       corecontroller.UUID
}

// PublicKeys returns all public SSH keys registered for a user.
// It calls the domain service to get the user's keys and converts
// them to the gossh.PublicKey type.
func (s sshService) PublicKeys(ctx context.Context, username string) ([]gossh.PublicKey, error) {
	name, err := user.NewName(username)
	if err != nil {
		return nil, errors.Trace(err)
	}
	keys, err := s.controllerSSHService.GetPublicKeysForUser(ctx, name)
	if err != nil {
		return nil, errors.Trace(err)
	}

	publicKeys := make([]gossh.PublicKey, 0, len(keys))
	for _, key := range keys {
		publicKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(key.Key))
		if err != nil {
			return nil, errors.Annotatef(err, "parsing public key for user %q", username)
		}
		publicKeys = append(publicKeys, publicKey)
	}
	return publicKeys, nil
}

// SSHServerHostKey returns the controller SSH server host key.
func (s sshService) SSHServerHostKey(ctx context.Context) (string, error) {
	return s.controllerSSHService.SSHServerHostKey(ctx)
}

// VirtualHostKey returns the terminating SSH host key for a virtual hostname.
// The virtual hostname contains the model UUID for the destination model database.
func (s sshService) VirtualHostKey(ctx context.Context, info virtualhostname.Info) (string, error) {
	sshService, err := s.getSSHService(ctx, s.domainServicesGetter, info.ModelUUID())
	if err != nil {
		return "", errors.Trace(err)
	}
	return sshService.VirtualHostKey(ctx, info)
}

// HasSSHAccessToModel checks whether a user has SSH access to a model.
// It resolves the model's domain services and checks access.
func (s sshService) HasSSHAccessToModel(ctx context.Context, username string, destination virtualhostname.Info) (bool, error) {
	name, err := user.NewName(username)
	if err != nil {
		return false, errors.Trace(err)
	}
	domainServices, err := s.domainServicesGetter.ServicesForModel(ctx, destination.ModelUUID())
	if err != nil {
		return false, errors.Trace(err)
	}
	return domainServices.Access().HasSSHAccessToModel(ctx, name, destination.ModelUUID(), s.controllerUUID)
}

// ResolveK8sExecInfo resolves the Kubernetes namespace and pod name for a destination.
func (s sshService) ResolveK8sExecInfo(ctx context.Context, destination virtualhostname.Info) (string, string, error) {
	sshService, err := s.getSSHService(ctx, s.domainServicesGetter, destination.ModelUUID())
	if err != nil {
		return "", "", err
	}
	return sshService.ResolveK8sExecInfo(ctx, destination)
}

// MachineForDestination resolves an IAAS machine or machine-backed unit to the
// machine name expected by the reverse tunnel tracker.
func (s sshService) MachineForDestination(ctx context.Context, destination virtualhostname.Info) (coremachine.Name, error) {
	sshService, err := s.getSSHService(ctx, s.domainServicesGetter, destination.ModelUUID())
	if err != nil {
		return "", err
	}
	return sshService.MachineForDestination(ctx, destination)
}

type tunnelConnector struct {
	tunnelTracker workerTunneler.TunnelTracker
	controllerID  string
	resolver      sshService
}

// Connect requests a one-shot reverse tunnel to the machine resolved from a
// routed SSH destination. The local controller node ID preserves HA affinity:
// the machine connects back to the controller handling the client session.
func (c tunnelConnector) Connect(ctx context.Context, destination virtualhostname.Info) (*gossh.Client, error) {
	machineName, err := c.resolver.MachineForDestination(ctx, destination)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctx, cancel := context.WithTimeout(ctx, machineConnectionTimeout)
	defer cancel()
	return c.tunnelTracker.RequestTunnel(ctx, internalTunneler.RequestArgs{
		MachineID:        machineName.String(),
		ModelUUID:        destination.ModelUUID().String(),
		ControllerNodeID: c.controllerID,
	})
}
