// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/watcher"
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
}

// Broker instances interact with the CAAS substrate.
type Broker interface {
	// Provider returns the ContainerEnvironProvider that created this Broker.
	Provider() ContainerEnvironProvider

	// Destroy terminates all containers and other resources in this broker's namespace.
	Destroy(context.ProviderCallContext) error

	// EnsureNamespace ensures this broker's namespace is created.
	EnsureNamespace() error

	// EnsureOperator creates or updates an operator pod for running
	// a charm for the specified application.
	EnsureOperator(appName, agentPath string, config *OperatorConfig) error

	// DeleteOperator deletes the specified operator.
	DeleteOperator(appName string) error

	// EnsureService creates or updates a service for pods with the given params.
	EnsureService(appName string, params *ServiceParams, numUnits int, config application.ConfigAttributes) error

	// Service returns the service for the specified application.
	Service(appName string) (*Service, error)

	// DeleteService deletes the specified service.
	DeleteService(appName string) error

	// ExposeService sets up external access to the specified service.
	ExposeService(appName string, config application.ConfigAttributes) error

	// UnexposeService removes external access to the specified service.
	UnexposeService(appName string) error

	// WatchUnits returns a watcher which notifies when there
	// are changes to units of the specified application.
	WatchUnits(appName string) (watcher.NotifyWatcher, error)

	// Units returns all units and any associated filesystems
	// of the specified application. Filesystems are mounted
	// via volumes bound to the unit.
	Units(appName string) ([]Unit, error)

	// ProviderRegistry is an interface for obtaining storage providers.
	storage.ProviderRegistry
}

// Service represents information about the status of a caas service entity.
type Service struct {
	Id        string
	Addresses []network.Address
}

// FilesystemInfo represents information about a filesystem
// mounted by a unit.
type FilesystemInfo struct {
	StorageName  string
	FilesystemId string
	Size         uint64
	MountPoint   string
	ReadOnly     bool
}

// Unit represents information about the status of a "pod".
type Unit struct {
	Id             string
	Address        string
	Ports          []string
	Dying          bool
	Status         status.StatusInfo
	FilesystemInfo []FilesystemInfo
}

// OperatorConfig is the config to use when creating an operator.
type OperatorConfig struct {
	// OperatorImagePath is the docker registry URL for the image.
	OperatorImagePath string

	// Version is the Juju version of the operator image.
	Version version.Number

	// AgentConf is the contents of the agent.conf file.
	AgentConf []byte
}
