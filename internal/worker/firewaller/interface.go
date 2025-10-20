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
	domainrelation "github.com/juju/juju/domain/relation"
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
	ControllerAPIInfoForModel(ctx context.Context, modelUUID string) (*api.Info, error)
	SetRelationStatus(ctx context.Context, relationKey string, status relation.Status, message string) error
	AllSpaceInfos(ctx context.Context) (network.SpaceInfos, error)
	WatchSubnets(ctx context.Context) (watcher.StringsWatcher, error)
}

// CrossModelFirewallerFacade exposes firewaller functionality on the
// remote offering model to a worker.
type CrossModelFirewallerFacade interface {
	// PublishIngressNetworkChange publishes changes to the required
	// ingress addresses to the model hosting the offer in the relation.
	PublishIngressNetworkChange(context.Context, params.IngressNetworksChangeEvent) error

	// WatchEgressAddressesForRelation creates a watcher that notifies when
	// addresses, from which connections will originate for the relation,
	// change.
	// Each event contains the entire set of addresses which are required for
	// ingress for the relation.
	WatchEgressAddressesForRelation(ctx context.Context, details params.RemoteEntityArg) (watcher.StringsWatcher, error)
}

// CrossModelFirewallerFacadeCloser implements CrossModelFirewallerFacade
// and adds a Close() method.
type CrossModelFirewallerFacadeCloser interface {
	io.Closer
	CrossModelFirewallerFacade
}

// CrossModelRelationService provides access to cross-model relations.
type CrossModelRelationService interface {
	// GetMacaroonForRelation retrieves the macaroon associated with the provided
	// relation key.
	GetMacaroonForRelationKey(ctx context.Context, relationKey relation.Key) (*macaroon.Macaroon, error)

	// GetRelationNetworkEgress retrieves all egress network CIDRs for the
	// specified relation.
	GetRelationNetworkEgress(ctx context.Context, relationUUID string) ([]string, error)

	// GetRelationNetworkIngress retrieves all ingress network CIDRs for the
	// specified relation.
	GetRelationNetworkIngress(ctx context.Context, relationUUID string) ([]string, error)

	// RemoteApplications returns the current state for the named remote applications.
	RemoteApplications(ctx context.Context, applications []string) ([]params.RemoteApplicationResult, error)

	// WatchConsumerRelations watches the changes to (remote) relations on the
	// consuming model and notifies the worker of any changes.
	WatchConsumerRelations(ctx context.Context) (watcher.StringsWatcher, error)

	// WatchOffererRelations watches the changes to (remote) relations on the
	// offering model and notifies the worker of any changes.
	WatchOffererRelations(ctx context.Context) (watcher.StringsWatcher, error)

	// WatchRelationEgressNetworks watches for changes to the egress networks
	// for the specified relation UUID. It returns a NotifyWatcher that emits
	// events when there are insertions or deletions in the relation_network_egress
	// table.
	WatchRelationEgressNetworks(ctx context.Context, relationUUID relation.UUID) (watcher.NotifyWatcher, error)

	// WatchRelationIngressNetworks watches for changes to the ingress networks
	// for the specified relation UUID. It returns a NotifyWatcher that emits
	// events when there are insertions or deletions in the relation_network_ingress
	// table.
	WatchRelationIngressNetworks(ctx context.Context, relationUUID relation.UUID) (watcher.NotifyWatcher, error)
}

// RelationService provides access to relations.
type RelationService interface {
	// GetRelationUUIDByKey returns a relation UUID for the given Key.
	GetRelationUUIDByKey(ctx context.Context, relationKey relation.Key) (relation.UUID, error)

	// GetRelationDetails returns RelationDetails for the given relation UUID.
	GetRelationDetails(ctx context.Context, relationUUID relation.UUID) (domainrelation.RelationDetails, error)

	// SetRelationStatus sets the status and message for the given relation
	// UUID.
	SetRelationStatus(ctx context.Context, relationUUID relation.UUID, status relation.Status, message string) error
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
	GetMachineUUID(ctx context.Context, name machine.Name) (machine.UUID, error)
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

	// WatchUnitAddRemoveOnMachine returns a watcher that observes changes to the
	// units on a specified machine, emitting the names of the units. That is, we
	// emit unit names only when a unit is create or deleted on the specified machine.
	// The following errors may be returned:
	// - [applicationerrors.MachineNotFound] if the machine does not exist
	WatchUnitAddRemoveOnMachine(context.Context, machine.Name) (watcher.StringsWatcher, error)

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

	// GetUnitMachineName gets the name of the unit's machine.
	//
	// The following errors may be returned:
	//   - [applicationerrors.UnitMachineNotAssigned] if the unit does not have a
	//     machine assigned.
	//   - [applicationerrors.UnitNotFound] if the unit cannot be found.
	//   - [applicationerrors.UnitIsDead] if the unit is dead.
	GetUnitMachineName(context.Context, unit.Name) (machine.Name, error)
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
}

// Application represents a model application.
type Application interface {
	Name() string
	Tag() names.ApplicationTag
}
