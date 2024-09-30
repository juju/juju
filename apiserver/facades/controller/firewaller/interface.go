// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// State provides the subset of global state required by the
// remote firewaller facade.
type State interface {
	firewall.State

	IsController() bool
	ModelUUID() string
	GetMacaroon(entity names.Tag) (*macaroon.Macaroon, error)
	WatchOpenedPorts() state.StringsWatcher
	FindEntity(tag names.Tag) (state.Entity, error)
	AllEndpointBindings() (map[string]map[string]string, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	// Watch returns a watcher that observes changes to subnets and their
	// association (fan underlays), filtered based on the provided list of subnets
	// to watch.
	WatchSubnets(ctx context.Context, subnetUUIDsToWatch set.Strings) (watcher.StringsWatcher, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// EnsureDeadMachine sets the provided machine's life status to Dead.
	// No error is returned if the provided machine doesn't exist, just nothing
	// gets updated.
	EnsureDeadMachine(ctx context.Context, machineName machine.Name) error
	// GetMachineUUID returns the UUID of a machine identified by its name.
	// It returns a MachineNotFound if the machine does not exist.
	GetMachineUUID(ctx context.Context, name machine.Name) (string, error)
	// InstanceID returns the cloud specific instance id for this machine.
	InstanceID(ctx context.Context, mUUID string) (string, error)
}

// ApplicationService provides access to the application service.
type ApplicationService interface {
	GetUnitLife(context.Context, string) (life.Value, error)
}

// ControllerConfigAPI provides the subset of common.ControllerConfigAPI
// required by the remote firewaller facade
type ControllerConfigAPI interface {
	// ControllerConfig returns the controller's configuration.
	ControllerConfig(context.Context) (params.ControllerConfigResult, error)

	// ControllerAPIInfoForModels returns the controller api connection details for the specified models.
	ControllerAPIInfoForModels(ctx context.Context, args params.Entities) (params.ControllerAPIInfoResults, error)
}

// TODO(wallyworld) - for tests, remove when remaining firewaller tests become unit tests.
func StateShim(st *state.State, m *state.Model) stateShim {
	return stateShim{st: st, State: firewall.StateShim(st, m)}
}

type stateShim struct {
	firewall.State
	st *state.State
}

func (st stateShim) IsController() bool {
	return st.st.IsController()
}

func (st stateShim) ModelUUID() string {
	return st.st.ModelUUID()
}

func (st stateShim) GetMacaroon(entity names.Tag) (*macaroon.Macaroon, error) {
	r := st.st.RemoteEntities()
	return r.GetMacaroon(entity)
}

func (st stateShim) FindEntity(tag names.Tag) (state.Entity, error) {
	return st.st.FindEntity(tag)
}

func (st stateShim) WatchOpenedPorts() state.StringsWatcher {
	return st.st.WatchOpenedPorts()
}

func (st stateShim) AllEndpointBindings() (map[string]map[string]string, error) {
	model, err := st.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	allEndpointBindings, err := model.AllEndpointBindings()
	if err != nil {
		return nil, errors.Trace(err)
	}

	res := make(map[string]map[string]string, len(allEndpointBindings))
	for appName, bindings := range allEndpointBindings {
		res[appName] = bindings.Map() // endpoint -> spaceID
	}
	return res, nil
}
