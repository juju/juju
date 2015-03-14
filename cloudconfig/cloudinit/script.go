// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"fmt"
	"strings"

	"github.com/juju/juju/cloudconfig/cloudinit/packaging"
	"github.com/juju/juju/version"
)

// ConfigureScript generates the bash script that applies
// the specified cloud-config.
func ConfigureScript(cloudcfg CloudConfig, series string) (string, error) {
	if cloudcfg == nil {
		panic("cloudcfg is nil")
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
	bootcmds := cloudcfg.BootCmds()

	// Depending on cloudcfg, potentially add package sources and packages.
	pkgcmds, err := getCommandsForAddingPackages(cloudcfg, series)
	if err != nil {
		return "", err
	}

	// Runcmds come last.
	runcmds := cloudcfg.RunCmds()

	// We prepend "set -xe". This is already in runcmds,
	// but added here to avoid relying on that to be
	// invariant.
	script := []string{"#!/bin/bash", "set -e"}
	// We must initialise progress reporting before entering
	// the subshell and redirecting stderr.
	script = append(script, InitProgressCmd())
	stdout, stderr := cloudcfg.Output(OutAll)
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

// getCommandsForAddingPackages returns a slice of commands that, when run,
// will add the required apt repositories and packages.
func getCommandsForAddingPackages(cfg CloudConfig, series string) ([]string, error) {
	if cfg == nil {
		panic("cfg is nil")
	} else if !cfg.SystemUpdate() && len(cfg.PackageSources()) > 0 {
		return nil, fmt.Errorf("update sources were specified, but OS updates have been disabled.")
	}

	os, err := version.GetOSFromSeries(series)
	if err != nil {
		return nil, err
	}
	pacman, err := packaging.New(series)
	if err != nil {
		return nil, err
	}
	switch os {
	case version.Ubuntu:
		return getUbuntuCommandsForAddingPackages(cfg, pacman)
	case version.CentOS:
		return getCentOSCommandsForAddingPackages(cfg, pacman)
	}
	return nil, err

}
