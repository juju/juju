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
		store, args.Cloud.Name, args.CredentialName,
	)
	if err != nil {
		return nil, "", "", errors.Trace(err)
	}
	regionName = args.CloudRegion
	if regionName == "" {
		regionName = defaultRegion
	}
	credential, err = VerifyCredentials(ctx, &args.Cloud, credential, credentialName, regionName)
	if err != nil {
		return nil, "", "", errors.Trace(err)
	}
	return credential, credentialName, regionName, nil
}

// VerifyCredentials ensures that a given credential complies with the provider/cloud understanding of what
// its valid credential looks like.
func VerifyCredentials(ctx *cmd.Context, aCloud *cloud.Cloud, credential *cloud.Credential, credentialName string, regionName string) (_ *cloud.Credential, _ error) {
	cloudEndpoint := aCloud.Endpoint
	cloudStorageEndpoint := aCloud.StorageEndpoint
	cloudIdentityEndpoint := aCloud.IdentityEndpoint
	if regionName != "" {
		region, err := cloud.RegionByName(aCloud.Regions, regionName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		cloudEndpoint = region.Endpoint
		cloudStorageEndpoint = region.StorageEndpoint
		cloudIdentityEndpoint = region.IdentityEndpoint
	}

	// Finalize credential against schemas supported by the provider.
	provider, err := environs.Provider(aCloud.Type)
	if err != nil {
		return nil, errors.Trace(err)
	}

	credential, err = FinalizeFileContent(credential, provider)
	if err != nil {
		return nil, AnnotateWithFinalizationError(err, credentialName, aCloud.Name)
	}

	credential, err = provider.FinalizeCredential(
		ctx, environs.FinalizeCredentialParams{
			Credential:            *credential,
			CloudEndpoint:         cloudEndpoint,
			CloudStorageEndpoint:  cloudStorageEndpoint,
			CloudIdentityEndpoint: cloudIdentityEndpoint,
		},
	)
	if err != nil {
		return nil, AnnotateWithFinalizationError(err, credentialName, aCloud.Name)
	}

	return credential, nil
}

// FinalizeFileContent replaces the path content of cloud credentials "file" attribute with its content.
func FinalizeFileContent(credential *cloud.Credential, provider environs.EnvironProvider) (*cloud.Credential, error) {
	readFile := func(f string) ([]byte, error) {
		f, err := utils.NormalizePath(f)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ioutil.ReadFile(f)
	}

	var err error
	credential, err = cloud.FinalizeCredential(
		*credential, provider.CredentialSchemas(), readFile,
	)
	if err != nil {
		return nil, err
	}

	return credential, nil
}

func AnnotateWithFinalizationError(err error, credentialName, cloudName string) error {
	return errors.Annotatef(
		err, "finalizing %q credential for cloud %q",
		credentialName, cloudName,
	)
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
func DetectCredential(cloudName string, provider environs.EnvironProvider) (*cloud.CloudCredential, error) {
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

// RegisterCredentialsParams contains parameters for the RegisterCredentials function.
type RegisterCredentialsParams struct {
	// Cloud is the cloud definition.
	Cloud cloud.Cloud
}

// RegisterCredentials returns any credentials that need to be registered for
// a provider.
func RegisterCredentials(provider environs.EnvironProvider, args RegisterCredentialsParams) (map[string]*cloud.CloudCredential, error) {
	if register, ok := provider.(environs.ProviderCredentialsRegister); ok {
		found, err := register.RegisterCredentials(args.Cloud)
		if err != nil {
			return nil, errors.Annotatef(
				err, "registering credentials for provider",
			)
		}
		logger.Tracef("provider registered credentials: %v", found)
		if len(found) == 0 {
			return nil, errors.NotFoundf("credentials for provider")
		}
		return found, errors.Trace(err)
	}
	return nil, nil
}

//go:generate go run github.com/golang/mock/mockgen -package modelcmd -destination cloudprovider_mock_test.go github.com/juju/juju/cmd/modelcmd TestCloudProvider

// TestCloudProvider is used for testing.
type TestCloudProvider interface {
	environs.EnvironProvider
	environs.ProviderCredentialsRegister
}
