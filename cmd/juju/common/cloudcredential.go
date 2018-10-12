// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
)

// ErrMultipleDetectedCredentials is the error returned by
// GetOrDetectCredential when multiple credentials are
// detected, meaning Juju cannot choose one automatically.
var ErrMultipleDetectedCredentials = errors.New("multiple detected credentials")

//go:generate mockgen -package common -destination credentialstore_mock_test.go github.com/juju/juju/jujuclient CredentialStore

// RegisterCredentials will attempt to register any credentials that a provider
// has to offer.
func RegisterCredentials(
	ctx *cmd.Context,
	store jujuclient.CredentialStore,
	provider environs.EnvironProvider,
) error {
	credentials, err := modelcmd.RegisterCredentials(provider)
	switch {
	case errors.IsNotFound(err):
		return nil
	case err != nil:
		return errors.Trace(err)
	case credentials == nil:
		return nil
	}

	ctx.Verbosef("updating credential store")

	for name, credential := range credentials {
		if err := store.UpdateCredential(name, *credential); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// GetOrDetectCredential returns a credential to use for given cloud. This
// function first calls modelcmd.GetCredentials, and returns its results if it
// finds credentials. If modelcmd.GetCredentials cannot find a credential, and a
// credential has not been specified by name, then this function will attempt to
// detect credentials from the environment.
//
// If multiple credentials are found in the client store, then
// modelcmd.ErrMultipleCredentials is returned. If multiple credentials are
// detected by the provider, then ErrMultipleDetectedCredentials is returned.
func GetOrDetectCredential(
	ctx *cmd.Context,
	store jujuclient.CredentialGetter,
	provider environs.EnvironProvider,
	args modelcmd.GetCredentialsParams,
) (_ *jujucloud.Credential, chosenCredentialName, regionName string, isDetected bool, _ error) {
	fail := func(err error) (*jujucloud.Credential, string, string, bool, error) {
		return nil, "", "", false, err
	}
	credential, chosenCredentialName, regionName, err := modelcmd.GetCredentials(ctx, store, args)
	if !errors.IsNotFound(err) || args.CredentialName != "" {
		return credential, chosenCredentialName, regionName, false, err
	}

	// No credential was explicitly specified, and no credential was found
	// in the credential store; have the provider detect credentials from
	// the environment.
	ctx.Verbosef("no credentials found, checking environment")

	detected, err := modelcmd.DetectCredential(args.Cloud.Name, provider)
	if errors.Cause(err) == modelcmd.ErrMultipleCredentials {
		return fail(ErrMultipleDetectedCredentials)
	} else if err != nil {
		return fail(errors.Trace(err))
	}

	// We have one credential so extract it from the map.
	var oneCredential jujucloud.Credential
	for chosenCredentialName, oneCredential = range detected.AuthCredentials {
	}
	regionName = args.CloudRegion
	if regionName == "" {
		regionName = detected.DefaultRegion
	}

	// Finalize the credential against the cloud/region.
	region, err := ChooseCloudRegion(args.Cloud, regionName)
	if err != nil {
		return fail(err)
	}

	credential, err = modelcmd.FinalizeFileContent(&oneCredential, provider)
	if err != nil {
		return nil, "", "", false, modelcmd.AnnotateWithFinalizationError(err, chosenCredentialName, args.Cloud.Name)
	}

	credential, err = provider.FinalizeCredential(
		ctx, environs.FinalizeCredentialParams{
			Credential:            *credential,
			CloudEndpoint:         region.Endpoint,
			CloudStorageEndpoint:  region.StorageEndpoint,
			CloudIdentityEndpoint: region.IdentityEndpoint,
		},
	)
	if err != nil {
		return fail(errors.Trace(err))
	}
	return credential, chosenCredentialName, regionName, true, nil
}

// ResolveCloudCredentialTag takes a string which is of either the format
// "<credential>" or "<user>/<credential>". If the string does not include
// a user, then the supplied user tag is implied.
func ResolveCloudCredentialTag(user names.UserTag, cloud names.CloudTag, credentialName string) (names.CloudCredentialTag, error) {
	if i := strings.IndexRune(credentialName, '/'); i == -1 {
		credentialName = fmt.Sprintf("%s/%s", user.Id(), credentialName)
	}
	s := fmt.Sprintf("%s/%s", cloud.Id(), credentialName)
	if !names.IsValidCloudCredential(s) {
		return names.CloudCredentialTag{}, errors.NotValidf("cloud credential name %q", s)
	}
	return names.NewCloudCredentialTag(s), nil
}

// OutputUpdateCredentialModelResult prints detailed results of UpdateCredentialsCheckModels.
func OutputUpdateCredentialModelResult(ctx *cmd.Context, models []params.UpdateCredentialModelResult, showValid bool) {
	var valid []string
	invalid := map[string][]error{}
	for _, m := range models {
		if len(m.Errors) == 0 {
			valid = append(valid, m.ModelName)
			continue
		} else {
			var mError []error
			for _, anErr := range m.Errors {
				mError = append(mError, errors.Trace(anErr.Error))
			}
			invalid[m.ModelName] = mError
		}
	}

	if showValid && len(valid) > 0 {
		ctx.Infof("Credential valid for:")
		for _, v := range valid {
			ctx.Infof("  %v", v)
		}
	}
	if len(invalid) > 0 {
		// ensure we sort the valid, invalid slices so that the output is consistent
		i := 0
		names := make([]string, len(invalid))
		for k := range invalid {
			names[i] = k
			i++
		}
		sort.Strings(names)

		ctx.Infof("Credential invalid for:")
		for _, v := range names {
			ctx.Infof("  %v:", v)
			for _, e := range invalid[v] {
				ctx.Infof("    %v", e)
			}
		}
	}
}

//go:generate mockgen -package common -destination cloudprovider_mock_test.go github.com/juju/juju/cmd/juju/common TestCloudProvider

// TestCloudProvider is used for testing.
type TestCloudProvider interface {
	environs.EnvironProvider
	environs.ProviderCredentialsRegister
}
