// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit

import (
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

	// Client is the SSH client to connect with.
	// If Client is nil, ssh.DefaultClient will be used.
	Client ssh.Client

	// Config is the cloudinit config to carry out.
	Config *cloudinit.Config

	// ProgressWriter is an io.Writer to which progress will be written,
	// for realtime feedback.
	ProgressWriter io.Writer
}

// Configure connects to the specified host over SSH,
// and executes a script that carries out cloud-config.
func Configure(params ConfigureParams) error {
	logger.Infof("Provisioning machine agent on %s", params.Host)
	script, err := ConfigureScript(params.Config)
	if err != nil {
		return err
	}
	return RunConfigureScript(script, params)
}

// RunConfigureScript connects to the specified host over
// SSH, and executes the provided script which is expected
// to have been returned by ConfigureScript.
func RunConfigureScript(script string, params ConfigureParams) error {
	logger.Debugf("Running script on %s: %s", params.Host, script)
	client := params.Client
	if client == nil {
		client = ssh.DefaultClient
	}
	cmd := ssh.Command(params.Host, []string{"sudo", "/bin/bash"}, nil)
	cmd.Stdin = strings.NewReader(script)
	cmd.Stderr = params.ProgressWriter
	return cmd.Run()
}

// ConfigureScript generates the bash script that applies
// the specified cloud-config.
func ConfigureScript(cloudcfg *cloudinit.Config) (string, error) {
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

// The options specified are to prevent any kind of prompting.
//  * --assume-yes answers yes to any yes/no question in apt-get;
//  * the --force-confold option is passed to dpkg, and tells dpkg
//    to always keep old configuration files in the face of change.
const aptget = "apt-get --option Dpkg::Options::=--force-confold --assume-yes "

// addPackageCommands returns a slice of commands that, when run,
// will add the required apt repositories and packages.
func addPackageCommands(cfg *cloudinit.Config) ([]string, error) {
	var cmds []string
	if len(cfg.AptSources()) > 0 {
		// Ensure add-apt-repository is available.
		cmds = append(cmds, cloudinit.LogProgressCmd("Installing add-apt-repository"))
		cmds = append(cmds, aptget+"install python-software-properties")
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
		cmds = append(cmds, aptget+"update")
	}
	if cfg.AptUpgrade() {
		cmds = append(cmds, cloudinit.LogProgressCmd("Running apt-get upgrade"))
		cmds = append(cmds, aptget+"upgrade")
	}
	for _, pkg := range cfg.Packages() {
		cmds = append(cmds, cloudinit.LogProgressCmd("Installing package: %s", pkg))
		cmd := fmt.Sprintf(aptget+"install %s", utils.ShQuote(pkg))
		cmds = append(cmds, cmd)
	}
	if len(cmds) > 0 {
		// setting DEBIAN_FRONTEND=noninteractive prevents debconf
		// from prompting, always taking default values instead.
		cmds = append([]string{"export DEBIAN_FRONTEND=noninteractive"}, cmds...)
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
