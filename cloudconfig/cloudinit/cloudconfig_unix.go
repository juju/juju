// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// The cloudinit package implements a way of creating
// a cloud-init configuration file.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import (
	"fmt"
	"strings"

	"github.com/juju/utils/apt"
	"github.com/juju/utils/proxy"
)

type unixCloudConfig struct {
	unixCommon
}

type unixCommon interface {
	AddPackageCommandsCommon(
		CloudConfig,
		proxy.Settings,
		string,
		bool,
		bool,
	)

	updatePackagesCommon(CloudConfig)

	renderScriptCommon(CloudConfig)
}

func (c *unixCloudConfig) addPackageCommandsCommon(
	cfg CloudConfig,
	aptProxySettings proxy.Settings,
	aptMirror string,
	addUpdateScripts bool,
	addUpgradeScripts bool,
	series string,
) {
	// Check preconditions
	//TODO: probably delete this since it can't really happen anymore
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
		cfg.updatePackages()
	}

	// TODO: Deal with proxy settings on CentOS
	cfg.updateProxySettings(aptProxySettings)
}

func (c *unixCloudConfig) updatePackagesCommon(cfg CloudConfig, packages []string, series string) {
	// The required packages need to come from the correct repo.
	// For precise, that might require an explicit --target-release parameter.
	for _, pkg := range packages {
		// We cannot just pass requiredPackages below, because
		// this will generate install commands which older
		// versions of cloud-init (e.g. 0.6.3 in precise) will
		// interpret incorrectly (see bug http://pad.lv/1424777).
		// TODO: do we do getpreparepackages on centos/debian? if we don't
		// separate this in different implementations
		cmds := apt.GetPreparePackages([]string{pkg}, series)
		if len(cmds) != 1 {
			// One package given, one command (with possibly
			// multiple args) expected.
			panic(fmt.Sprintf("expected one install command per package, got %v", cmds))
		}
		for _, p := range cmds[0] {
			// We need to add them one package at a time. Also for
			// precise where --target-release
			// precise-updates/cloud-tools is needed for some packages
			// we need to pass these as "packages", otherwise the
			// aforementioned older cloud-init will also fail to
			// interpret the command correctly.
			cfg.AddPackage(p)
		}
	}
}

// RenderScript generates the bash script that applies
// the cloud-config.
func (c *unixCloudConfig) renderScriptCommon(cfg CloudConfig) (string, error) {
	//TODO: probably delete this since it can't really happen anymore
	if cfg == nil {
		panic("cfg is nil")
	}

	// TODO(axw): 2013-08-23 bug 1215777
	// Carry out configuration for ssh-keys-per-user,
	// machine-updates-authkeys, using cloud-init config.
	//
	// We should work with smoser to get a supported
	// command in (or next to) cloud-init for manually
	// invoking cloud-config. This would address the
	// above comment by removing the need to generate a
	// script "by hand".

	// Bootcmds must be run before anything else,
	// as they may affect package installation.
	bootcmds := cfg.BootCmds()

	// Depending on cfg, potentially add package sources and packages.
	pkgcmds, err := cfg.getCommandsForAddingPackages()
	if err != nil {
		return "", err
	}

	// Runcmds come last.
	runcmds := cfg.RunCmds()

	// We prepend "set -xe". This is already in runcmds,
	// but added here to avoid relying on that to be
	// invariant.
	script := []string{"#!/bin/bash", "set -e"}
	// We must initialise progress reporting before entering
	// the subshell and redirecting stderr.
	script = append(script, InitProgressCmd())
	stdout, stderr := cfg.Output(OutAll)
	script = append(script, "(")
	if stderr != "" {
		script = append(script, "(")
	}
	script = append(script, bootcmds...)
	script = append(script, pkgcmds...)
	script = append(script, runcmds...)
	if stderr != "" {
		script = append(script, ") "+stdout)
		script = append(script, ") "+stderr)
	} else {
		script = append(script, ") "+stdout+" 2>&1")
	}
	return strings.Join(script, "\n"), nil
}
