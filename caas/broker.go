// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version"
	core "k8s.io/api/core/v1"

	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/storage"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/broker_mock.go github.com/juju/juju/caas Broker

// ContainerEnvironProvider represents a computing and storage provider
// for a container runtime.
type ContainerEnvironProvider interface {
	environs.EnvironProvider

	// Open opens the broker and returns it. The configuration must
	// have passed through PrepareConfig at some point in its lifecycle.
	//
	// Open should not perform any expensive operations, such as querying
	// the cloud API, as it will be called frequently.
	Open(args environs.OpenParams) (Broker, error)
}

// RegisterContainerProvider is used for providers that we want to use for managing 'instances',
// but are not possible sources for 'juju bootstrap'.
func RegisterContainerProvider(name string, p ContainerEnvironProvider, alias ...string) (unregister func()) {
	if err := environs.GlobalProviderRegistry().RegisterProvider(p, name, alias...); err != nil {
		panic(fmt.Errorf("juju: %v", err))
	}
	return func() {
		environs.GlobalProviderRegistry().UnregisterProvider(name)
	}
}

// New returns a new broker based on the provided configuration.
func New(args environs.OpenParams) (Broker, error) {
	p, err := environs.Provider(args.Cloud.Type)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return Open(p, args)
}

// Open creates a Broker instance and errors if the provider is not for
// a container substrate.
func Open(p environs.EnvironProvider, args environs.OpenParams) (Broker, error) {
	if envProvider, ok := p.(ContainerEnvironProvider); !ok {
		return nil, errors.NotValidf("container environ provider %T", p)
	} else {
		return envProvider.Open(args)
	}
}

// NewContainerBrokerFunc returns a Container Broker.
type NewContainerBrokerFunc func(args environs.OpenParams) (Broker, error)

// StatusCallbackFunc represents a function that can be called to report a status.
type StatusCallbackFunc func(appName string, settableStatus status.Status, info string, data map[string]interface{}) error

// DeploymentType defines a deployment type.
type DeploymentType string

// Validate validates if this deployment type is supported.
func (dt DeploymentType) Validate() error {
	if dt == "" {
		return nil
	}
	if dt == DeploymentStateless ||
		dt == DeploymentStateful ||
		dt == DeploymentDaemon {
		return nil
	}
	return errors.NotSupportedf("deployment type %q", dt)
}

const (
	DeploymentStateless DeploymentType = "stateless"
	DeploymentStateful  DeploymentType = "stateful"
	DeploymentDaemon    DeploymentType = "daemon"
)

// DeploymentMode defines a deployment mode.
type DeploymentMode string

const (
	ModeOperator DeploymentMode = "operator"
	ModeWorkload DeploymentMode = "workload"
	ModeEmbedded DeploymentMode = "embedded"
)

// ServiceType defines a service type.
type ServiceType string

// IsOmit indicates if a service is required.
func (st ServiceType) IsOmit() bool {
	return st == ServiceOmit
}

const (
	ServiceCluster      ServiceType = "cluster"
	ServiceLoadBalancer ServiceType = "loadbalancer"
	ServiceExternal     ServiceType = "external"
	ServiceOmit         ServiceType = "omit"
)

// DeploymentParams defines parameters for specifying how a service is deployed.
type DeploymentParams struct {
	DeploymentType DeploymentType
	ServiceType    ServiceType
}

// ServiceParams defines parameters used to create a service.
type ServiceParams struct {
	// Deployment defines how a service is deployed.
	Deployment DeploymentParams

	// PodSpec is the spec used to configure a pod.
	PodSpec *specs.PodSpec

	// RawK8sSpec is the raw spec used to to apply to the cluster.
	RawK8sSpec string

	// ResourceTags is a set of tags to set on the created service.
	ResourceTags map[string]string

	// Constraints is a set of constraints on
	// the pod to create.
	Constraints constraints.Value

	// Filesystems is a set of parameters for filesystems that should be created.
	Filesystems []storage.KubernetesFilesystemParams

	// Devices is a set of parameters for Devices that is required.
	Devices []devices.KubernetesDeviceParams

	// OperatorImagePath is the path to the OCI image shared by the operator and pod init.
	OperatorImagePath string

	// CharmModifiedVersion increases when the charm changes in some way.
	CharmModifiedVersion int
}

// DeploymentState is returned by the OperatorExists call.
type DeploymentState struct {
	// Exists is true if the operator/application exists in the cluster.
	Exists bool

	// Terminating is true if the operator/application is in Terminating state.
	Terminating bool
}

// Broker instances interact with the CAAS substrate.
type Broker interface {
	// Provider returns the ContainerEnvironProvider that created this Broker.
	Provider() ContainerEnvironProvider

	// APIVersion returns the master kubelet API version.
	APIVersion() (string, error)

	// EnsureModelOperator creates or updates a model operator pod for running
	// model operations in a CAAS namespace/model
	EnsureModelOperator(modelUUID, agentPath string, config *ModelOperatorConfig) error

	// ModelOperator return the model operator config used to create the current
	// model operator for this broker
	ModelOperator() (*ModelOperatorConfig, error)

	// ModelOperatorExists indicates if the model operator for the given broker
	// exists
	ModelOperatorExists() (bool, error)

	// EnsureOperator creates or updates an operator pod for running
	// a charm for the specified application.
	EnsureOperator(appName, agentPath string, config *OperatorConfig) error

	// OperatorExists indicates if the operator for the specified
	// application exists, and whether the operator is terminating.
	OperatorExists(appName string) (DeploymentState, error)

	// DeleteOperator deletes the specified operator.
	DeleteOperator(appName string) error

	// WatchUnits returns a watcher which notifies when there
	// are changes to units of the specified application.
	WatchUnits(appName string, mode DeploymentMode) (watcher.NotifyWatcher, error)

	// Units returns all units and any associated filesystems
	// of the specified application. Filesystems are mounted
	// via volumes bound to the unit.
	Units(appName string, mode DeploymentMode) ([]Unit, error)

	// AnnotateUnit annotates the specified pod (name or uid) with a unit tag.
	AnnotateUnit(appName string, mode DeploymentMode, podName string, unit names.UnitTag) error

	// WatchContainerStart returns a watcher which is notified when the specified container
	// for each unit in the application is starting/restarting. Each string represents
	// the provider id for the unit. If containerName is empty, then the first workload container
	// is used.
	WatchContainerStart(appName string, containerName string) (watcher.StringsWatcher, error)

	// WatchOperator returns a watcher which notifies when there
	// are changes to the operator of the specified application.
	WatchOperator(string) (watcher.NotifyWatcher, error)

	// WatchService returns a watcher which notifies when there
	// are changes to the deployment of the specified application.
	WatchService(appName string, mode DeploymentMode) (watcher.NotifyWatcher, error)

	// Operator returns an Operator with current status and life details.
	Operator(string) (*Operator, error)

	// Application returns the broker interface for an Application
	Application(string, DeploymentType) Application

	// ClusterMetadataChecker provides an API to query cluster metadata.
	ClusterMetadataChecker

	// NamespaceWatcher provides the API to watch caas namespace.
	NamespaceWatcher

	// InstancePrechecker provides a means of "prechecking" placement
	// arguments before recording them in state.
	environs.InstancePrechecker

	// BootstrapEnviron defines methods for bootstrapping a controller.
	environs.BootstrapEnviron

	// ResourceAdopter defines methods for adopting resources.
	environs.ResourceAdopter

	// NamespaceGetterSetter provides the API to get/set namespace.
	NamespaceGetterSetter

	// StorageValidator provides methods to validate storage.
	StorageValidator

	// ServiceGetterSetter provides the API to get/set service.
	ServiceGetterSetter

	// Upgrader provides the API to perform upgrades.
	Upgrader

	// ClusterVersionGetter provides methods to get cluster version information.
	ClusterVersionGetter
}

// Upgrader provides the API to perform upgrades.
type Upgrader interface {
	// Upgrade sets the OCI image for the app to the specified version.
	Upgrade(appName string, vers version.Number) error
}

// StorageValidator provides methods to validate storage.
type StorageValidator interface {
	// ValidateStorageClass returns an error if the storage config is not valid.
	ValidateStorageClass(config map[string]interface{}) error
}

// ClusterVersionGetter provides methods to get cluster version information.
type ClusterVersionGetter interface {
	// Version returns cluster version information.
	Version() (*version.Number, error)
}

// ServiceGetterSetter provides the API to get/set service.
type ServiceGetterSetter interface {
	// EnsureService creates or updates a service for pods with the given params.
	EnsureService(appName string, statusCallback StatusCallbackFunc, params *ServiceParams, numUnits int, config application.ConfigAttributes) error

	// DeleteService deletes the specified service with all related resources.
	DeleteService(appName string) error

	// ExposeService sets up external access to the specified service.
	ExposeService(appName string, resourceTags map[string]string, config application.ConfigAttributes) error

	// UnexposeService removes external access to the specified service.
	UnexposeService(appName string) error

	// GetService returns the service for the specified application.
	GetService(appName string, mode DeploymentMode, includeClusterIP bool) (*Service, error)
}

// NamespaceGetterSetter provides the API to get/set namespace.
type NamespaceGetterSetter interface {
	// Namespaces returns name names of the namespaces on the cluster.
	Namespaces() ([]string, error)

	// GetNamespace returns the namespace for the specified name or current namespace.
	GetNamespace(name string) (*core.Namespace, error)

	// GetCurrentNamespace returns current namespace name.
	GetCurrentNamespace() string
}

// ClusterMetadataChecker provides an API to query cluster metadata.
type ClusterMetadataChecker interface {
	// GetClusterMetadata returns metadata about host cloud and storage for the cluster.
	GetClusterMetadata(storageClass string) (result *ClusterMetadata, err error)

	// CheckDefaultWorkloadStorage returns an error if the opinionated storage defined for
	// the cluster does not match the specified storage.
	CheckDefaultWorkloadStorage(cluster string, storageProvisioner *StorageProvisioner) error

	// EnsureStorageProvisioner creates a storage provisioner with the specified config, or returns an existing one.
	EnsureStorageProvisioner(cfg StorageProvisioner) (*StorageProvisioner, bool, error)
}

// NamespaceWatcher provides the API to watch caas namespace.
type NamespaceWatcher interface {
	// WatchNamespace returns a watcher which notifies when there
	// are changes to current namespace.
	WatchNamespace() (watcher.NotifyWatcher, error)
}

// Service represents information about the status of a caas service entity.
type Service struct {
	Id         string
	Addresses  network.ProviderAddresses
	Scale      *int
	Generation *int64
	Status     status.StatusInfo
}

// FilesystemInfo represents information about a filesystem
// mounted by a unit.
type FilesystemInfo struct {
	StorageName  string
	FilesystemId string
	Size         uint64
	MountPoint   string
	ReadOnly     bool
	Status       status.StatusInfo
	Volume       VolumeInfo
}

// VolumeInfo represents information about a volume
// mounted by a unit.
type VolumeInfo struct {
	VolumeId   string
	Size       uint64
	Persistent bool
	Status     status.StatusInfo
}

// Unit represents information about the status of a "pod".
type Unit struct {
	Id             string
	Address        string
	Ports          []string
	Dying          bool
	Stateful       bool
	Status         status.StatusInfo
	FilesystemInfo []FilesystemInfo
}

// Operator represents information about the status of an "operator pod".
type Operator struct {
	Id     string
	Dying  bool
	Status status.StatusInfo
	Config *OperatorConfig
}

// CharmStorageParams defines parameters used to create storage
// for operators to use for charm state.
type CharmStorageParams struct {
	// Size is the minimum size of the filesystem in MiB.
	Size uint64

	// The provider type for this filesystem.
	Provider storage.ProviderType

	// Attributes is a set of provider-specific options for storage creation,
	// as defined in a storage pool.
	Attributes map[string]interface{}

	// ResourceTags is a set of tags to set on the created filesystem, if the
	// storage provider supports tags.
	ResourceTags map[string]string
}

// ModelOperatorConfig is the config to when creating a model operator
type ModelOperatorConfig struct {
	// AgentConf is the contents of the agent.conf file.
	AgentConf []byte

	// OperatorImagePath is the docker registry URL for the image.
	OperatorImagePath string

	// Port is the socket port that the operator model will be listening on
	Port int32
}

// OperatorConfig is the config to use when creating an operator.
type OperatorConfig struct {
	// OperatorImagePath is the docker registry URL for the image.
	OperatorImagePath string

	// Version is the Juju version of the operator image.
	Version version.Number

	// CharmStorage defines parameters used to optionally
	// create storage for operators to use for charm state.
	CharmStorage *CharmStorageParams

	// AgentConf is the contents of the agent.conf file.
	AgentConf []byte

	// OperatorInfo is the contents of the operator.yaml file.
	OperatorInfo []byte

	// ResourceTags is a set of tags to set on the operator pod.
	ResourceTags map[string]string

	// ConfigMapGeneration is set when updating the operator config
	// map for consistency in Read after Write and Write after Write.
	// A value of 0 is ignored.
	ConfigMapGeneration int64
}
