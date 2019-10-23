// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"encoding/hex"
	"math/rand"

	"github.com/juju/errors"

	jujucloud "github.com/juju/juju/cloud"
)

// TODO: a better id name?????
const caasCrendentialLabelKeyName = "rbac-id"

func ensureCredentialUID(
	credentialName, credentialUID string,
	credential jujucloud.Credential,
) (cred jujucloud.Credential, _ error) {

	newAttr := credential.Attributes()
	if newAttr == nil {
		return cred, errors.NotValidf("empty credential %q", credentialName)
	}
	newAttr[caasCrendentialLabelKeyName] = credentialUID
	return jujucloud.NewNamedCredential(
		credentialName, credential.AuthType(), newAttr, credential.Revoked,
	), nil
}

type credentialGetter interface {
	// CredentialForCloud gets credentials for the named cloud.
	CredentialForCloud(string) (*jujucloud.CloudCredential, error)
}

func getExistingCredential(store credentialGetter, cloudName, credentialName string) (credential jujucloud.Credential, err error) {
	cloudCredential, err := store.CredentialForCloud(cloudName)
	if err != nil {
		return credential, errors.Trace(err)
	}
	var ok bool
	if credential, ok = cloudCredential.AuthCredentials[credentialName]; !ok {
		return credential, errors.NotFoundf("credential %q for cloud %q", credentialName, cloudName)
	}
	return credential, nil
}

func decideCredentialUID(store credentialGetter, cloudName, credentialName string) (string, error) {
	var credUID string
	existingCredential, err := getExistingCredential(store, cloudName, credentialName)
	if err != nil && !errors.IsNotFound(err) {
		return "", errors.Trace(err)
	}
	if err == nil && existingCredential.Attributes() != nil {
		credUID = existingCredential.Attributes()[caasCrendentialLabelKeyName]
	}

	if credUID == "" {
		b := make([]byte, 4)
		if _, err := rand.Read(b); err != nil {
			return credUID, errors.Trace(err)
		}
		credUID = hex.EncodeToString(b)
	}
	return credUID, nil
}
