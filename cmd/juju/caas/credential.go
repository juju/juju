// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/errors"
	"github.com/juju/utils"

	jujucloud "github.com/juju/juju/cloud"
)

const caasCrendentialLabelKeyName = "juju-caas-credential-uuid"

func ensureCredentialLabel(
	credentialName string,
	existingCredential, newcredential jujucloud.Credential,
) (_ *jujucloud.Credential, err error) {

	newAttr := newcredential.Attributes()
	if newAttr == nil {
		return nil, errors.NotValidf("empty credential %q", credentialName)
	}

	var credUUID string
	if existingCredential.Attributes() != nil {
		credUUID = existingCredential.Attributes()[caasCrendentialLabelKeyName]
	}
	if credUUID == "" {
		uuid, err := utils.NewUUID()
		if err != nil {
			return nil, errors.Trace(err)
		}
		credUUID = uuid.String()
	}

	newAttr[caasCrendentialLabelKeyName] = credUUID

	cred := jujucloud.NewNamedCredential(
		credentialName, newcredential.AuthType(), newAttr, newcredential.Revoked,
	)
	return &cred, nil
}

func getExistingCredential() (jujucloud.Credential, error) {
	// TODO: return existing  local/remote credential!!!!!
	// how to handle conflict between local and remote cred??????????
	return jujucloud.Credential{}, nil
}
