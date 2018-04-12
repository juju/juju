// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"net"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/schema"
	"github.com/juju/utils"
	"github.com/lxc/lxd/shared"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/lxd/lxdnames"
	"github.com/juju/juju/tools/lxdclient"
)

type environProvider struct {
	environProviderCredentials
	interfaceAddress func(string) (string, error)
}

// NewProvider returns a new LXD EnvironProvider.
func NewProvider() environs.EnvironProvider {
	return &environProvider{
		environProviderCredentials: environProviderCredentials{
			generateMemCert:     shared.GenerateMemCert,
			newLocalRawProvider: newLocalRawProvider,
			lookupHost:          net.LookupHost,
			interfaceAddrs:      net.InterfaceAddrs,
		},
		interfaceAddress: utils.GetAddressForInterface,
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

// CloudSchema returns the schema used to validate input for add-cloud.  Since
// this provider does not support custom clouds, this always returns nil.
func (p *environProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p *environProvider) Ping(endpoint string) error {
	return errors.NotImplementedf("Ping")
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
	return args.Config.Apply(attrs)
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
				return err
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
	raw, err := p.newLocalRawProvider()
	if err != nil {
		return "", errors.Trace(err)
	}
	bridgeName := raw.DefaultProfileBridgeName()
	hostAddress, err := p.interfaceAddress(bridgeName)
	if err != nil {
		return "", errors.Trace(err)
	}
	// LXD itself reports the host:ports that is listens on.
	// Cross-check the address we have with the values
	// reported by LXD.
	if err := lxdclient.EnableHTTPSListener(raw); err != nil {
		return "", errors.Annotate(err, "enabling HTTPS listener")
	}
	serverAddresses, err := raw.ServerAddresses()
	if err != nil {
		return "", errors.Trace(err)
	}
	var found bool
	for _, addr := range serverAddresses {
		if strings.HasPrefix(addr, hostAddress+":") {
			hostAddress = addr
			found = true
			break
		}
	}
	if !found {
		return "", errors.Errorf(
			"LXD is not listening on address %s ("+
				"reported addresses: %s)",
			hostAddress, serverAddresses,
		)
	}
	ctx.Verbosef(
		"Resolved LXD host address on bridge %s: %s",
		bridgeName, hostAddress,
	)
	return hostAddress, nil
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
		if _, _, ok := getCerts(spec); !ok {
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
