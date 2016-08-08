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
	env, err := NewEnviron(args.Config)
	if err != nil {
		return nil, err
	}
	return env, nil
}

var errAgentNameAlreadySet = errors.New(
	"maas-agent-name is already set; this should not be set by hand")

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
func (p maasEnvironProvider) RestrictedConfigAttributes() []string {
	return []string{"maas-server"}
}

// PrepareConfig is specified in the EnvironProvider interface.
func (p maasEnvironProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	// For MAAS, the cloud endpoint may be either a full URL
	// for the MAAS server, or just the IP/host.
	if args.Cloud.Endpoint == "" {
		return nil, errors.New("MAAS server not specified")
	}
	server := args.Cloud.Endpoint
	if url, err := url.Parse(server); err != nil || url.Scheme == "" {
		server = fmt.Sprintf("http://%s/MAAS", args.Cloud.Endpoint)
	}

	attrs := map[string]interface{}{
		"maas-server": server,
	}
	// Add the credentials.
	switch authType := args.Cloud.Credential.AuthType(); authType {
	case cloud.OAuth1AuthType:
		credentialAttrs := args.Cloud.Credential.Attributes()
		for k, v := range credentialAttrs {
			attrs[k] = v
		}
	default:
		return nil, errors.NotSupportedf("%q auth-type", authType)
	}

	// Set maas-agent-name; make sure it's not set by the user.
	if _, ok := args.Config.UnknownAttrs()["maas-agent-name"]; ok {
		return nil, errAgentNameAlreadySet
	}
	attrs["maas-agent-name"] = args.Config.UUID()

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

// SecretAttrs is specified in the EnvironProvider interface.
func (prov maasEnvironProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	secretAttrs := make(map[string]string)
	maasCfg, err := prov.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	secretAttrs["maas-oauth"] = maasCfg.maasOAuth()
	return secretAttrs, nil
}

// DetectRegions is specified in the environs.CloudRegionDetector interface.
func (p maasEnvironProvider) DetectRegions() ([]cloud.Region, error) {
	return nil, errors.NotFoundf("regions")
}
