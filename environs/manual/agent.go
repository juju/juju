// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"launchpad.net/juju-core/agent"
	corecloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
)

type provisionMachineAgentArgs struct {
	host      string
	dataDir   string
	env       environs.Environ
	machine   *state.Machine
	nonce     string
	stateInfo *state.Info
	apiInfo   *api.Info
	series    string
	arch      string
	tools     *tools.Tools
}

// provisionMachineAgent connects to a machine over SSH,
// copies across the tools, and installs a machine agent.
func provisionMachineAgent(args provisionMachineAgentArgs) error {
	script, err := provisionMachineAgentScript(args)
	if err != nil {
		return err
	}
	scriptBase64 := base64.StdEncoding.EncodeToString([]byte(script))
	script = fmt.Sprintf(`F=$(mktemp); echo %s | base64 -d > $F; . $F`, scriptBase64)
	cmd := sshCommand(
		args.host,
		fmt.Sprintf("sudo bash -c '%s'", script),
		allocateTTY,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// provisionMachineAgentScript generates the script necessary
// to install a machine agent on the specified host.
func provisionMachineAgentScript(args provisionMachineAgentArgs) (string, error) {
	tools := args.tools
	if tools == nil {
		var err error
		tools, err = findMachineAgentTools(args.env, args.series, args.arch)
		if err != nil {
			return "", err
		}
	}

	// We generate a cloud-init config, which we'll then pull the runcmds
	// and prerequisite packages out of. Rather than generating a cloud-config,
	// we'll just generate a shell script.
	mcfg := environs.NewMachineConfig(args.machine.Id(), args.nonce, args.stateInfo, args.apiInfo)
	if args.dataDir != "" {
		mcfg.DataDir = args.dataDir
	}
	mcfg.Tools = tools
	err := environs.FinishMachineConfig(mcfg, args.env.Config(), constraints.Value{})
	if err != nil {
		return "", err
	}
	mcfg.AgentEnvironment[agent.ProviderType] = provider.Null
	cloudcfg := corecloudinit.New()
	if cloudcfg, err = cloudinit.Configure(mcfg, cloudcfg); err != nil {
		return "", err
	}

	// TODO(axw): 2013-08-23 bug 1215777
	// Carry out configuration for ssh-keys-per-user,
	// machine-updates-authkeys, using cloud-init config.

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

func findMachineAgentTools(env environs.Environ, series, arch string) (*tools.Tools, error) {
	agentVersion, ok := env.Config().AgentVersion()
	if !ok {
		return nil, fmt.Errorf("no agent version set in environment configuration")
	}
	possibleTools, err := envtools.FindInstanceTools(env, agentVersion, series, &arch)
	if err != nil {
		return nil, err
	}
	arches := possibleTools.Arches()
	possibleTools, err = possibleTools.Match(tools.Filter{Arch: arch})
	if err != nil {
		return nil, fmt.Errorf("chosen architecture %v not present in %v", arch, arches)
	}
	return possibleTools[0], nil
}
