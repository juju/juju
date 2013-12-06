// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit

import (
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/ssh"
)

var logger = loggo.GetLogger("juju.cloudinit.sshinit")

type ConfigureParams struct {
	// Host is the host to configure, in the format [user@]hostname.
	Host string

	// Config is the cloudinit config to carry out.
	Config *cloudinit.Config

	// Stdin is required to solicit sudo prompts,
	// and must be a terminal (except in tests)
	Stdin io.Reader

	// Stdout is required to present sudo prompts to the user.
	Stdout io.Writer

	// Stderr is required to present bootstrap progress to the user.
	Stderr io.Writer
}

// Configure connects to the specified host over SSH,
// and executes a script that carries out cloud-config.
func Configure(params ConfigureParams) error {
	logger.Infof("Provisioning machine agent on %s", params.Host)
	script, err := generateScript(params.Config)
	if err != nil {
		return err
	}
	scriptBase64 := base64.StdEncoding.EncodeToString([]byte(script))
	script = fmt.Sprintf(`F=$(mktemp); echo %s | base64 -d > $F; . $F`, scriptBase64)
	cmd := ssh.Command(
		params.Host,
		[]string{"sudo", fmt.Sprintf("bash -c '%s'", script)},
		ssh.AllocateTTY,
	)
	cmd.Stdout = params.Stdout
	cmd.Stderr = params.Stderr
	cmd.Stdin = params.Stdin
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
	script := []string{"#!/bin/bash", "set -e"}
	// We must initialise progress reporting before entering
	// the subshell and redirecting stderr.
	script = append(script, cloudinit.InitProgressCmd())
	stdout, stderr := cloudcfg.Output(cloudinit.OutAll)
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

// addPackageCommands returns a slice of commands that, when run,
// will add the required apt repositories and packages.
func addPackageCommands(cfg *cloudinit.Config) ([]string, error) {
	var cmds []string
	if len(cfg.AptSources()) > 0 {
		// Ensure add-apt-repository is available.
		cmds = append(cmds, cloudinit.LogProgressCmd("Installing add-apt-repository"))
		cmds = append(cmds, "apt-get -y install python-software-properties")
	}
	for _, src := range cfg.AptSources() {
		// PPA keys are obtained by add-apt-repository, from launchpad.
		if !strings.HasPrefix(src.Source, "ppa:") {
			if src.Key != "" {
				key := utils.ShQuote(src.Key)
				cmd := fmt.Sprintf("printf '%%s\\n' %s | apt-key add -", key)
				cmds = append(cmds, cmd)
			}
		}
		cmds = append(cmds, cloudinit.LogProgressCmd("Adding apt repository: %s", src.Source))
		cmds = append(cmds, "add-apt-repository -y "+utils.ShQuote(src.Source))
	}
	if len(cfg.AptSources()) > 0 || cfg.AptUpdate() {
		cmds = append(cmds, cloudinit.LogProgressCmd("Running apt-get update"))
		cmds = append(cmds, "apt-get -y update")
	}
	if cfg.AptUpgrade() {
		cmds = append(cmds, cloudinit.LogProgressCmd("Running apt-get upgrade"))
		cmds = append(cmds, "apt-get -y upgrade")
	}
	for _, pkg := range cfg.Packages() {
		cmds = append(cmds, cloudinit.LogProgressCmd("Installing package: %s", pkg))
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
