// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"io/ioutil"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/utils"
)

// GetCredentials returns a curated set of credential values for a given cloud.
// The credential key values are read from the credentials store and the provider
// finalises the values to resolve things like json files.
// If region is not specified, the default credential region is used.
func GetCredentials(
	ctx *cmd.Context, store jujuclient.CredentialGetter, region, credentialName, cloudName, cloudType string,
) (_ *jujucloud.Credential, regionName string, _ error) {

	credential, credentialName, defaultRegion, err := credentialByName(
		store, cloudName, credentialName,
	)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	regionName = region
	if regionName == "" {
		regionName = defaultRegion
	}

	readFile := func(f string) ([]byte, error) {
		f, err := utils.NormalizePath(f)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ioutil.ReadFile(ctx.AbsPath(f))
	}

	// Finalize credential against schemas supported by the provider.
	provider, err := environs.Provider(cloudType)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	credential, err = jujucloud.FinalizeCredential(
		*credential, provider.CredentialSchemas(), readFile,
	)
	if err != nil {
		return nil, "", errors.Annotatef(
			err, "validating %q credential for cloud %q",
			credentialName, cloudName,
		)
	}
	return credential, regionName, nil
}

// credentialByName returns the credential and default region to use for the
// specified cloud, optionally specifying a credential name. If no credential
// name is specified, then use the default credential for the cloud if one has
// been specified. The credential name is returned also, in case the default
// credential is used. If there is only one credential, it is implicitly the
// default.
//
// If there exists no matching credentials, an error satisfying
// errors.IsNotFound will be returned.
func credentialByName(
	store jujuclient.CredentialGetter, cloudName, credentialName string,
) (_ *jujucloud.Credential, credentialNameUsed string, defaultRegion string, _ error) {

	cloudCredentials, err := store.CredentialForCloud(cloudName)
	if err != nil {
		return nil, "", "", errors.Annotate(err, "loading credentials")
	}
	if credentialName == "" {
		// No credential specified, so use the default for the cloud.
		credentialName = cloudCredentials.DefaultCredential
		if credentialName == "" && len(cloudCredentials.AuthCredentials) == 1 {
			for credentialName = range cloudCredentials.AuthCredentials {
			}
		}
	}
	credential, ok := cloudCredentials.AuthCredentials[credentialName]
	if !ok {
		return nil, "", "", errors.NotFoundf(
			"%q credential for cloud %q", credentialName, cloudName,
		)
	}
	return &credential, credentialName, cloudCredentials.DefaultRegion, nil
}
