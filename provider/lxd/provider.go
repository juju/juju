// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"net"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/retry"
	"github.com/juju/schema"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"github.com/lxc/lxd/shared"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/lxd/lxdnames"
)

type environProvider struct {
	environProviderCredentials
	interfaceAddress func(string) (string, error)
	newLocalServer   func() (*lxd.Server, error)
	Clock            clock.Clock
}

var cloudSchema = &jsonschema.Schema{
	Type:     []jsonschema.Type{jsonschema.ObjectType},
	Required: []string{cloud.EndpointKey},
	Order:    []string{cloud.EndpointKey},
	Properties: map[string]*jsonschema.Schema{
		cloud.EndpointKey: {
			Singular: "the API endpoint url for the remote LXD cloud",
			Type:     []jsonschema.Type{jsonschema.StringType},
			Format:   jsonschema.FormatURI,
		},
	},
}

// NewProvider returns a new LXD EnvironProvider.
func NewProvider() environs.CloudEnvironProvider {
	return &environProvider{
		environProviderCredentials: environProviderCredentials{
			generateMemCert: shared.GenerateMemCert,
			lookupHost:      net.LookupHost,
			interfaceAddrs:  net.InterfaceAddrs,
			newLocalServer:  createLXDServer,
		},
		interfaceAddress: utils.GetAddressForInterface,
		newLocalServer:   createLXDServer,
	}
}

// Version is part of the EnvironProvider interface.
func (*environProvider) Version() int {
	return 0
}

// Open implements environs.EnvironProvider.
func (p *environProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	local, err := p.validateCloudSpec(args.Cloud)
	if err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	env, err := newEnviron(
		p,
		local,
		args.Cloud,
		args.Config,
		newRawProvider,
	)
	return env, errors.Trace(err)
}

// CloudSchema returns the schema used to validate input for add-cloud.
func (p *environProvider) CloudSchema() *jsonschema.Schema {
	return cloudSchema
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p *environProvider) Ping(ctx context.ProviderCallContext, endpoint string) error {
	// connection to the remote server will also do a implicit get request upon
	// connecting to seed the server.
	// TODO (stickupkid): use a centralized construction
	_, err := lxd.ConnectRemote(lxd.RemoteServer{
		Host: endpoint,
	})
	if err != nil {
		return errors.Errorf("no lxd server running at %s", endpoint)
	}
	return nil
}

// PrepareConfig implements environs.EnvironProvider.
func (p *environProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	_, err := p.validateCloudSpec(args.Cloud)
	if err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	// Set the default filesystem-storage source.
	attrs := make(map[string]interface{})
	if _, ok := args.Config.StorageDefaultFilesystemSource(); !ok {
		attrs[config.StorageDefaultFilesystemSourceKey] = lxdStorageProviderType
	}
	if len(attrs) == 0 {
		return args.Config, nil
	}
	cfg, err := args.Config.Apply(attrs)
	return cfg, errors.Trace(err)
}

// Validate implements environs.EnvironProvider.
func (*environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	if _, err := newValidConfig(cfg); err != nil {
		return nil, errors.Annotate(err, "invalid base config")
	}
	return cfg, nil
}

// DetectClouds implements environs.CloudDetector.
func (p *environProvider) DetectClouds() ([]cloud.Cloud, error) {
	return []cloud.Cloud{localhostCloud}, nil
}

// DetectCloud implements environs.CloudDetector.
func (p *environProvider) DetectCloud(name string) (cloud.Cloud, error) {
	// For now we just return a hard-coded "localhost" cloud,
	// i.e. the local LXD daemon. We may later want to detect
	// locally-configured remotes.
	switch name {
	case "lxd", "localhost":
		return localhostCloud, nil
	}
	return cloud.Cloud{}, errors.NotFoundf("cloud %s", name)
}

// FinalizeCloud is part of the environs.CloudFinalizer interface.
func (p *environProvider) FinalizeCloud(
	ctx environs.FinalizeCloudContext,
	in cloud.Cloud,
) (cloud.Cloud, error) {

	var endpoint string
	resolveEndpoint := func(ep *string) error {
		if *ep != "" {
			return nil
		}
		if endpoint == "" {
			// The cloud endpoint is empty, which means
			// that we should connect to the local LXD.
			var err error
			hostAddress, err := p.getLocalHostAddress(ctx)
			if err != nil {
				return errors.Trace(err)
			}
			endpoint = hostAddress
		}
		*ep = endpoint
		return nil
	}

	if err := resolveEndpoint(&in.Endpoint); err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}
	for i := range in.Regions {
		if err := resolveEndpoint(&in.Regions[i].Endpoint); err != nil {
			return cloud.Cloud{}, errors.Trace(err)
		}
	}
	return in, nil
}

func (p *environProvider) getLocalHostAddress(ctx environs.FinalizeCloudContext) (string, error) {
	svr, err := p.newLocalServer()
	if err != nil {
		return "", errors.Trace(err)
	}

	bridgeName := svr.LocalBridgeName()
	hostAddress, err := p.interfaceAddress(bridgeName)
	if err != nil {
		return "", errors.Trace(err)
	}
	hostAddress = lxd.EnsureHTTPS(hostAddress)

	// The following retry mechanism is required for newer LXD versions, where
	// the new lxd client doesn't propagate the EnableHTTPSListener quick enough
	// to get the addresses or on the same existing local provider.

	// connInfoAddresses is really useful for debugging, so let's keep that
	// information around for the debugging errors.
	var connInfoAddresses []string
	errNotExists := errors.New("not-exists")
	retryArgs := retry.CallArgs{
		Clock: p.clock(),
		IsFatalError: func(err error) bool {
			return errors.Cause(err) != errNotExists
		},
		Func: func() error {
			cInfo, err := svr.GetConnectionInfo()
			if err != nil {
				return errors.Trace(err)
			}

			connInfoAddresses = cInfo.Addresses
			for _, addr := range cInfo.Addresses {
				if strings.HasPrefix(addr, hostAddress+":") {
					hostAddress = addr
					return nil
				}
			}

			// Requesting a NewLocalServer forces a new connection, so that when
			// we GetConnectionInfo it gets the required addresses.
			// Note: this modifies the outer svr server.
			if svr, err = lxd.NewLocalServer(); err != nil {
				return errors.Trace(err)
			}

			return errNotExists
		},
		Delay:    2 * time.Second,
		Attempts: 30,
	}
	if err := retry.Call(retryArgs); err != nil {
		return "", errors.Errorf(
			"LXD is not listening on address %s (reported addresses: %s)",
			hostAddress, connInfoAddresses,
		)
	}
	ctx.Verbosef(
		"Resolved LXD host address on bridge %s: %s",
		bridgeName, hostAddress,
	)
	return hostAddress, nil
}

func (p *environProvider) clock() clock.Clock {
	if p.Clock == nil {
		return clock.WallClock
	}
	return p.Clock
}

func createLXDServer() (*lxd.Server, error) {
	svr, err := lxd.NewLocalServer()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// We need to get a default profile, so that the local bridge name
	// can be discovered correctly to then get the host address.
	defaultProfile, profileETag, err := svr.GetProfile("default")
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := svr.VerifyNetworkDevice(defaultProfile, profileETag); err != nil {
		return nil, errors.Trace(err)
	}

	// LXD itself reports the host:ports that it listens on.
	// Cross-check the address we have with the values reported by LXD.
	if err := svr.EnableHTTPSListener(); err != nil {
		return nil, errors.Annotate(err, "enabling HTTPS listener")
	}

	return svr, nil
}

// localhostCloud is the predefined "localhost" LXD cloud. We leave the
// endpoints empty to indicate that LXD is on the local host. When the
// cloud is finalized (see FinalizeCloud), we resolve the bridge address
// of the LXD host, and use that as the endpoint.
var localhostCloud = cloud.Cloud{
	Name: lxdnames.DefaultCloud,
	Type: lxdnames.ProviderType,
	AuthTypes: []cloud.AuthType{
		interactiveAuthType,
		cloud.CertificateAuthType,
	},
	Endpoint: "",
	Regions: []cloud.Region{{
		Name:     lxdnames.DefaultRegion,
		Endpoint: "",
	}},
	Description: cloud.DefaultCloudDescription(lxdnames.ProviderType),
}

// DetectRegions implements environs.CloudRegionDetector.
func (*environProvider) DetectRegions() ([]cloud.Region, error) {
	// For now we just return a hard-coded "localhost" region,
	// i.e. the local LXD daemon. We may later want to detect
	// locally-configured remotes.
	return []cloud.Region{{Name: lxdnames.DefaultRegion}}, nil
}

// Schema returns the configuration schema for an environment.
func (*environProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

func (p *environProvider) validateCloudSpec(spec environs.CloudSpec) (local bool, _ error) {
	if err := spec.Validate(); err != nil {
		return false, errors.Trace(err)
	}
	if spec.Credential == nil {
		return false, errors.NotValidf("missing credential")
	}
	if spec.Endpoint == "" {
		// If we're dealing with an old controller, or we're preparing
		// a local LXD, we'll have an empty endpoint. Connect to the
		// default Unix socket.
		local = true
	} else {
		if _, err := endpointURL(spec.Endpoint); err != nil {
			return false, errors.Trace(err)
		}
	}
	switch authType := spec.Credential.AuthType(); authType {
	case cloud.CertificateAuthType:
		if _, _, ok := getCertificates(spec); !ok {
			return false, errors.NotValidf("certificate credentials")
		}
	default:
		return false, errors.NotSupportedf("%q auth-type", authType)
	}
	return local, nil
}

// ConfigSchema returns extra config attributes specific
// to this provider only.
func (p *environProvider) ConfigSchema() schema.Fields {
	return configFields
}

// ConfigDefaults returns the default values for the
// provider specific config attributes.
func (p *environProvider) ConfigDefaults() schema.Defaults {
	return configDefaults
}
