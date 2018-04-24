// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/state"
)

// Backend is an interface for manipulating cloud credential.
type Backend interface {

	// CloudCredential returns the cloud credential for the given tag.
	CloudCredential(tag names.CloudCredentialTag) (state.Credential, error)

	// UpdateCloudCredential adds or updates a cloud credential with the given tag.
	UpdateCloudCredential(tag names.CloudCredentialTag, credential cloud.Credential) error
}

// ChangeCloudCredentialsValidity marks given cloud credentials as valid/invalid according
// to supplied validity indicators using given persistence interface.
func ChangeCloudCredentialsValidity(b Backend, creds params.ValidateCredentialArgs) ([]params.ErrorResult, error) {
	if len(creds.All) == 0 {
		return nil, nil
	}
	all := make([]params.ErrorResult, len(creds.All))
	for i, one := range creds.All {
		tag, err := names.ParseCloudCredentialTag(one.CredentialTag)
		if err != nil {
			all[i].Error = common.ServerError(err)
			continue
		}
		storedCredential, err := b.CloudCredential(tag)
		if err != nil {
			all[i].Error = common.ServerError(err)
			continue
		}
		cloudCredential := cloud.NewNamedCredential(
			storedCredential.Name,
			cloud.AuthType(storedCredential.AuthType),
			storedCredential.Attributes,
			storedCredential.Revoked,
		)

		cloudCredential.Invalid = !one.Valid
		cloudCredential.InvalidReason = one.Reason

		err = b.UpdateCloudCredential(tag, cloudCredential)
		if err != nil {
			all[i].Error = common.ServerError(err)
		}
	}
	return all, nil
}
