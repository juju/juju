// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"github.com/juju/juju/version"
	"github.com/juju/utils/proxy"
)

// AddAptCommands update the cloudinit.Config instance with the necessary
// packages, the request to do the apt-get update/upgrade on boot, and adds
// the apt proxy and mirror settings if there are any.

// This belongs in packaging in my opinion but we have to keep it here because
// of import cycles

// My main idea would be to split this in os/packagetype dependant functions
// and create those to deal with the stuff

// It will also probably at some point receive yumMirror as well as yumProxy
// settings in addition to the apt ones because we need information about both
// for a proper environment

// Same ideas apply to the MaybeAddStuff
func AddPackageCommands(
	series string,
	aptProxySettings proxy.Settings,
	aptMirror string,
	cfg CloudConfig,
	addUpdateScripts bool,
	addUpgradeScripts bool,
) error {
	// Check preconditions
	if cfg == nil {
		panic("AddPackageCommands received nil CloudConfig")
	}

	// Set the APT mirror.
	// TODO: in the future we might pass yumMirror as well here
	// SetPackage mirror knows the OS of the configuration we need to make sure
	// what we pass to it or parse it inside
	cfg.SetPackageMirror(aptMirror)

	// For LTS series which need support for the cloud-tools archive,
	// we need to enable apt-get update regardless of the environ
	// setting, otherwise bootstrap or provisioning will fail.
	if series == "precise" && !addUpdateScripts {
		addUpdateScripts = true
	}

	// Bring packages up-to-date.
	cfg.SetSystemUpdate(addUpdateScripts)
	cfg.SetSystemUpgrade(addUpgradeScripts)
	//c.SetAptGetWrapper("eatmydata")

	// If we're not doing an update, adding these packages is
	// meaningless.
	// TODO: Decide when we update on CentOS
	if addUpdateScripts {
		err := updatePackages(series, cfg)
		if err != nil {
			return err
		}
	}

	// TODO: Deal with proxy settings on CentOS
	err := updateProxySettings(series, cfg, aptProxySettings)
	if err != nil {
		return err
	}
	return nil
}

//TODO: These 3 functions might have to be refactored into something else, now
//they're just ugly
func updatePackages(series string, cfg CloudConfig) error {
	os, err := version.GetOSFromSeries(series)
	if err != nil {
		return err
	}
	switch os {
	case version.Ubuntu:
		updatePackagesUbuntu(cfg, series)
	case version.CentOS:
		//TODO: Do something here
	}
	return nil
}

func updateProxySettings(series string, cfg CloudConfig, aptProxySettings proxy.Settings) error {
	os, err := version.GetOSFromSeries(series)
	if err != nil {
		return err
	}
	switch os {
	case version.Ubuntu:
		updateProxySettingsUbuntu(cfg, aptProxySettings)
	case version.CentOS:
		//TODO: Do something here
	}
	return nil
}

// MaybeAddCloudArchiveCloudTools adds the cloud-archive cloud-tools
// pocket to apt sources, if the series requires it.
func MaybeAddCloudArchiveCloudTools(cfg CloudConfig, series string) error {
	os, err := version.GetOSFromSeries(series)
	if err != nil {
		return err
	}
	switch os {
	case version.Ubuntu:
		maybeAddCloudArchiveCloudToolsUbuntu(cfg, series)
	case version.CentOS:
		//TODO: Do something here
	}
	return nil
}
