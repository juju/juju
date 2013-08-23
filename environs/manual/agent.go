// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"

	corecloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils"
)

type provisionMachineAgentArgs struct {
	host      string
	dataDir   string
	envcfg    *config.Config
	machine   *state.Machine
	nonce     string
	stateInfo *state.Info
	apiInfo   *api.Info
	cons      constraints.Value
}

// provisionMachineAgent connects to a machine over SSH,
// copies across the tools, and installs a machine agent.
func provisionMachineAgent(args provisionMachineAgentArgs) error {
	script, err := provisionMachineAgentSCript(args)
	if err != nil {
		return err
	}
	scriptBase64 := base64.StdEncoding.EncodeToString([]byte(script))
	script = fmt.Sprintf(`F=$(mktemp); echo %s | base64 -d > $F; . $F`, scriptBase64)
	sshArgs := []string{
		args.host,
		"-t", // allocate a pseudo-tty
		"--", fmt.Sprintf("sudo bash -c '%s'", script),
	}
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// provisionMachineAgentScript generates the script necessary
// to install a machine agent on the specified host.
func provisionMachineAgentSCript(args provisionMachineAgentArgs) (string, error) {
	tools, err := args.machine.AgentTools()
	if err != nil {
		return "", fmt.Errorf("machine %v has no associated agent tools", args.machine)
	}

	// We generate a cloud-init config, which we'll then pull the runcmds
	// and prerequisite packages out of. Rather than generating a cloud-config,
	// we'll just generate a shell script.
	mcfg := environs.NewMachineConfig(args.machine.Id(), args.nonce, args.stateInfo, args.apiInfo)
	if args.dataDir != "" {
		mcfg.DataDir = args.dataDir
	}
	mcfg.Tools = tools
	err = environs.FinishMachineConfig(mcfg, args.envcfg, args.cons)
	if err != nil {
		return "", err
	}
	mcfg.MachineEnvironment[osenv.JujuProviderType] = provider.Manual
	cloudcfg := corecloudinit.New()
	if cloudcfg, err = cloudinit.Configure(mcfg, cloudcfg); err != nil {
		return "", err
	}

	// TODO(axw): 2013-08-23 bug 1215777
	// Carry out configuration for ssh-keys-per-user,
	// machine-updates-authkeys, when that functionality
	// exists in our cloud-init configuration.

	// Convert runcmds to a series of shell commands.
	script := []string{"#!/bin/sh"}
	for _, cmd := range cloudcfg.RunCmds() {
		switch cmd := cmd.(type) {
		case []string:
			// Quote args, so shell meta-characters are not interpreted.
			for i, arg := range cmd[1:] {
				cmd[i] = utils.ShQuote(arg)
			}
			script = append(script, strings.Join(cmd, " "))
		case string:
			script = append(script, cmd)
		default:
			return "", fmt.Errorf("unexpected runcmd type: %T", cmd)
		}
	}

	// The first command is "set -xe", which we want to leave in place.
	head := []string{script[0]}
	tail := script[1:]
	for _, pkg := range cloudcfg.Packages() {
		cmd := fmt.Sprintf("apt-get -y install %s", utils.ShQuote(pkg))
		head = append(head, cmd)
	}
	script = append(head, tail...)
	return strings.Join(script, "\n"), nil
}
