// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"io"

	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/models"
	"github.com/juju/juju/rpc/params"
)

// FirewallerAPI exposes functionality off the firewaller API facade to a worker.
type FirewallerAPI interface {
	WatchModelMachines() (watcher.StringsWatcher, error)
	WatchOpenedPorts() (watcher.StringsWatcher, error)
	WatchModelFirewallRules() (watcher.NotifyWatcher, error)
	ModelFirewallRules() (firewall.IngressRules, error)
	ModelConfig() (*config.Config, error)
	Machine(tag names.MachineTag) (Machine, error)
	Unit(tag names.UnitTag) (Unit, error)
	Relation(tag names.RelationTag) (*firewaller.Relation, error)
	WatchEgressAddressesForRelation(tag names.RelationTag) (watcher.StringsWatcher, error)
	WatchIngressAddressesForRelation(tag names.RelationTag) (watcher.StringsWatcher, error)
	ControllerAPIInfoForModel(modelUUID string) (*api.Info, error)
	MacaroonForRelation(relationKey string) (*macaroon.Macaroon, error)
	SetRelationStatus(relationKey string, status relation.Status, message string) error
	AllSpaceInfos() (network.SpaceInfos, error)
	WatchSubnets() (watcher.StringsWatcher, error)
}

// CrossModelFirewallerFacade exposes firewaller functionality on the
// remote offering model to a worker.
type CrossModelFirewallerFacade interface {
	PublishIngressNetworkChange(params.IngressNetworksChangeEvent) error
	WatchEgressAddressesForRelation(details params.RemoteEntityArg) (watcher.StringsWatcher, error)
}

// CrossModelFirewallerFacadeCloser implements CrossModelFirewallerFacade
// and adds a Close() method.
type CrossModelFirewallerFacadeCloser interface {
	io.Closer
	CrossModelFirewallerFacade
}

// RemoteRelationsAPI provides the remote relations facade.
type RemoteRelationsAPI interface {
	GetToken(names.Tag) (string, error)
	Relations(keys []string) ([]params.RemoteRelationResult, error)
	RemoteApplications(names []string) ([]params.RemoteApplicationResult, error)
	WatchRemoteRelations() (watcher.StringsWatcher, error)
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
	Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error)
}

// EnvironInstance represents an instance with firewall apis.
type EnvironInstance interface {
	instances.Instance
	instances.InstanceFirewaller
}

// Machine represents a model machine.
type Machine interface {
	Tag() names.MachineTag
	WatchUnits() (watcher.StringsWatcher, error)
	InstanceId() (instance.Id, error)
	Life() life.Value
	IsManual() (bool, error)
	OpenedMachinePortRanges() (byUnitAndCIDR map[names.UnitTag]network.GroupedPortRanges, byUnitAndEndpoint map[names.UnitTag]network.GroupedPortRanges, err error)
}

// Unit represents a model unit.
type Unit interface {
	Name() string
	Tag() names.UnitTag
	Life() life.Value
	Refresh() error
	Application() (Application, error)
	AssignedMachine() (names.MachineTag, error)
}

// Application represents a model application.
type Application interface {
	Name() string
	Tag() names.ApplicationTag
	Watch() (watcher.NotifyWatcher, error)
	ExposeInfo() (bool, map[string]params.ExposedEndpoint, error)
}
