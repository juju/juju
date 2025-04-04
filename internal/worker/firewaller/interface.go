// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"
	"io"

	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/models"
	"github.com/juju/juju/rpc/params"
)

// FirewallerAPI exposes functionality off the firewaller API facade to a worker.
type FirewallerAPI interface {
	WatchModelMachines(context.Context) (watcher.StringsWatcher, error)
	WatchModelFirewallRules(context.Context) (watcher.NotifyWatcher, error)
	ModelFirewallRules(context.Context) (firewall.IngressRules, error)
	ModelConfig(context.Context) (*config.Config, error)
	Machine(ctx context.Context, tag names.MachineTag) (Machine, error)
	Unit(ctx context.Context, tag names.UnitTag) (Unit, error)
	Relation(ctx context.Context, tag names.RelationTag) (*firewaller.Relation, error)
	WatchEgressAddressesForRelation(ctx context.Context, tag names.RelationTag) (watcher.StringsWatcher, error)
	WatchIngressAddressesForRelation(ctx context.Context, tag names.RelationTag) (watcher.StringsWatcher, error)
	ControllerAPIInfoForModel(ctx context.Context, modelUUID string) (*api.Info, error)
	MacaroonForRelation(ctx context.Context, relationKey string) (*macaroon.Macaroon, error)
	SetRelationStatus(ctx context.Context, relationKey string, status relation.Status, message string) error
	AllSpaceInfos(ctx context.Context) (network.SpaceInfos, error)
	WatchSubnets(ctx context.Context) (watcher.StringsWatcher, error)
}

// CrossModelFirewallerFacade exposes firewaller functionality on the
// remote offering model to a worker.
type CrossModelFirewallerFacade interface {
	PublishIngressNetworkChange(context.Context, params.IngressNetworksChangeEvent) error
	WatchEgressAddressesForRelation(ctx context.Context, details params.RemoteEntityArg) (watcher.StringsWatcher, error)
}

// CrossModelFirewallerFacadeCloser implements CrossModelFirewallerFacade
// and adds a Close() method.
type CrossModelFirewallerFacadeCloser interface {
	io.Closer
	CrossModelFirewallerFacade
}

// RemoteRelationsAPI provides the remote relations facade.
type RemoteRelationsAPI interface {
	GetToken(context.Context, names.Tag) (string, error)
	Relations(ctx context.Context, keys []string) ([]params.RemoteRelationResult, error)
	RemoteApplications(ctx context.Context, names []string) ([]params.RemoteApplicationResult, error)
	WatchRemoteRelations(context.Context) (watcher.StringsWatcher, error)
}

// PortService provides methods to query opened ports for machines
type PortService interface {
	// WatchMachineOpenedPorts returns a strings watcher for opened ports. This watcher
	// emits events for changes to the opened ports table. Each emitted event
	// contains the machine name which is associated with the changed port range.
	WatchMachineOpenedPorts(ctx context.Context) (watcher.StringsWatcher, error)

	// GetMachineOpenedPorts returns the opened ports for all endpoints, for all the
	// units on the machine. Opened ports are grouped first by unit name and then by
	// endpoint.
	GetMachineOpenedPorts(ctx context.Context, machineUUID string) (map[unit.Name]network.GroupedPortRanges, error)
}

// MachineService provides methods to query machines.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	// It returns a MachineNotFound if the machine does not exist.
	GetMachineUUID(ctx context.Context, name machine.Name) (string, error)
}

// ApplicationService provides methods to query applications.
type ApplicationService interface {
	// WatchApplicationExposed watches for changes to the specified application's
	// exposed endpoints.
	// This notifies on any changes to the application's exposed endpoints. It is up
	// to the caller to determine if the exposed endpoints they're interested in has
	// changed.
	//
	// If the application does not exist an error satisfying
	// [applicationerrors.NotFound] will be returned.
	WatchApplicationExposed(ctx context.Context, name string) (watcher.NotifyWatcher, error)

	// IsApplicationExposed returns whether the provided application is exposed or not.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	IsApplicationExposed(ctx context.Context, appName string) (bool, error)

	// GetExposedEndpoints returns map where keys are endpoint names (or the ""
	// value which represents all endpoints) and values are ExposedEndpoint
	// instances that specify which sources (spaces or CIDRs) can access the
	// opened ports for each endpoint once the application is exposed.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetExposedEndpoints(ctx context.Context, appName string) (map[string]application.ExposedEndpoint, error)
}

// EnvironFirewaller defines methods to allow the worker to perform
// firewall operations (open/close ports) on a Juju global firewall.
type EnvironFirewaller interface {
	environs.Firewaller
}

// EnvironModelFirewaller defines methods to allow the worker to
// perform firewall operations (open/close port) on a Juju model firewall.
type EnvironModelFirewaller interface {
	models.ModelFirewaller
}

// EnvironInstances defines methods to allow the worker to perform
// operations on instances in a Juju cloud environment.
type EnvironInstances interface {
	Instances(ctx context.Context, ids []instance.Id) ([]instances.Instance, error)
}

// EnvironInstance represents an instance with firewall apis.
type EnvironInstance interface {
	instances.Instance
	instances.InstanceFirewaller
}

// Machine represents a model machine.
type Machine interface {
	Tag() names.MachineTag
	WatchUnits(context.Context) (watcher.StringsWatcher, error)
	InstanceId(context.Context) (instance.Id, error)
	Life() life.Value
	IsManual(context.Context) (bool, error)
}

// Unit represents a model unit.
type Unit interface {
	Name() string
	Life() life.Value
	Refresh(ctx context.Context) error
	Application() (Application, error)
	AssignedMachine(context.Context) (names.MachineTag, error)
}

// Application represents a model application.
type Application interface {
	Name() string
	Tag() names.ApplicationTag
}
