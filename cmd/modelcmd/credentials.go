// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"io/ioutil"

	"github.com/juju/cmd"
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

// GetCredentialsParams contains parameters for the GetCredentials function.
type GetCredentialsParams struct {
	// Cloud is the cloud definition.
	Cloud cloud.Cloud

	// CloudName is the name of the cloud for which credentials are being
	// obtained.
	CloudName string

	// CloudRegion is the name of the region that the user has specified.
	// If this is empty, then GetCredentials will determine the default
	// region, and return that. The default region is the one set by the
	// user in credentials.yaml, or if there is none set, the first region
	// in the cloud's list.
	CloudRegion string

	// CredentialName is the name of the credential to get.
	CredentialName string
}

// GetCredentials returns a curated set of credential values for a given cloud.
// The credential key values are read from the credentials store and the provider
// finalises the values to resolve things like json files.
// If region is not specified, the default credential region is used.
func GetCredentials(
	ctx *cmd.Context,
	store jujuclient.CredentialGetter,
	args GetCredentialsParams,
) (_ *cloud.Credential, chosenCredentialName, regionName string, _ error) {

	credential, credentialName, defaultRegion, err := credentialByName(
		store, args.CloudName, args.CredentialName,
	)
	if err != nil {
		return nil, "", "", errors.Trace(err)
	}

	regionName = args.CloudRegion
	if regionName == "" {
		regionName = defaultRegion
		if regionName == "" && len(args.Cloud.Regions) > 0 {
			// No region was specified, use the first region
			// in the list.
			regionName = args.Cloud.Regions[0].Name
		}
	}

	cloudEndpoint := args.Cloud.Endpoint
	cloudIdentityEndpoint := args.Cloud.IdentityEndpoint
	if regionName != "" {
		region, err := cloud.RegionByName(args.Cloud.Regions, regionName)
		if err != nil {
			return nil, "", "", errors.Trace(err)
		}
		cloudEndpoint = region.Endpoint
		cloudIdentityEndpoint = region.IdentityEndpoint
	}

	readFile := func(f string) ([]byte, error) {
		f, err := utils.NormalizePath(f)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ioutil.ReadFile(f)
	}

	// Finalize credential against schemas supported by the provider.
	provider, err := environs.Provider(args.Cloud.Type)
	if err != nil {
		return nil, "", "", errors.Trace(err)
	}

	credential, err = cloud.FinalizeCredential(
		*credential, provider.CredentialSchemas(), readFile,
	)
	if err != nil {
		return nil, "", "", errors.Annotatef(
			err, "finalizing %q credential for cloud %q",
			credentialName, args.CloudName,
		)
	}

	credential, err = provider.FinalizeCredential(
		ctx, environs.FinalizeCredentialParams{
			Credential:            *credential,
			CloudEndpoint:         cloudEndpoint,
			CloudIdentityEndpoint: cloudIdentityEndpoint,
		},
	)
	if err != nil {
		return nil, "", "", errors.Annotatef(
			err, "finalizing %q credential for cloud %q",
			credentialName, args.CloudName,
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
