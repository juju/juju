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
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
)

type provisionMachineAgentArgs struct {
	host          string
	dataDir       string
	bootstrap     bool
	environConfig *config.Config
	machineId     string
	nonce         string
	stateFileURL  string
	stateInfo     *state.Info
	apiInfo       *api.Info
	tools         *tools.Tools

	// agentEnv is an optional map of
	// arbitrary key/value pairs to pass
	// into the machine agent.
	agentEnv map[string]string
}

// provisionMachineAgent connects to a machine over SSH,
// copies across the tools, and installs a machine agent.
func provisionMachineAgent(args provisionMachineAgentArgs) error {
	logger.Infof("Provisioning machine agent on %s", args.host)
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
	// We generate a cloud-init config, which we'll then pull the runcmds
	// and prerequisite packages out of. Rather than generating a cloud-config,
	// we'll just generate a shell script.
	var mcfg *cloudinit.MachineConfig
	if args.bootstrap {
		mcfg = environs.NewBootstrapMachineConfig(args.stateFileURL)
	} else {
		mcfg = environs.NewMachineConfig(args.machineId, args.nonce, args.stateInfo, args.apiInfo)
	}
	if args.dataDir != "" {
		mcfg.DataDir = args.dataDir
	}
	mcfg.Tools = args.tools
	err := environs.FinishMachineConfig(mcfg, args.environConfig, constraints.Value{})
	if err != nil {
		return "", err
	}
	mcfg.AgentEnvironment[agent.ProviderType] = provider.Null
	for k, v := range args.agentEnv {
		mcfg.AgentEnvironment[k] = v
	}
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
