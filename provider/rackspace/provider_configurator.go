// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/goose.v2/nova"

	"github.com/juju/juju/cloudconfig/cloudinit"
	jujuos "github.com/juju/juju/core/os"
	jujuseries "github.com/juju/juju/core/series"
	"github.com/juju/juju/environs"
)

type rackspaceConfigurator struct {
}

// ModifyRunServerOptions implements ProviderConfigurator interface.
func (c *rackspaceConfigurator) ModifyRunServerOptions(options *nova.RunServerOpts) {
	// More on how ConfigDrive option is used on rackspace:
	// http://docs.rackspace.com/servers/api/v2/cs-devguide/content/config_drive_ext.html
	options.ConfigDrive = true
}

// GetCloudConfig implements ProviderConfigurator interface.
func (c *rackspaceConfigurator) GetCloudConfig(args environs.StartInstanceParams) (cloudinit.CloudConfig, error) {
	series := args.Tools.OneSeries()
	cloudcfg, err := cloudinit.New(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Additional package required for sshInstanceConfigurator, to save
	// iptables state between restarts.
	cloudcfg.AddPackage("iptables-persistent")

	if args.InstanceConfig.EnableOSRefreshUpdate {
		// cloud-init often fails to update APT caches
		// during instance startup on RackSpace. Add an
		// extra call to "apt-get update" with a sleep
		// on failure to attempt to alleviate this.
		// See lp:1677425.
		os, err := jujuseries.GetOSFromSeries(series)
		if err == nil && os == jujuos.Ubuntu {
			cloudcfg.AddBootCmd("apt-get update || (sleep 30s; apt-get update)")
		}
	}

	return cloudcfg, nil
}

// GetConfigDefaults implements ProviderConfigurator interface.
func (c *rackspaceConfigurator) GetConfigDefaults() schema.Defaults {
	return schema.Defaults{
		"use-floating-ip":      false,
		"use-default-secgroup": false,
		"network":              "",
		"external-network":     "",
		"use-openstack-gbp":    false,
		"policy-target-group":  "",
	}
}
