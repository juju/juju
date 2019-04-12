// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/version"
	core "k8s.io/api/core/v1"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/storage"
)

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

	// ParsePodSpec unmarshalls the given YAML pod spec.
	ParsePodSpec(in string) (*PodSpec, error)
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

// ServiceParams defines parameters used to create a service.
type ServiceParams struct {
	// PodSpec is the spec used to configure a pod.
	PodSpec *PodSpec

	// ResourceTags is a set of tags to set on the created service.
	ResourceTags map[string]string

	// Constraints is a set of constraints on
	// the pod to create.
	Constraints constraints.Value

	// Filesystems is a set of parameters for filesystems that should be created.
	Filesystems []storage.KubernetesFilesystemParams

	// Devices is a set of parameters for Devices that is required.
	Devices []devices.KubernetesDeviceParams
}

// Broker instances interact with the CAAS substrate.
type Broker interface {
	// Provider returns the ContainerEnvironProvider that created this Broker.
	Provider() ContainerEnvironProvider

	// APIVersion returns the master kubelet API version.
	APIVersion() (string, error)

	// EnsureOperator creates or updates an operator pod for running
	// a charm for the specified application.
	EnsureOperator(appName, agentPath string, config *OperatorConfig) error

	// OperatorExists returns true if the operator for the specified
	// application exists.
	OperatorExists(appName string) (bool, error)

	// DeleteOperator deletes the specified operator.
	DeleteOperator(appName string) error

	// EnsureCustomResourceDefinition creates or updates a custom resource definition resource.
	EnsureCustomResourceDefinition(appName string, podSpec *PodSpec) error

	// WatchUnits returns a watcher which notifies when there
	// are changes to units of the specified application.
	WatchUnits(appName string) (watcher.NotifyWatcher, error)

	// Units returns all units and any associated filesystems
	// of the specified application. Filesystems are mounted
	// via volumes bound to the unit.
	Units(appName string) ([]Unit, error)

	// WatchOperator returns a watcher which notifies when there
	// are changes to the operator of the specified application.
	WatchOperator(string) (watcher.NotifyWatcher, error)

	// WatchService returns a watcher which notifies when there
	// are changes to the deployment of the specified application.
	WatchService(appName string) (watcher.NotifyWatcher, error)

	// Operator returns an Operator with current status and life details.
	Operator(string) (*Operator, error)

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
	GetService(appName string, includeClusterIP bool) (*Service, error)
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
	EnsureStorageProvisioner(cfg StorageProvisioner) (*StorageProvisioner, error)
}

// NamespaceWatcher provides the API to watch caas namespace.
type NamespaceWatcher interface {
	// WatchNamespace returns a watcher which notifies when there
	// are changes to current namespace.
	WatchNamespace() (watcher.NotifyWatcher, error)
}

// Service represents information about the status of a caas service entity.
type Service struct {
	Id        string
	Addresses []network.Address
	Scale     *int
	Status    status.StatusInfo
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

// OperatorConfig is the config to use when creating an operator.
type OperatorConfig struct {
	// OperatorImagePath is the docker registry URL for the image.
	OperatorImagePath string

	// Version is the Juju version of the operator image.
	Version version.Number

	// CharmStorage defines parameters used to create storage
	// for operators to use for charm state.
	CharmStorage CharmStorageParams

	// AgentConf is the contents of the agent.conf file.
	AgentConf []byte

	// ResourceTags is a set of tags to set on the operator pod.
	ResourceTags map[string]string
}
