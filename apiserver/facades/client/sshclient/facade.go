// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"context"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/virtualhostname"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/rpc/params"
)

// Facade implements the API required by the sshclient worker.
type Facade struct {
	authorizer facade.Authorizer

	applicationService   ApplicationService
	machineService       MachineService
	networkService       NetworkService
	modelConfigService   ModelConfigService
	modelProviderService ModelProviderService
	modelTag             names.ModelTag
	controllerTag        names.ControllerTag
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
	controllerTag names.ControllerTag,
	modelTag names.ModelTag,
	applicationService ApplicationService,
	machineService MachineService,
	networkService NetworkService,
	modelConfigService ModelConfigService,
	modelProviderService ModelProviderService,
	auth facade.Authorizer,
) (*Facade, error) {
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &Facade{
		applicationService:   applicationService,
		modelConfigService:   modelConfigService,
		modelProviderService: modelProviderService,
		machineService:       machineService,
		networkService:       networkService,
		controllerTag:        controllerTag,
		modelTag:             modelTag,
		authorizer:           auth,
	}, nil
}

func (facade *Facade) checkIsModelAdmin(ctx context.Context) error {
	err := facade.authorizer.HasPermission(ctx, permission.SuperuserAccess, facade.controllerTag)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return facade.authorizer.HasPermission(ctx, permission.AdminAccess, facade.modelTag)
}

// VirtualHostname is not implemented in v4.
func (f *FacadeV4) VirtualHostname(_, _, _ struct{}) {}

// VirtualHostname returns the virtual hostname for the given entity.
func (facade *Facade) VirtualHostname(ctx context.Context, arg params.VirtualHostnameTargetArg) (params.SSHAddressResult, error) {
	if err := facade.checkIsModelAdmin(ctx); err != nil {
		return params.SSHAddressResult{}, errors.Trace(err)
	}
	modelUUID := facade.modelTag.Id()
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
func (facade *Facade) PublicAddress(ctx context.Context, args params.Entities) (params.SSHAddressResults, error) {
	if err := facade.checkIsModelAdmin(ctx); err != nil {
		return params.SSHAddressResults{}, errors.Trace(err)
	}

	getter := func(ctx context.Context, machineUUID machine.UUID) (network.SpaceAddress, error) {
		return facade.networkService.GetMachinePublicAddress(ctx, machineUUID)
	}
	return facade.getAddressPerEntity(ctx, args, getter)
}

// PrivateAddress reports the preferred private network address for one or
// more entities. Machines and units are supported.
func (facade *Facade) PrivateAddress(ctx context.Context, args params.Entities) (params.SSHAddressResults, error) {
	if err := facade.checkIsModelAdmin(ctx); err != nil {
		return params.SSHAddressResults{}, errors.Trace(err)
	}

	getter := func(ctx context.Context, machineUUID machine.UUID) (network.SpaceAddress, error) {
		return facade.networkService.GetMachinePrivateAddress(ctx, machineUUID)
	}
	return facade.getAddressPerEntity(ctx, args, getter)
}

// AllAddresses reports all addresses that might have SSH listening for each
// entity in args. The result is sorted with public addresses first.
// Machines and units are supported as entity types.
func (facade *Facade) AllAddresses(ctx context.Context, args params.Entities) (params.SSHAddressesResults, error) {
	if err := facade.checkIsModelAdmin(ctx); err != nil {
		return params.SSHAddressesResults{}, errors.Trace(err)
	}

	getter := func(ctx context.Context, machineUUID machine.UUID) ([]network.SpaceAddress, error) {
		devicesAddresses, err := facade.networkService.GetMachineAddresses(ctx, machineUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}

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

	return facade.getAllEntityAddresses(ctx, args, getter)
}

func (facade *Facade) getAllEntityAddresses(ctx context.Context, args params.Entities,
	getter func(context.Context, machine.UUID) ([]network.SpaceAddress, error)) (
	params.SSHAddressesResults, error,
) {
	out := params.SSHAddressesResults{
		Results: make([]params.SSHAddressesResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		machineUUID, err := facade.getMachineForEntity(ctx, entity.Tag)
		if err != nil {
			out.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		addresses, err := getter(ctx, machineUUID)
		if err != nil {
			out.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		out.Results[i].Addresses = make([]string, len(addresses))
		for j := range addresses {
			out.Results[i].Addresses[j] = addresses[j].Value
		}
	}
	return out, nil
}

func (facade *Facade) getAddressPerEntity(ctx context.Context, args params.Entities,
	addressGetter func(context.Context, machine.UUID) (network.SpaceAddress, error)) (
	params.SSHAddressResults, error,
) {
	out := params.SSHAddressResults{
		Results: make([]params.SSHAddressResult, len(args.Entities)),
	}

	getter := func(ctx context.Context, m machine.UUID) ([]network.SpaceAddress, error) {
		address, err := addressGetter(ctx, m)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return []network.SpaceAddress{address}, nil
	}

	fullResults, err := facade.getAllEntityAddresses(ctx, args, getter)
	if err != nil {
		return params.SSHAddressResults{}, errors.Trace(err)
	}

	for i, result := range fullResults.Results {
		if result.Error != nil {
			out.Results[i].Error = result.Error
			continue
		} else if len(result.Addresses) == 0 {
			continue
		}

		out.Results[i].Address = result.Addresses[0]
	}

	return out, nil
}

// PublicKeys returns the public SSH hosts for one or more
// entities. Machines and units are supported.
func (facade *Facade) PublicKeys(ctx context.Context, args params.Entities) (params.SSHPublicKeysResults, error) {
	if err := facade.checkIsModelAdmin(ctx); err != nil {
		return params.SSHPublicKeysResults{}, errors.Trace(err)
	}

	out := params.SSHPublicKeysResults{
		Results: make([]params.SSHPublicKeysResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		machineUUID, err := facade.getMachineForEntity(ctx, entity.Tag)
		if err != nil {
			out.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		keys, err := facade.machineService.GetSSHHostKeys(ctx, machineUUID)
		if err != nil {
			out.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		out.Results[i].PublicKeys = keys
	}
	return out, nil
}

// Proxy returns whether SSH connections should be proxied through the
// controller hosts for the model associated with the API connection.
func (facade *Facade) Proxy(ctx context.Context) (params.SSHProxyResult, error) {
	if err := facade.checkIsModelAdmin(ctx); err != nil {
		return params.SSHProxyResult{}, errors.Trace(err)
	}
	config, err := facade.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return params.SSHProxyResult{}, err
	}
	return params.SSHProxyResult{UseProxy: config.ProxySSH()}, nil
}

// ModelCredentialForSSH returns a cloud spec for ssh purpose.
// This facade call is only used for k8s model.
func (facade *Facade) ModelCredentialForSSH(ctx context.Context) (params.CloudSpecResult, error) {
	var result params.CloudSpecResult

	if err := facade.checkIsModelAdmin(ctx); err != nil {
		return result, err
	}

	spec, err := facade.modelProviderService.GetCloudSpecForSSH(ctx)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	if spec.Credential == nil {
		result.Error = apiservererrors.ServerError(errors.NotValidf("cloud spec %q has empty credential", spec.Name))
		return result, nil
	}
	result.Result = common.CloudSpecToParams(spec)
	return result, nil
}

func (facade *Facade) getMachineForEntity(ctx context.Context, tagString string) (machine.UUID, error) {
	tag, err := names.ParseTag(tagString)
	if err != nil {
		return "", errors.Trace(err)
	}

	switch tag := tag.(type) {
	case names.MachineTag:
		machineName := machine.Name(tag.Id())
		machineUUID, err := facade.machineService.GetMachineUUID(ctx, machineName)
		return machineUUID, errors.Trace(err)

	case names.UnitTag:
		machineName, err := facade.applicationService.GetUnitMachineName(ctx, unit.Name(tag.Id()))
		if errors.Is(err, applicationerrors.UnitNotFound) {
			return "", errors.NotFoundf("unit %q", tag.Id())
		} else if err != nil {
			return "", errors.Trace(err)
		}
		machineUUID, err := facade.machineService.GetMachineUUID(ctx, machineName)
		return machineUUID, errors.Trace(err)

	default:
		return "", errors.Errorf("unsupported entity: %q", tagString)
	}
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
