// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"context"
	"net/http"

	"github.com/go-goose/goose/v5/client"
	goosehttp "github.com/go-goose/goose/v5/http"
	"github.com/go-goose/goose/v5/identity"
	gooselogging "github.com/go-goose/goose/v5/logging"
	"github.com/go-goose/goose/v5/neutron"
	"github.com/go-goose/goose/v5/nova"
	"github.com/juju/errors"

	corelogger "github.com/juju/juju/core/logger"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	jujuhttp "github.com/juju/juju/internal/http"
	internallogger "github.com/juju/juju/internal/logger"
)

// ClientOption to be passed into the transport construction to customize the
// default transport.
type ClientOption func(*clientOptions)

type clientOptions struct {
	caCertificates           []string
	skipHostnameVerification bool
	httpHeadersFunc          goosehttp.HeadersFunc
	httpClient               *http.Client
}

// WithCACertificates contains Authority certificates to be used to validate
// certificates of cloud infrastructure components.
// The contents are Base64 encoded x.509 certs.
func WithCACertificates(value ...string) ClientOption {
	return func(opt *clientOptions) {
		opt.caCertificates = value
	}
}

// WithSkipHostnameVerification will skip hostname verification on the TLS/SSL
// certificates.
func WithSkipHostnameVerification(value bool) ClientOption {
	return func(opt *clientOptions) {
		opt.skipHostnameVerification = value
	}
}

// WithHTTPHeadersFunc allows passing in a new HTTP headers func for the client
// to execute for each request.
func WithHTTPHeadersFunc(httpHeadersFunc goosehttp.HeadersFunc) ClientOption {
	return func(clientOptions *clientOptions) {
		clientOptions.httpHeadersFunc = httpHeadersFunc
	}
}

// WithHTTPClient allows to define the http.Client to use.
func WithHTTPClient(value *http.Client) ClientOption {
	return func(opt *clientOptions) {
		opt.httpClient = value
	}
}

// Create a clientOptions instance with default values.
func newOptions() *clientOptions {
	// In this case, use a default http.Client.
	// Ideally we should always use the NewHTTPTLSTransport,
	// however some test suites and some facade tests
	// rely on settings to the http.DefaultTransport for
	// tests to run with different protocol scheme such as "test"
	// and some replace the RoundTripper to answer test scenarios.
	//
	// https://bugs.launchpad.net/juju/+bug/1888888
	defaultCopy := *http.DefaultClient

	return &clientOptions{
		httpHeadersFunc: goosehttp.DefaultHeaders,
		httpClient:      &defaultCopy,
	}
}

// SSLHostnameConfig defines the options for host name verification
type SSLHostnameConfig interface {
	SSLHostnameVerification() bool
}

// ClientFunc is used to create a goose client.
type ClientFunc = func(cred identity.Credentials,
	authMode identity.AuthMode,
	options ...ClientOption) (client.AuthenticatingClient, error)

// ClientFactory creates various goose (openstack) clients.
// TODO (stickupkid): This should be moved into goose and the factory should
// accept a configuration returning back goose clients.
type ClientFactory struct {
	spec              environscloudspec.CloudSpec
	sslHostnameConfig SSLHostnameConfig

	// We store the auth client, so nova can reuse it.
	authClient client.AuthenticatingClient

	// clientFunc is used to create a client from a set of arguments.
	clientFunc ClientFunc
}

// NewClientFactory creates a new ClientFactory from the CloudSpec and environ
// config arguments.
func NewClientFactory(spec environscloudspec.CloudSpec, sslHostnameConfig SSLHostnameConfig) *ClientFactory {
	return &ClientFactory{
		spec:              spec,
		sslHostnameConfig: sslHostnameConfig,
		clientFunc:        newClient,
	}
}

// Init the client factory, returns an error if the initialization fails.
func (c *ClientFactory) Init() error {
	// This is an unwanted side effect of the previous implementation only
	// calling AuthClient once.
	// To prevent the regression of calling it three times, one for checking,
	// which auth client to use, then for nova and then for neutron.
	// We get the auth client for the factory and reuse it for nova.
	authClient, err := c.getClientState()
	if err != nil {
		return errors.Trace(err)
	}
	c.authClient = authClient
	return nil
}

// AuthClient returns a goose AuthenticatingClient.
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
	client, err := c.getClientState(WithHTTPHeadersFunc(neutron.NeutronHeaders))
	if err != nil {
		return nil, errors.Trace(err)
	}
	return neutron.New(client), nil
}

func (c *ClientFactory) getClientState(options ...ClientOption) (client.AuthenticatingClient, error) {
	identityClientVersion, err := identityClientVersion(c.spec.Endpoint)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create a client")
	}
	cred, authMode, err := newCredentials(c.spec)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create credential")
	}

	// Create a new fallback client using the existing authMode.
	newClient, _ := c.getClientByAuthMode(authMode, cred, options...)

	// Before returning, lets make sure that we want to have AuthMode
	// AuthUserPass instead of its V3 counterpart.
	if authMode == identity.AuthUserPass && (identityClientVersion == -1 || identityClientVersion == 3) {
		authOptions, err := newClient.IdentityAuthOptions()
		if err != nil {
			logger.Errorf(context.TODO(), "cannot determine available auth versions %v", err)
		}

		// Walk over the options to verify if the AuthUserPassV3 exists, if it
		// does exist use that to attempt authentication.
		var authOption *identity.AuthOption
		for _, v := range authOptions {
			option := v
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

		newClientV3, err := c.getClientByAuthMode(identity.AuthUserPassV3, *newCreds, options...)
		if err != nil {
			return nil, errors.Trace(err)
		}

		// If the AuthUserPassV3 client can authenticate, use it.
		if err = newClientV3.Authenticate(); err == nil {
			return newClientV3, nil
		}
		if identityClientVersion == 3 {
			// We know it's a v3 server, so we can't fall back to v2.
			return nil, errors.Trace(err)
		}
		// Otherwise, fall back to the v2 client.
	}
	return newClient, nil
}

// getClientByAuthMode creates a new client for the given AuthMode.
func (c *ClientFactory) getClientByAuthMode(authMode identity.AuthMode, cred identity.Credentials, options ...ClientOption) (client.AuthenticatingClient, error) {
	newClient, err := c.clientFunc(cred, authMode,
		append(options,
			WithSkipHostnameVerification(!c.sslHostnameConfig.SSLHostnameVerification()),
			WithCACertificates(c.spec.CACertificates...),
		)...,
	)
	if err != nil {
		return nil, errors.NewNotValid(err, "cannot create a new client")
	}

	// Juju requires "compute" at a minimum. We'll use "network" if it's
	// available in preference to the Neutron network APIs; and "volume" or
	// "volume2" for storage if either one is available.
	newClient.SetRequiredServiceTypes([]string{"compute"})
	return newClient, nil
}

// newClient returns an authenticating client to talk to the
// OpenStack cloud.  CACertificate and SSLHostnameVerification == false
// config options are mutually exclusive here.
func newClient(
	cred identity.Credentials,
	authMode identity.AuthMode,
	clientOptions ...ClientOption,
) (client.AuthenticatingClient, error) {
	opts := newOptions()
	for _, option := range clientOptions {
		option(opts)
	}

	logger := internallogger.GetLogger("goose")
	gooseLogger := gooselogging.DebugLoggerAdapater{
		Logger: wrapLogger(logger),
	}

	httpClient := jujuhttp.NewClient(
		jujuhttp.WithSkipHostnameVerification(opts.skipHostnameVerification),
		jujuhttp.WithCACertificates(opts.caCertificates...),
		jujuhttp.WithLogger(logger.Child("http", corelogger.HTTP)),
	)
	return client.NewClient(&cred, authMode, gooseLogger,
		client.WithHTTPClient(httpClient.Client()),
		client.WithHTTPHeadersFunc(opts.httpHeadersFunc),
	), nil
}

// wrappedLogger is a logger.Logger that logs to dependency.Logger interface.
type wrappedLogger struct {
	logger corelogger.Logger
}

// wrapLogger returns a new instance of wrappedLogger.
func wrapLogger(logger corelogger.Logger) *wrappedLogger {
	return &wrappedLogger{
		logger: logger,
	}
}

// Debug logs a message at the debug level.
func (c *wrappedLogger) Debugf(msg string, args ...any) {
	c.logger.Helper()
	// We should either fix the goose logger to use a context, or we should
	// instantiate a new client for each request rather than caching it for
	// the lifetime of the provider.
	c.logger.Debugf(context.Background(), msg, args...)
}
