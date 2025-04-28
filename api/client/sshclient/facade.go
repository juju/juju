// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/rpc/params"
)

// NewFacade returns a new Facade based on an existing API connection.
func NewFacade(callCloser base.APICallCloser) *Facade {
	clientFacade, caller := base.NewClientFacade(callCloser, "SSHClient")
	return &Facade{
		ClientFacade: clientFacade,
		caller:       caller,
	}
}

type Facade struct {
	base.ClientFacade
	caller base.FacadeCaller
}

// PublicAddress returns the public address for the SSH target
// provided. The target may be provided as a machine ID or unit name.
func (facade *Facade) PublicAddress(target string) (string, error) {
	addr, err := facade.addressCall("PublicAddress", target)
	return addr, errors.Trace(err)
}

// PrivateAddress returns the private address for the SSH target
// provided. The target may be provided as a machine ID or unit name.
func (facade *Facade) PrivateAddress(target string) (string, error) {
	addr, err := facade.addressCall("PrivateAddress", target)
	return addr, errors.Trace(err)
}

// AllAddresses returns all addresses for the SSH target provided. The target
// may be provided as a machine ID or unit name.
func (facade *Facade) AllAddresses(target string) ([]string, error) {
	entities, err := targetToEntities(target)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var out params.SSHAddressesResults
	err = facade.caller.FacadeCall("AllAddresses", entities, &out)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(out.Results) != 1 {
		return nil, countError(len(out.Results))
	}
	if err := out.Results[0].Error; err != nil {
		return nil, errors.Trace(err)
	}
	return out.Results[0].Addresses, nil
}

// VirtualHostname returns the virtual hostname for the SSH target provided.
func (facade *Facade) VirtualHostname(target string, container *string) (string, error) {
	tag, err := targetToTag(target)
	if err != nil {
		return "", errors.Trace(err)
	}
	in := params.VirtualHostnameTargetArg{
		Tag:       tag.String(),
		Container: container,
	}
	var out params.SSHAddressResult
	err = facade.caller.FacadeCall("VirtualHostname", in, &out)
	if err != nil {
		return "", errors.Trace(err)
	}
	if err := out.Error; err != nil {
		return "", errors.Trace(err)
	}
	return out.Address, nil
}

func (facade *Facade) addressCall(callName, target string) (string, error) {
	entities, err := targetToEntities(target)
	if err != nil {
		return "", errors.Trace(err)
	}
	var out params.SSHAddressResults
	err = facade.caller.FacadeCall(callName, entities, &out)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(out.Results) != 1 {
		return "", countError(len(out.Results))
	}
	if err := out.Results[0].Error; err != nil {
		return "", errors.Trace(err)
	}
	return out.Results[0].Address, nil
}

// PublicKeys returns the SSH public host keys for the SSH target
// provided. The target may be provided as a machine ID or unit name.
func (facade *Facade) PublicKeys(target string) ([]string, error) {
	entities, err := targetToEntities(target)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var out params.SSHPublicKeysResults
	err = facade.caller.FacadeCall("PublicKeys", entities, &out)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(out.Results) != 1 {
		return nil, countError(len(out.Results))
	}
	if err := out.Results[0].Error; err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}
	return out.Results[0].PublicKeys, nil
}

// Proxy returns whether SSH connections should be proxied through the
// controller hosts for the associated model.
func (facade *Facade) Proxy() (bool, error) {
	var out params.SSHProxyResult
	err := facade.caller.FacadeCall("Proxy", nil, &out)
	if err != nil {
		return false, errors.Trace(err)
	}
	return out.UseProxy, nil
}

func targetToEntities(target string) (params.Entities, error) {
	tag, err := targetToTag(target)
	if err != nil {
		return params.Entities{}, errors.Trace(err)
	}
	return params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}, nil
}

func targetToTag(target string) (names.Tag, error) {
	switch {
	case names.IsValidMachine(target):
		return names.NewMachineTag(target), nil
	case names.IsValidUnit(target):
		return names.NewUnitTag(target), nil
	default:
		return nil, errors.NotValidf("target %q", target)
	}
}

// countError complains about malformed results.
func countError(count int) error {
	return errors.Errorf("expected 1 result, got %d", count)
}

// ModelCredentialForSSH returns a cloud spec for ssh purpose.
// This facade call is only used for k8s model.
func (facade *Facade) ModelCredentialForSSH() (cloudspec.CloudSpec, error) {
	var result params.CloudSpecResult

	err := facade.caller.FacadeCall("ModelCredentialForSSH", nil, &result)
	if err != nil {
		return cloudspec.CloudSpec{}, err
	}
	if result.Error != nil {
		err := apiservererrors.RestoreError(result.Error)
		return cloudspec.CloudSpec{}, err
	}
	pSpec := result.Result
	if pSpec == nil {
		return cloudspec.CloudSpec{}, errors.NotValidf("empty value")
	}
	var credential *cloud.Credential
	if pSpec.Credential != nil {
		credentialValue := cloud.NewCredential(
			cloud.AuthType(pSpec.Credential.AuthType),
			pSpec.Credential.Attributes,
		)
		credential = &credentialValue
	}
	spec := cloudspec.CloudSpec{
		Type:              pSpec.Type,
		Name:              pSpec.Name,
		Region:            pSpec.Region,
		Endpoint:          pSpec.Endpoint,
		IdentityEndpoint:  pSpec.IdentityEndpoint,
		StorageEndpoint:   pSpec.StorageEndpoint,
		CACertificates:    pSpec.CACertificates,
		SkipTLSVerify:     pSpec.SkipTLSVerify,
		Credential:        credential,
		IsControllerCloud: pSpec.IsControllerCloud,
	}
	if err := spec.Validate(); err != nil {
		return cloudspec.CloudSpec{}, errors.Annotatef(err, "cannot validate CloudSpec %q", spec.Name)
	}
	return spec, nil
}

// PublicHostKeyForTarget returns the public SSH host key for the target virtualhostname and
// the controller's jump server public key.
func (facade *Facade) PublicHostKeyForTarget(target string) (params.PublicSSHHostKeyResult, error) {
	var arg params.SSHVirtualHostKeyRequestArg
	arg.Hostname = target
	var out params.PublicSSHHostKeyResult
	err := facade.caller.FacadeCall("PublicHostKeyForTarget", arg, &out)
	if err != nil {
		return params.PublicSSHHostKeyResult{}, errors.Trace(err)
	}
	if err := out.Error; err != nil {
		return params.PublicSSHHostKeyResult{}, errors.Trace(apiservererrors.RestoreError(err))
	}
	return out, nil
}
