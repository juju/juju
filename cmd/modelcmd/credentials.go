// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"io/ioutil"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
)

var (
	// ErrMultipleCredentials is the error returned by DetectCredential
	// if more than one credential is detected.
	ErrMultipleCredentials = errors.New("more than one credential detected")
)

// GetCredentials returns a curated set of credential values for a given cloud.
// The credential key values are read from the credentials store and the provider
// finalises the values to resolve things like json files.
// If region is not specified, the default credential region is used.
func GetCredentials(
	store jujuclient.CredentialGetter, region, credentialName, cloudName, cloudType string,
) (_ *cloud.Credential, chosenCredentialName, regionName string, _ error) {

	credential, credentialName, defaultRegion, err := credentialByName(
		store, cloudName, credentialName,
	)
	if err != nil {
		return nil, "", "", errors.Trace(err)
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
		return ioutil.ReadFile(f)
	}

	// Finalize credential against schemas supported by the provider.
	provider, err := environs.Provider(cloudType)
	if err != nil {
		return nil, "", "", errors.Trace(err)
	}

	credential, err = cloud.FinalizeCredential(
		*credential, provider.CredentialSchemas(), readFile,
	)
	if err != nil {
		return nil, "", "", errors.Annotatef(
			err, "validating %q credential for cloud %q",
			credentialName, cloudName,
		)
	}
	return credential, credentialName, regionName, nil
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
) (_ *cloud.Credential, credentialNameUsed string, defaultRegion string, _ error) {

	cloudCredentials, err := store.CredentialForCloud(cloudName)
	if err != nil {
		return nil, "", "", errors.Annotate(err, "loading credentials")
	}
	if credentialName == "" {
		credentialName = cloudCredentials.DefaultCredential
		if credentialName == "" {
			// No credential specified, but there's more than one.
			if len(cloudCredentials.AuthCredentials) > 1 {
				return nil, "", "", ErrMultipleCredentials
			}
			// No credential specified, so use the default for the cloud.
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

// DetectCredential detects credentials for the specified cloud type, and, if
// exactly one is detected, returns it.
//
// If no credentials are detected, an error satisfying errors.IsNotFound will
// be returned. If more than one credential is detected, ErrMultipleCredentials
// will be returned.
func DetectCredential(cloudName, cloudType string) (*cloud.CloudCredential, error) {
	provider, err := environs.Provider(cloudType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	detected, err := provider.DetectCredentials()
	if err != nil {
		return nil, errors.Annotatef(
			err, "detecting credentials for %q cloud provider", cloudName,
		)
	}
	logger.Tracef("provider detected credentials: %v", detected)
	if len(detected.AuthCredentials) == 0 {
		return nil, errors.NotFoundf("credentials for cloud %q", cloudName)
	}
	if len(detected.AuthCredentials) > 1 {
		return nil, ErrMultipleCredentials
	}
	return detected, nil
}
