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

const (
	// packageGetLoopFunction is a bash function that executes its arguments
	// in a loop with a delay until either the command either returns
	// with an exit code other than 100.
	packageGetLoopFunction = `
function package_get_loop {
    local rc=
    while true; do
        if ($*); then
                return 0
        else
                rc=$?
        fi
        if [ $rc -eq 100 ]; then
		sleep 10s
                continue
        fi
        return $rc
    done
}
`
)

type unixCloudConfig struct {
	unixCommon
}

// TODO(centos): This is supposed to provide common functionality for all the unix's
// However, it is not necessary to have it as an interface. These might as well
// just be helper functions. While the interface might bring more structure to
// it, it might not necessarily be the best idea. Thoughts?
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

// This provides common functionality for doing this in every unix os.
// The unexported functions that are called in here are supposed to be declared
// by every distribution.
//TODO(centos): One weird thing about this function is that it sets the package mirror
//using cloudinit and then writes a script for the proxy settings(at least this
//is the way it used to work on ubuntu).
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
	// TODO(centos): in the future we might pass yumMirror as well here
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

	// If we're not doing an update, adding these packages is
	// meaningless.
	// TODO(centos): Decide when we update on CentOS
	if addUpdateScripts {
		cfg.updatePackages()
	}

	cfg.updateProxySettings(aptProxySettings)
}

// TODO(centos): This might still be split for every distribution. However, at some point we
// might need this kind of functionality for every distribution so it might be
// better to do it in packaging.
func (c *unixCloudConfig) updatePackagesCommon(cfg CloudConfig, packages []string, series string) {
	// The required packages need to come from the correct repo.
	// For precise, that might require an explicit --target-release parameter.
	for _, pkg := range packages {
		// We cannot just pass requiredPackages below, because
		// this will generate install commands which older
		// versions of cloud-init (e.g. 0.6.3 in precise) will
		// interpret incorrectly (see bug http://pad.lv/1424777).
		// TODO(centos): We want something akin to GetPreparePackages to exist for
		// every OS
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
// This provides common functionality for doing this in every unix os.
// The unexported functions that are called in here are supposed to be declared
// by every distribution.
func (c *unixCloudConfig) renderScriptCommon(cfg CloudConfig) (string, error) {
	//TODO(centos): probably delete this since it can't really happen anymore
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
