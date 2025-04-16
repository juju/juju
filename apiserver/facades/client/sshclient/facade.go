// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	stdcontext "context"
	"sort"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type newCaasBrokerFunc func(_ stdcontext.Context, args environs.OpenParams) (Broker, error)

// Facade implements the API required by the sshclient worker.
type Facade struct {
	backend     Backend
	authorizer  facade.Authorizer
	callContext context.ProviderCallContext

	leadershipReader leadership.Reader
	getBroker        newCaasBrokerFunc
}

// FacadeV5 provides the SSH Client API facade version 5
// which adds VirtualHostname.
type FacadeV5 struct {
	*Facade
}

// FacadeV4 provides the SSH Client API facade version 4.
type FacadeV4 struct {
	*FacadeV5
}

func internalFacade(
	backend Backend, leadershipReader leadership.Reader, auth facade.Authorizer, callCtx context.ProviderCallContext,
	getBroker newCaasBrokerFunc,
) (*Facade, error) {
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &Facade{
		backend:          backend,
		authorizer:       auth,
		callContext:      callCtx,
		leadershipReader: leadershipReader,
		getBroker:        getBroker,
	}, nil
}

func (facade *Facade) checkIsModelAdmin() error {
	err := facade.authorizer.HasPermission(permission.SuperuserAccess, facade.backend.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return facade.authorizer.HasPermission(permission.AdminAccess, facade.backend.ModelTag())
}

func (facade *Facade) checkIsModelReader() error {
	// Check if superuser, if it's not a missing perm error, the user may have
	// a lower level of permission (Write, Read) for the model.
	err := facade.authorizer.HasPermission(permission.SuperuserAccess, facade.backend.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return facade.authorizer.HasPermission(permission.ReadAccess, facade.backend.ModelTag())
}

// VirtualHostname is not implemented in v4.
func (f *FacadeV4) VirtualHostname(_, _, _ struct{}) {}

// VirtualHostname returns the virtual hostname for the given entity.
func (facade *Facade) VirtualHostname(arg params.VirtualHostnameTargetArg) (params.SSHAddressResult, error) {
	if err := facade.checkIsModelAdmin(); err != nil {
		return params.SSHAddressResult{}, errors.Trace(err)
	}
	modelUUID := facade.backend.ModelTag().Id()
	virtualHostname, err := getVirtualHostnameForEntity(modelUUID, arg.Tag, arg.Container)
	if err != nil {
		return params.SSHAddressResult{
			Error: apiservererrors.ServerError(err),
		}, errors.Trace(err)
	}
	return params.SSHAddressResult{
		Address: virtualHostname,
	}, nil
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

// AllAddresses reports all addresses that might have SSH listening for each
// entity in args. The result is sorted with public addresses first.
// Machines and units are supported as entity types.
func (facade *Facade) AllAddresses(args params.Entities) (params.SSHAddressesResults, error) {
	if err := facade.checkIsModelAdmin(); err != nil {
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

// ModelCredentialForSSH returns a cloud spec for ssh purpose.
// This facade call is only used for k8s model.
func (facade *Facade) ModelCredentialForSSH() (params.CloudSpecResult, error) {
	var result params.CloudSpecResult

	if err := facade.checkIsModelAdmin(); err != nil {
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

	spec, err := facade.backend.CloudSpec()
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	if spec.Credential == nil {
		result.Error = apiservererrors.ServerError(errors.NotValidf("cloud spec %q has empty credential", spec.Name))
		return result, nil
	}

	token, err := facade.getExecSecretToken(spec, model)
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

func (facade *Facade) getExecSecretToken(cloudSpec environscloudspec.CloudSpec, model Model) (string, error) {
	cfg, err := model.Config()
	if err != nil {
		return "", errors.Trace(err)
	}

	broker, err := facade.getBroker(stdcontext.TODO(), environs.OpenParams{
		ControllerUUID: model.ControllerUUID(),
		Cloud:          cloudSpec,
		Config:         cfg,
	})
	if err != nil {
		return "", errors.Annotate(err, "failed to open kubernetes client")
	}
	return broker.GetSecretToken(k8sprovider.ExecRBACResourceName)
}

// PublicHostKeyForTarget returns the virtual host key for the target host. In addition, it also returns
// the jump server's host key.
func (facade *Facade) PublicHostKeyForTarget(arg params.SSHVirtualHostKeyRequestArg) params.PublicSSHHostKeyResult {
	var res params.PublicSSHHostKeyResult

	// Check if superuser or at least model reader
	if err := facade.checkIsModelReader(); err != nil {
		res.Error = apiservererrors.ServerError(err)
		return res
	}

	info, err := virtualhostname.Parse(arg.Hostname)
	if err != nil {
		res.Error = apiservererrors.ServerError(errors.Annotate(err, "failed to parse hostname"))
		return res
	}

	var pubKey []byte
	switch info.Target() {
	case virtualhostname.MachineTarget:
		machineId, _ := info.Machine()
		pubKey, err = facade.backend.MachineVirtualPublicKey(strconv.Itoa(machineId))
		if err != nil {
			res.Error = apiservererrors.ServerError(errors.Annotate(err, "failed to get machine host key"))
			return res
		}
	case virtualhostname.ContainerTarget, virtualhostname.UnitTarget:
		unitName, _ := info.Unit()
		pubKey, err = facade.backend.UnitVirtualPublicKey(unitName)
		if err != nil {
			res.Error = apiservererrors.ServerError(errors.Annotate(err, "failed to get unit host key"))
			return res
		}
	default:
		res.Error = apiservererrors.ServerError(errors.NotValidf("unsupported target: %v", info.Target()))
		return res
	}

	res.PublicKey = pubKey

	jumpServerPubKey, err := facade.backend.JumpServerVirtualPublicKey()
	if err != nil {
		res.Error = apiservererrors.ServerError(errors.Annotate(err, "failed to get controller jumpserver host key"))
		return res
	}

	res.JumpServerPublicKey = jumpServerPubKey

	return res
}

// getVirtualHostnameForEntity returns the virtual hostname for the given entity. It parses the tag string to
// evaluate if the entity is a machine or a unit. If the entity is a unit, it also takes an optional container
// name which is used to construct the virtual hostname.
func getVirtualHostnameForEntity(modelUUID string, tagString string, container *string) (string, error) {
	tag, err := names.ParseTag(tagString)
	if err != nil {
		return "", errors.Trace(err)
	}
	var info virtualhostname.Info
	switch tag.Kind() {
	case names.MachineTagKind:
		tag := tag.(names.MachineTag)
		info, err = virtualhostname.NewInfoMachineTarget(modelUUID, tag.Id())
		if err != nil {
			return "", errors.Trace(err)
		}
	case names.UnitTagKind:
		tag := tag.(names.UnitTag)
		if container != nil {
			info, err = virtualhostname.NewInfoContainerTarget(modelUUID, tag.Id(), *container)
			if err != nil {
				return "", errors.Trace(err)
			}
		} else {
			info, err = virtualhostname.NewInfoUnitTarget(modelUUID, tag.Id())
			if err != nil {
				return "", errors.Trace(err)
			}
		}
	default:
		return "", errors.Errorf("unsupported entity: %q", tagString)
	}
	return info.String(), nil
}
