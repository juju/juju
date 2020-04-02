// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/loggo"
	"gopkg.in/goose.v2/client"
	"gopkg.in/goose.v2/identity"
	gooselogging "gopkg.in/goose.v2/logging"
	"gopkg.in/goose.v2/neutron"
	"gopkg.in/goose.v2/nova"
)

// ClientFactory creates various goose (openstack) clients.
// TODO (stickupkid): This should be moved into goose and the factory should
// accept a configuration returning back goose clients.
type ClientFactory struct {
	spec environs.CloudSpec
	ecfg *environConfig

	// We store the auth client, so nova can reuse it.
	authClient client.AuthenticatingClient
}

// NewClientFactory creates a new ClientFactory from the CloudSpec and environ
// config arguments.
func NewClientFactory(spec environs.CloudSpec, ecfg *environConfig) (*ClientFactory, error) {
	// This is an wanted side effect of the previous implementation only calling
	// AuthClient once. To prevent the regression of calling it three times, one
	// for checking which auth client to use, then for nova and then for
	// neutron. We get the auth client for the factory and reuse it for nova.
	authClient, err := getClientState(spec, ecfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &ClientFactory{
		spec:       spec,
		ecfg:       ecfg,
		authClient: authClient,
	}, nil
}

// AuthClient returns an goose AuthenticatingClient.
func (c *ClientFactory) AuthClient() client.AuthenticatingClient {
	return c.authClient
}

// Nova creates a new Nova client from the auth mode (v3 or falls back to v2)
// and the updated credentials.
func (c *ClientFactory) Nova() (*nova.Client, error) {
	return nova.New(c.authClient), nil
}

// Neutron creates a new Neutron client from the auth mode (v3 or falls back to v2)
// and the updated credentials.
// Note: we override the http.Client headers with specific neutron client
// headers.
func (c *ClientFactory) Neutron() (*neutron.Client, error) {
	httpOption := client.WithHTTPHeadersFunc(neutron.NeutronHeaders)

	client, err := getClientState(c.spec, c.ecfg, httpOption)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return neutron.New(client), nil
}

func getClientState(spec environs.CloudSpec, ecfg *environConfig, options ...client.Option) (client.AuthenticatingClient, error) {
	identityClientVersion, err := identityClientVersion(spec.Endpoint)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create a client")
	}
	cred, authMode, err := newCredentials(spec)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create credential")
	}

	// Create a new fallback client using the existing authMode.
	newClient, _ := getClientByAuthModel(authMode, cred, spec, ecfg, options...)

	// Before returning, lets make sure that we want to have AuthMode
	// AuthUserPass instead of its V3 counterpart.
	if authMode == identity.AuthUserPass && (identityClientVersion == -1 || identityClientVersion == 3) {
		authOptions, err := newClient.IdentityAuthOptions()
		if err != nil {
			logger.Errorf("cannot determine available auth versions %v", err)
		}

		// Walk over the options to verify if the AuthUserPassV3 exists, if it
		// does exist use that to attempt authenticate.
		var authOption *identity.AuthOption
		for _, option := range authOptions {
			if option.Mode == identity.AuthUserPassV3 {
				authOption = &option
				break
			}
		}

		// No AuthUserPassV3 found, exit early as no additional work is
		// required.
		if authOption == nil {
			return newClient, nil
		}

		// Update the credential with the new identity.AuthOption and
		// attempt to Authenticate.
		newCreds := &cred
		newCreds.URL = authOption.Endpoint

		newClientV3, err := getClientByAuthModel(identity.AuthUserPassV3, *newCreds, spec, ecfg, options...)
		if err != nil {
			return nil, errors.Trace(err)
		}

		// If the AuthUserPassV3 client can authenticate, use it.
		// Otherwise fallback to the v2 client.
		if err = newClientV3.Authenticate(); err == nil {
			return newClientV3, nil
		}
	}
	return newClient, nil
}

// getClientByAuthModel creates a new client for the given AuthMode.
func getClientByAuthModel(authMode identity.AuthMode, cred identity.Credentials, spec environs.CloudSpec, ecfg *environConfig, options ...client.Option) (client.AuthenticatingClient, error) {
	gooseLogger := gooselogging.LoggoLogger{
		Logger: loggo.GetLogger("goose"),
	}

	newClient, err := newClientByType(cred, authMode, gooseLogger, ecfg.SSLHostnameVerification(), spec.CACertificates, options...)
	if err != nil {
		return nil, errors.NewNotValid(err, "cannot create a new client")
	}

	// Juju requires "compute" at a minimum. We'll use "network" if it's
	// available in preference to the Neutron network APIs; and "volume" or
	// "volume2" for storage if either one is available.
	newClient.SetRequiredServiceTypes([]string{"compute"})
	return newClient, nil
}

// newClientByType returns an authenticating client to talk to the
// OpenStack cloud.  CACertificate and SSLHostnameVerification == false
// config options are mutually exclusive here.
func newClientByType(
	cred identity.Credentials,
	authMode identity.AuthMode,
	gooseLogger gooselogging.CompatLogger,
	sslHostnameVerification bool,
	certs []string,
	options ...client.Option,
) (client.AuthenticatingClient, error) {
	switch {
	case len(certs) > 0:
		tlsConfig := tlsConfig(certs)
		logger.Tracef("using NewClientTLSConfig")
		return client.NewClientTLSConfig(&cred, authMode, gooseLogger, tlsConfig, options...), nil
	case sslHostnameVerification == false:
		logger.Tracef("using NewNonValidatingClient")
		return client.NewNonValidatingClient(&cred, authMode, gooseLogger, options...), nil
	default:
		logger.Tracef("using NewClient")
		return client.NewClient(&cred, authMode, gooseLogger, options...), nil
	}
}
