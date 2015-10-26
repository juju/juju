// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/openstack"
)

var logger = loggo.GetLogger("juju.provider.rackspace")

type environProvider struct {
<<<<<<< HEAD
<<<<<<< HEAD
	environs.EnvironProvider
=======
	openstackProvider environs.EnvironProvider
>>>>>>> modifications to opestack provider applied
=======
	environs.EnvironProvider
>>>>>>> review comments implemented
}

var providerInstance environProvider

<<<<<<< HEAD
<<<<<<< HEAD
<<<<<<< HEAD
=======
>>>>>>> working version of rackspace provider
func (p environProvider) setConfigurator(env environs.Environ, err error) (environs.Environ, error) {
=======
func (p environProvider) setFirewaller(env environs.Environ, err error) (environs.Environ, error) {
>>>>>>> Firewaller interface added, Waith ssh method reused
	if err != nil {
		return nil, errors.Trace(err)
	}
<<<<<<< HEAD
<<<<<<< HEAD
	if osEnviron, ok := env.(*openstack.Environ); ok {
		osEnviron.SetProviderConfigurator(new(rackspaceProviderConfigurator))
=======
// Open implements environs.EnvironProvider.
func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	env, err := p.openstackProvider.Open(cfg)
=======
>>>>>>> working version of rackspace provider
	if os, ok := env.(*openstack.Environ); ok {
		os.SetProviderConfigurator(new(rackspaceProviderConfigurator))
>>>>>>> modifications to opestack provider applied
		return environ{env}, errors.Trace(err)
=======
	if osEnviron, ok := env.(*openstack.Environ); ok {
<<<<<<< HEAD
		osEnviron.SetProviderConfigurator(new(rackspaceProviderConfigurator))
<<<<<<< HEAD
		return environ{env}, err
>>>>>>> review comments implemented
=======
=======
		osEnviron.SetFirewaller(new(rackspaceFirewaller))
>>>>>>> Firewaller interface added, Waith ssh method reused
		return environ{env}, errors.Trace(err)
>>>>>>> More review comments implemented
	}
	return nil, errors.Errorf("Expected openstack.Environ, but got: %T", env)
}

<<<<<<< HEAD
<<<<<<< HEAD
// Open implements environs.EnvironProvider.
func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	env, err := p.EnvironProvider.Open(cfg)
	res, err := p.setFirewaller(env, err)
	return res, errors.Trace(err)
=======
=======
// Open implements environs.EnvironProvider.
func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
<<<<<<< HEAD
	env, err := p.openstackProvider.Open(cfg)
	return p.setConfigurator(env, err)
}

>>>>>>> working version of rackspace provider
// RestrictedConfigAttributes implements environs.EnvironProvider.
func (p environProvider) RestrictedConfigAttributes() []string {
	return p.openstackProvider.RestrictedConfigAttributes()
}

// PrepareForCreateEnvironment implements environs.EnvironProvider.
func (p environProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	return p.openstackProvider.PrepareForCreateEnvironment(cfg)
>>>>>>> modifications to opestack provider applied
=======
	env, err := p.EnvironProvider.Open(cfg)
	res, err := p.setConfigurator(env, err)
	return res, errors.Trace(err)
>>>>>>> review comments implemented
}

// PrepareForBootstrap implements environs.EnvironProvider.
func (p environProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
<<<<<<< HEAD
<<<<<<< HEAD
<<<<<<< HEAD
	env, err := p.EnvironProvider.PrepareForBootstrap(ctx, cfg)
	res, err := p.setFirewaller(env, err)
	return res, errors.Trace(err)
=======
	return p.openstackProvider.PrepareForBootstrap(ctx, cfg)
>>>>>>> modifications to opestack provider applied
=======
	env, err := p.openstackProvider.PrepareForBootstrap(ctx, cfg)
	return p.setConfigurator(env, err)
>>>>>>> working version of rackspace provider
=======
	env, err := p.EnvironProvider.PrepareForBootstrap(ctx, cfg)
	res, err := p.setConfigurator(env, err)
	return res, errors.Trace(err)
>>>>>>> review comments implemented
}

<<<<<<< HEAD
// Validate implements environs.EnvironProvider.
func (p environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
<<<<<<< HEAD
<<<<<<< HEAD
<<<<<<< HEAD
<<<<<<< HEAD
	return p.openstackProvider.Validate(cfg, old)
=======
	cfg, err = cfg.Apply(map[string]interface{}{
		"use-floating-ip":      false,
		"use-default-secgroup": false,
		"auth-url":             "https://identity.api.rackspacecloud.com/v2.0",
	})
	if err != nil {
		return nil, err
	}
	return p.EnvironProvider.Validate(cfg, old)
>>>>>>> review comments implemented
=======
	return p.openstackProvider.Validate(cfg, old)
>>>>>>> modifications to opestack provider applied
=======
	return p.openstackProvider.Validate(cfg, old)
=======
=======
>>>>>>> fix conflicts after rabasing against master branch
	cfg, err = cfg.Apply(map[string]interface{}{
		"use-floating-ip":      false,
		"use-default-secgroup": false,
		"auth-url":             "https://identity.api.rackspacecloud.com/v2.0",
	})
	if err != nil {
		return nil, err
	}
	return p.EnvironProvider.Validate(cfg, old)
<<<<<<< HEAD
>>>>>>> review comments implemented
>>>>>>> review comments implemented
=======
>>>>>>> fix conflicts after rabasing against master branch
}

=======
>>>>>>> Firewaller interface added, Waith ssh method reused
// BoilerplateConfig implements environs.EnvironProvider.
func (p environProvider) BoilerplateConfig() string {
	return `
# https://juju.ubuntu.com/docs/config-rackspace.html
rackspace:
    type: rackspace

    # network specifies the network label or uuid to bring machines up
    # on, in the case where multiple networks exist. It may be omitted
    # otherwise.
    #
    # network: <your network label or uuid>

    # agent-metadata-url specifies the location of the Juju tools and
    # metadata. It defaults to the global public tools metadata
    # location https://streams.canonical.com/tools.
    #
    # agent-metadata-url:  https://your-agent-metadata-url

    # image-metadata-url specifies the location of Ubuntu cloud image
    # metadata. It defaults to the global public image metadata
    # location https://cloud-images.ubuntu.com/releases.
    #
    # image-metadata-url:  https://your-image-metadata-url

    # image-stream chooses a simplestreams stream from which to select
    # OS images, for example daily or released images (or any other stream
    # available on simplestreams).
    #
    # image-stream: "released"

    # agent-stream chooses a simplestreams stream from which to select tools,
    # for example released or proposed tools (or any other stream available
    # on simplestreams).
    #
    # agent-stream: "released"

    # tenant-name holds the openstack tenant name. It defaults to the
    # environment variable OS_TENANT_NAME.
    #
    # tenant-name: <your tenant name>

    # region holds the openstack region. It defaults to the
    # environment variable OS_REGION_NAME.
    #
    # region: <your region>

    # The auth-mode, username and password attributes are used for
    # userpass authentication (the default).
    #
    # auth-mode holds the authentication mode. For user-password
    # authentication, auth-mode should be "userpass" and username and
    # password should be set appropriately; they default to the
    # environment variables OS_USERNAME and OS_PASSWORD respectively.
    #
    # auth-mode: userpass
    # username: <your username>
    # password: <secret>

    # For key-pair authentication, auth-mode should be "keypair" and
    # access-key and secret-key should be set appropriately; they
    # default to the environment variables OS_ACCESS_KEY and
    # OS_SECRET_KEY respectively.
    #
    # auth-mode: keypair
    # access-key: <secret>
    # secret-key: <secret>

    # Whether or not to refresh the list of available updates for an
    # OS. The default option of true is recommended for use in
    # production systems, but disabling this can speed up local
    # deployments for development or testing.
    #
    # enable-os-refresh-update: true

    # Whether or not to perform OS upgrades when machines are
    # provisioned. The default option of true is recommended for use
    # in production systems, but disabling this can speed up local
    # deployments for development or testing.
    #
    # enable-os-upgrade: true
`[1:]
}
<<<<<<< HEAD
<<<<<<< HEAD
=======

// SecretAttrs implements environs.EnvironProvider.
func (p environProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	return p.openstackProvider.SecretAttrs(cfg)
}
>>>>>>> modifications to opestack provider applied
=======
>>>>>>> review comments implemented
