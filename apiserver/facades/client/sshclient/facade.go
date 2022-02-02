// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sshclient implements the API endpoint required for Juju
// clients that wish to make SSH connections to Juju managed machines.
package sshclient

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/state/stateenvirons"
)

// Facade implements the API required by the sshclient worker.
type Facade struct {
	backend     Backend
	authorizer  facade.Authorizer
	callContext context.ProviderCallContext

	leadershipReader leadership.Reader
}

type FacadeV2 struct {
	*Facade
}

func NewFacade(ctx facade.Context) (*Facade, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	leadershipReader, err := ctx.LeadershipReader(m.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return internalFacade(
		&backend{st, stateenvirons.EnvironConfigGetter{Model: m}, m.ModelTag()},
		leadershipReader,
		ctx.Auth(),
		context.CallContext(st))
}

// NewFacade is used for API registration.
func NewFacadeV2(ctx facade.Context) (*FacadeV2, error) {
	f, err := NewFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &FacadeV2{Facade: f}, nil
}

func internalFacade(backend Backend, leadershipReader leadership.Reader, auth facade.Authorizer, callCtx context.ProviderCallContext) (*Facade, error) {
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &Facade{backend: backend, authorizer: auth, callContext: callCtx, leadershipReader: leadershipReader}, nil
}

func (facade *Facade) checkIsModelAdmin() error {
	isModelAdmin, err := facade.authorizer.HasPermission(permission.AdminAccess, facade.backend.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !isModelAdmin {
		return apiservererrors.ErrPerm
	}
	return nil
}

// PublicAddress reports the preferred public network address for one
// or more entities. Machines and units are supported.
func (facade *Facade) PublicAddress(args params.Entities) (params.SSHAddressResults, error) {
	if err := facade.checkIsModelAdmin(); err != nil {
		return params.SSHAddressResults{}, errors.Trace(err)
	}

	getter := func(m SSHMachine) (network.SpaceAddress, error) { return m.PublicAddress() }
	return facade.getAddressPerEntity(args, getter)
}

// PrivateAddress reports the preferred private network address for one or
// more entities. Machines and units are supported.
func (facade *Facade) PrivateAddress(args params.Entities) (params.SSHAddressResults, error) {
	if err := facade.checkIsModelAdmin(); err != nil {
		return params.SSHAddressResults{}, errors.Trace(err)
	}

	getter := func(m SSHMachine) (network.SpaceAddress, error) { return m.PrivateAddress() }
	return facade.getAddressPerEntity(args, getter)
}

// AllAddresses reports all addresses that might have SSH listening for each given
// entity in args. Machines and units are supported as entity types.
// TODO(wpk): 2017-05-17 This is a temporary solution, we should not fetch environ here
// but get the addresses from state. We will be changing it since we want to have space-aware
// SSH settings.
func (facade *Facade) AllAddresses(args params.Entities) (params.SSHAddressesResults, error) {
	if err := facade.checkIsModelAdmin(); err != nil {
		return params.SSHAddressesResults{}, errors.Trace(err)
	}
	env, err := environs.GetEnviron(facade.backend, environs.New)
	if err != nil {
		return params.SSHAddressesResults{}, errors.Annotate(err, "opening environment")
	}

	environ, supportsNetworking := environs.SupportsNetworking(env)
	getter := func(m SSHMachine) ([]network.SpaceAddress, error) {
		devicesAddresses, err := m.AllDeviceSpaceAddresses()
		if err != nil {
			return nil, errors.Trace(err)
		}
		legacyAddresses := m.Addresses()
		devicesAddresses = append(devicesAddresses, legacyAddresses...)

		// Make the list unique
		addressMap := make(map[string]bool)
		var uniqueAddresses []network.SpaceAddress
		for _, address := range devicesAddresses {
			if !addressMap[address.Value] {
				addressMap[address.Value] = true
				uniqueAddresses = append(uniqueAddresses, address)
			}
		}

		if supportsNetworking {
			return environ.SSHAddresses(facade.callContext, uniqueAddresses)
		} else {
			return uniqueAddresses, nil
		}
	}

	return facade.getAllEntityAddresses(args, getter)
}

func (facade *Facade) getAllEntityAddresses(args params.Entities, getter func(SSHMachine) ([]network.SpaceAddress, error)) (
	params.SSHAddressesResults, error,
) {
	out := params.SSHAddressesResults{
		Results: make([]params.SSHAddressesResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		machine, err := facade.backend.GetMachineForEntity(entity.Tag)
		if err != nil {
			out.Results[i].Error = apiservererrors.ServerError(err)
		} else {
			addresses, err := getter(machine)
			if err != nil {
				out.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}

			out.Results[i].Addresses = make([]string, len(addresses))
			for j := range addresses {
				out.Results[i].Addresses[j] = addresses[j].Value
			}
		}
	}
	return out, nil
}

func (facade *Facade) getAddressPerEntity(args params.Entities, addressGetter func(SSHMachine) (network.SpaceAddress, error)) (
	params.SSHAddressResults, error,
) {
	out := params.SSHAddressResults{
		Results: make([]params.SSHAddressResult, len(args.Entities)),
	}

	getter := func(m SSHMachine) ([]network.SpaceAddress, error) {
		address, err := addressGetter(m)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return []network.SpaceAddress{address}, nil
	}

	fullResults, err := facade.getAllEntityAddresses(args, getter)
	if err != nil {
		return params.SSHAddressResults{}, errors.Trace(err)
	}

	for i, result := range fullResults.Results {
		if result.Error != nil {
			out.Results[i].Error = result.Error
		} else {
			out.Results[i].Address = result.Addresses[0]
		}
	}

	return out, nil
}

// PublicKeys returns the public SSH hosts for one or more
// entities. Machines and units are supported.
func (facade *Facade) PublicKeys(args params.Entities) (params.SSHPublicKeysResults, error) {
	if err := facade.checkIsModelAdmin(); err != nil {
		return params.SSHPublicKeysResults{}, errors.Trace(err)
	}

	out := params.SSHPublicKeysResults{
		Results: make([]params.SSHPublicKeysResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		machine, err := facade.backend.GetMachineForEntity(entity.Tag)
		if err != nil {
			out.Results[i].Error = apiservererrors.ServerError(err)
		} else {
			keys, err := facade.backend.GetSSHHostKeys(machine.MachineTag())
			if err != nil {
				out.Results[i].Error = apiservererrors.ServerError(err)
			} else {
				out.Results[i].PublicKeys = []string(keys)
			}
		}
	}
	return out, nil
}

// Proxy returns whether SSH connections should be proxied through the
// controller hosts for the model associated with the API connection.
func (facade *Facade) Proxy() (params.SSHProxyResult, error) {
	if err := facade.checkIsModelAdmin(); err != nil {
		return params.SSHProxyResult{}, errors.Trace(err)
	}
	config, err := facade.backend.ModelConfig()
	if err != nil {
		return params.SSHProxyResult{}, err
	}
	return params.SSHProxyResult{UseProxy: config.ProxySSH()}, nil
}

// Leader is not available via the V2 Facade.
func (a *FacadeV2) Leader(_ struct{}) {}

// Leader returns the unit name of the leader for the given application.
func (facade *Facade) Leader(entity params.Entity) (params.StringResult, error) {
	result := params.StringResult{}
	application, err := names.ParseApplicationTag(entity.Tag)
	if err != nil {
		return result, err
	}
	leaders, err := facade.leadershipReader.Leaders()
	if err != nil {
		return result, errors.Annotate(err, "could not fetch leaders")
	}
	var ok bool
	result.Result, ok = leaders[application.Name]
	if !ok || result.Result == "" {
		result.Error = apiservererrors.ServerError(errors.NotFoundf("leader for %s", entity.Tag))
	}
	return result, nil
}
