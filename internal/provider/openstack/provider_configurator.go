// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/go-goose/goose/v5/nova"
	"github.com/juju/errors"
	"github.com/juju/schema"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
)

// This interface is added to allow to customize OpenStack provider behaviour.
// This is used in other providers, that embeds OpenStack provider.
type ProviderConfigurator interface {
	// GetConfigDefaults sets some configuration default values, if any
	GetConfigDefaults() schema.Defaults

	// This method allows to adjust default RunServerOptions,
	// before new server is actually created.
	ModifyRunServerOptions(options *nova.RunServerOpts)

	// This method provides default cloud config.
	// This config can be different for different providers.
	GetCloudConfig(args environs.StartInstanceParams) (cloudinit.CloudConfig, error)
}

type defaultConfigurator struct{}

// ModifyRunServerOptions implements ProviderConfigurator interface.
func (c *defaultConfigurator) ModifyRunServerOptions(_ *nova.RunServerOpts) {
}

// GetCloudConfig implements ProviderConfigurator interface.
func (c *defaultConfigurator) GetCloudConfig(args environs.StartInstanceParams) (cloudinit.CloudConfig, error) {
	cloudCfg, err := cloudinit.New(args.InstanceConfig.Base.OS)
	return cloudCfg, errors.Trace(err)
}

// GetConfigDefaults implements ProviderConfigurator interface.
func (c *defaultConfigurator) GetConfigDefaults() schema.Defaults {
	return schema.Defaults{
		"use-default-secgroup": false,
		"network":              "",
		"external-network":     "",
		"use-openstack-gbp":    false,
		"policy-target-group":  "",
	}
}
