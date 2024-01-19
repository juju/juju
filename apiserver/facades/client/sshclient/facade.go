// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	stdcontext "context"
	coreuser "github.com/juju/juju/core/user"
	"sort"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type newCaasBrokerFunc func(_ stdcontext.Context, args environs.OpenParams) (Broker, error)

// Facade implements the API required by the sshclient worker.
type Facade struct {
	backend    Backend
	authorizer facade.Authorizer

	leadershipReader leadership.Reader
	getBroker        newCaasBrokerFunc
}

func internalFacade(
	backend Backend, leadershipReader leadership.Reader, auth facade.Authorizer,
	getBroker newCaasBrokerFunc,
) (*Facade, error) {
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &Facade{
		backend:          backend,
		authorizer:       auth,
		leadershipReader: leadershipReader,
		getBroker:        getBroker,
	}, nil
}

func (facade *Facade) checkIsModelAdmin(usr coreuser.User) error {
	err := facade.authorizer.HasPermission(usr, permission.SuperuserAccess, facade.backend.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return facade.authorizer.HasPermission(usr, permission.AdminAccess, facade.backend.ModelTag())
}

// PublicAddress reports the preferred public network address for one
// or more entities. Machines and units are supported.
func (facade *Facade) PublicAddress(ctx stdcontext.Context, args params.Entities) (params.SSHAddressResults, error) {
	if err := facade.checkIsModelAdmin(usr); err != nil {
		return params.SSHAddressResults{}, errors.Trace(err)
	}

	getter := func(m SSHMachine) (network.SpaceAddress, error) { return m.PublicAddress() }
	return facade.getAddressPerEntity(args, getter)
}

// PrivateAddress reports the preferred private network address for one or
// more entities. Machines and units are supported.
func (facade *Facade) PrivateAddress(ctx stdcontext.Context, args params.Entities) (params.SSHAddressResults, error) {
	if err := facade.checkIsModelAdmin(usr); err != nil {
		return params.SSHAddressResults{}, errors.Trace(err)
	}

	getter := func(m SSHMachine) (network.SpaceAddress, error) { return m.PrivateAddress() }
	return facade.getAddressPerEntity(args, getter)
}

// AllAddresses reports all addresses that might have SSH listening for each
// entity in args. The result is sorted with public addresses first.
// Machines and units are supported as entity types.
func (facade *Facade) AllAddresses(ctx stdcontext.Context, args params.Entities) (params.SSHAddressesResults, error) {
	if err := facade.checkIsModelAdmin(usr); err != nil {
		return params.SSHAddressesResults{}, errors.Trace(err)
	}

	getter := func(m SSHMachine) ([]network.SpaceAddress, error) {
		devicesAddresses, err := m.AllDeviceSpaceAddresses()
		if err != nil {
			return nil, errors.Trace(err)
		}
		legacyAddresses := m.Addresses()
		devicesAddresses = append(devicesAddresses, legacyAddresses...)

		// Make the list unique
		addressMap := make(map[string]bool)
		var uniqueAddresses network.SpaceAddresses
		for _, address := range devicesAddresses {
			if !addressMap[address.Value] {
				addressMap[address.Value] = true
				uniqueAddresses = append(uniqueAddresses, address)
			}
		}

		sort.Sort(uniqueAddresses)
		return uniqueAddresses, nil
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
func (facade *Facade) PublicKeys(ctx stdcontext.Context, args params.Entities) (params.SSHPublicKeysResults, error) {
	if err := facade.checkIsModelAdmin(usr); err != nil {
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
func (facade *Facade) Proxy(ctx stdcontext.Context) (params.SSHProxyResult, error) {
	if err := facade.checkIsModelAdmin(usr); err != nil {
		return params.SSHProxyResult{}, errors.Trace(err)
	}
	config, err := facade.backend.ModelConfig(ctx)
	if err != nil {
		return params.SSHProxyResult{}, err
	}
	return params.SSHProxyResult{UseProxy: config.ProxySSH()}, nil
}

// ModelCredentialForSSH returns a cloud spec for ssh purpose.
// This facade call is only used for k8s model.
func (facade *Facade) ModelCredentialForSSH(ctx stdcontext.Context) (params.CloudSpecResult, error) {
	var result params.CloudSpecResult

	if err := facade.checkIsModelAdmin(usr); err != nil {
		return result, err
	}

	model, err := facade.backend.Model()
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	if model.Type() != state.ModelTypeCAAS {
		result.Error = apiservererrors.ServerError(errors.NotSupportedf("facade ModelCredentialForSSH for non %q model", state.ModelTypeCAAS))
		return result, nil
	}

	spec, err := facade.backend.CloudSpec(ctx)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	if spec.Credential == nil {
		result.Error = apiservererrors.ServerError(errors.NotValidf("cloud spec %q has empty credential", spec.Name))
		return result, nil
	}

	token, err := facade.getExecSecretToken(ctx, spec, model)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}

	cred, err := k8scloud.UpdateCredentialWithToken(*spec.Credential, token)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	result.Result = &params.CloudSpec{
		Type:             spec.Type,
		Name:             spec.Name,
		Region:           spec.Region,
		Endpoint:         spec.Endpoint,
		IdentityEndpoint: spec.IdentityEndpoint,
		StorageEndpoint:  spec.StorageEndpoint,
		Credential: &params.CloudCredential{
			AuthType:   string(cred.AuthType()),
			Attributes: cred.Attributes(),
		},
		CACertificates:    spec.CACertificates,
		SkipTLSVerify:     spec.SkipTLSVerify,
		IsControllerCloud: spec.IsControllerCloud,
	}
	return result, nil
}

func (facade *Facade) getExecSecretToken(ctx stdcontext.Context, cloudSpec environscloudspec.CloudSpec, model Model) (string, error) {
	cfg, err := model.Config()
	if err != nil {
		return "", errors.Trace(err)
	}

	broker, err := facade.getBroker(ctx, environs.OpenParams{
		ControllerUUID: model.ControllerUUID(),
		Cloud:          cloudSpec,
		Config:         cfg,
	})
	if err != nil {
		return "", errors.Annotate(err, "failed to open kubernetes client")
	}
	return broker.GetSecretToken(ctx, k8sprovider.ExecRBACResourceName)
}
