// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	"github.com/juju/loggo"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

// Logger for the MAAS provider.
var logger = loggo.GetLogger("juju.provider.maas")

type maasEnvironProvider struct {
	environProviderCredentials
}

var _ environs.EnvironProvider = (*maasEnvironProvider)(nil)

var providerInstance maasEnvironProvider

func (maasEnvironProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	logger.Debugf("opening model %q.", args.Config.Name())
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	env, err := NewEnviron(args.Cloud, args.Config)
	if err != nil {
		return nil, err
	}
	return env, nil
}

var errAgentNameAlreadySet = errors.New(
	"maas-agent-name is already set; this should not be set by hand")

// PrepareConfig is specified in the EnvironProvider interface.
func (p maasEnvironProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	var attrs map[string]interface{}
	if _, ok := args.Config.StorageDefaultBlockSource(); !ok {
		attrs = map[string]interface{}{
			config.StorageDefaultBlockSourceKey: maasStorageProviderType,
		}
	}
	if len(attrs) == 0 {
		return args.Config, nil
	}
	return args.Config.Apply(attrs)
}

func verifyCredentials(env *maasEnviron) error {
	// Verify we can connect to the server and authenticate.
	if env.usingMAAS2() {
		// The maas2 controller verifies credentials at creation time.
		return nil
	}
	_, err := env.getMAASClient().GetSubObject("maas").CallGet("get_config", nil)
	if err, ok := errors.Cause(err).(gomaasapi.ServerError); ok && err.StatusCode == http.StatusUnauthorized {
		logger.Debugf("authentication failed: %v", err)
		return errors.New(`authentication failed.

Please ensure the credentials are correct.`)
	}
	return nil
}

// DetectRegions is specified in the environs.CloudRegionDetector interface.
func (p maasEnvironProvider) DetectRegions() ([]cloud.Region, error) {
	return nil, errors.NotFoundf("regions")
}

func validateCloudSpec(spec environs.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if _, err := parseCloudEndpoint(spec.Endpoint); err != nil {
		return errors.Annotate(err, "validating endpoint")
	}
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	if authType := spec.Credential.AuthType(); authType != cloud.OAuth1AuthType {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	if _, err := parseOAuthToken(*spec.Credential); err != nil {
		return errors.Annotate(err, "validating MAAS OAuth token")
	}
	return nil
}

func parseCloudEndpoint(endpoint string) (server string, _ error) {
	// For MAAS, the cloud endpoint may be either a full URL
	// for the MAAS server, or just the IP/host.
	if endpoint == "" {
		return "", errors.New("MAAS server not specified")
	}
	server = endpoint
	if url, err := url.Parse(server); err != nil || url.Scheme == "" {
		server = fmt.Sprintf("http://%s/MAAS", endpoint)
		if _, err := url.Parse(server); err != nil {
			return "", errors.NotValidf("endpoint %q", endpoint)
		}
	}
	return server, nil
}
