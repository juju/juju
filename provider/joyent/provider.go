// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"log"

	"github.com/joyent/gocommon/client"
	joyenterrors "github.com/joyent/gocommon/errors"
	"github.com/joyent/gosdc/cloudapi"
	"github.com/joyent/gosign/auth"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
)

var logger = loggo.GetLogger("juju.provider.joyent")

// TODO(ericsnow) gologWriter can go away once loggo.Logger has a GoLogger() method.

type gologWriter struct {
	loggo.Logger
	level loggo.Level
}

func newGoLogger() *log.Logger {
	return log.New(&gologWriter{logger, loggo.TRACE}, "", 0)
}

func (w *gologWriter) Write(p []byte) (n int, err error) {
	w.Logf(w.level, string(p))
	return len(p), nil
}

type joyentProvider struct {
	environProviderCredentials
}

var providerInstance = joyentProvider{}
var _ environs.EnvironProvider = providerInstance

var _ simplestreams.HasRegion = (*joyentEnviron)(nil)

// PrepareConfig is part of the EnvironProvider interface.
func (p joyentProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	return args.Config, nil
}

const unauthorisedMessage = `
Please ensure the SSH access key you have specified is correct.
You can create or import an SSH key via the "Account Summary"
page in the Joyent console.`

// verifyCredentials issues a cheap, non-modifying request to Joyent to
// verify the configured credentials. If verification fails, a user-friendly
// error will be returned, and the original error will be logged at debug
// level.
var verifyCredentials = func(e *joyentEnviron) error {
	creds, err := credentials(e.cloud)
	if err != nil {
		return err
	}
	httpClient := client.NewClient(e.cloud.Endpoint, cloudapi.DefaultAPIVersion, creds, nil)
	apiClient := cloudapi.New(httpClient)
	_, err = apiClient.CountMachines()
	if err != nil {
		logger.Debugf("joyent request failed: %v", err)
		if joyenterrors.IsInvalidCredentials(err) || joyenterrors.IsNotAuthorized(err) {
			return errors.New("authentication failed.\n" + unauthorisedMessage)
		}
		return err
	}
	return nil
}

func credentials(cloud environs.CloudSpec) (*auth.Credentials, error) {
	credAttrs := cloud.Credential.Attributes()
	sdcUser := credAttrs[credAttrSDCUser]
	sdcKeyID := credAttrs[credAttrSDCKeyID]
	privateKey := credAttrs[credAttrPrivateKey]
	algorithm := credAttrs[credAttrAlgorithm]
	if algorithm == "" {
		algorithm = algorithmDefault
	}

	authentication, err := auth.NewAuth(sdcUser, privateKey, algorithm)
	if err != nil {
		return nil, errors.Errorf("cannot create credentials: %v", err)
	}
	return &auth.Credentials{
		UserAuthentication: authentication,
		SdcKeyId:           sdcKeyID,
		SdcEndpoint:        auth.Endpoint{URL: cloud.Endpoint},
	}, nil
}

func (joyentProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	env, err := newEnviron(args.Cloud, args.Config)
	if err != nil {
		return nil, err
	}
	return env, nil
}

func (joyentProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	newEcfg, err := validateConfig(cfg, old)
	if err != nil {
		return nil, errors.Errorf("invalid Joyent provider config: %v", err)
	}
	return cfg.Apply(newEcfg.attrs)
}

func GetProviderInstance() environs.EnvironProvider {
	return providerInstance
}

// MetadataLookupParams returns parameters which are used to query image metadata to
// find matching image information.
func (p joyentProvider) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		return nil, errors.Errorf("region must be specified")
	}
	return &simplestreams.MetadataLookupParams{
		Region: region,
	}, nil
}

func (p joyentProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

func validateCloudSpec(spec environs.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	if authType := spec.Credential.AuthType(); authType != cloud.UserPassAuthType {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}
