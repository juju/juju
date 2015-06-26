// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"strings"

	"github.com/juju/utils/packaging/config"
	"github.com/juju/utils/proxy"
)

// addPackageCommandsCommon is a helper function which applies the given
// packaging-related options to the given CloudConfig.
func addPackageCommandsCommon(
	cfg CloudConfig,
	packageProxySettings proxy.Settings,
	packageMirror string,
	addUpdateScripts bool,
	addUpgradeScripts bool,
	series string,
) {
	// Set the package mirror.
	cfg.SetPackageMirror(packageMirror)

	// For LTS series which need support for the cloud-tools archive,
	// we need to enable package-list update regardless of the environ
	// setting, otherwise bootstrap or provisioning will fail.
	if config.SeriesRequiresCloudArchiveTools(series) && !addUpdateScripts {
		addUpdateScripts = true
	}

	// Bring packages up-to-date.
	cfg.SetSystemUpdate(addUpdateScripts)
	cfg.SetSystemUpgrade(addUpgradeScripts)

	// Always run this step - this is where we install packages that juju
	// requires.
	cfg.addRequiredPackages()

	// TODO(bogdanteleaga): Deal with proxy settings on CentOS
	cfg.updateProxySettings(packageProxySettings)
}

// renderScriptCommon is a helper function which generates a bash script that
// applies all the settings given by the provided CloudConfig when run.
func renderScriptCommon(cfg CloudConfig) (string, error) {
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

func copyStringSlice(s []string) []string {
	if s == nil {
		return nil
	}
	res := make([]string, len(s))
	for i, item := range s {
		res[i] = item
	}
	return res
}
