// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/ssh"
)

var logger = loggo.GetLogger("juju.cloudinit.sshinit")

// Configure connects to the specified host over SSH,
// and executes a script that carries out cloud-config.
func Configure(host string, cfg *cloudinit.Config) error {
	logger.Infof("Provisioning machine agent on %s", host)
	script, err := generateScript(cfg)
	if err != nil {
		return err
	}
	scriptBase64 := base64.StdEncoding.EncodeToString([]byte(script))
	script = fmt.Sprintf(`F=$(mktemp); echo %s | base64 -d > $F; . $F`, scriptBase64)
	cmd := ssh.Command(
		host,
		fmt.Sprintf("sudo bash -c '%s'", script),
		ssh.AllocateTTY,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// generateScript generates the script that applies
// the specified cloud-config.
func generateScript(cloudcfg *cloudinit.Config) (string, error) {
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
	bootcmds, err := cmdlist(cloudcfg.BootCmds())
	if err != nil {
		return "", err
	}

	// Add package sources and packages.
	pkgcmds, err := addPackageCommands(cloudcfg)
	if err != nil {
		return "", err
	}

	// Runcmds come last.
	runcmds, err := cmdlist(cloudcfg.RunCmds())
	if err != nil {
		return "", err
	}

	// We prepend "set -xe". This is already in runcmds,
	// but added here to avoid relying on that to be
	// invariant.
	script := []string{"#!/bin/bash", "set -xe"}
	script = append(script, bootcmds...)
	script = append(script, pkgcmds...)
	script = append(script, runcmds...)
	return strings.Join(script, "\n"), nil
}

// addPackageCommands returns a slice of commands that, when run,
// will add the required apt repositories and packages.
func addPackageCommands(cfg *cloudinit.Config) ([]string, error) {
	var cmds []string
	if len(cfg.AptSources()) > 0 {
		// Ensure apt-add-repository is available.
		cmds = append(cmds, "apt-get -y install python-software-properties")
	}
	for _, src := range cfg.AptSources() {
		// PPA keys are obtained by apt-add-repository, from launchpad.
		if !strings.HasPrefix(src.Source, "ppa:") {
			if src.Key != "" {
				key := utils.ShQuote(src.Key)
				cmd := fmt.Sprintf("printf '%%s\\n' %s | apt-key add -", key)
				cmds = append(cmds, cmd)
			}
		}
		cmds = append(cmds, "apt-add-repository -y "+utils.ShQuote(src.Source))
	}
	if len(cfg.AptSources()) > 0 || cfg.AptUpdate() {
		cmds = append(cmds, "apt-get -y update")
	}
	if cfg.AptUpgrade() {
		cmds = append(cmds, "apt-get -y upgrade")
	}
	for _, pkg := range cfg.Packages() {
		cmd := fmt.Sprintf("apt-get -y install %s", utils.ShQuote(pkg))
		cmds = append(cmds, cmd)
	}
	return cmds, nil
}

func cmdlist(cmds []interface{}) ([]string, error) {
	result := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		switch cmd := cmd.(type) {
		case []string:
			// Quote args, so shell meta-characters are not interpreted.
			for i, arg := range cmd[1:] {
				cmd[i] = utils.ShQuote(arg)
			}
			result = append(result, strings.Join(cmd, " "))
		case string:
			result = append(result, cmd)
		default:
			return nil, fmt.Errorf("unexpected command type: %T", cmd)
		}
	}
	return result, nil
}
