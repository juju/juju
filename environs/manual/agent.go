// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/provider"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/upstart"
)

var defaultDataDir = "/var/lib/juju"
var defaultLogDir = "/var/log/juju"

const upstartServiceName = "jujud-machine-manual"

type provisionMachineAgentArgs struct {
	host      string
	dataDir   string
	logDir    string
	envcfg    *config.Config
	machine   *state.Machine
	nonce     string
	stateInfo *state.Info
	apiInfo   *api.Info
}

// provisionMachineAgent connects to a machine over SSH,
// copies across the tools, and installs a machine agent.
func provisionMachineAgent(args provisionMachineAgentArgs) error {
	tools, err := args.machine.AgentTools()
	if err != nil {
		return fmt.Errorf("machine %v has no associated agent tools", args.machine)
	}

	dataDir := args.dataDir
	if dataDir == "" {
		dataDir = defaultDataDir
	}

	logDir := args.logDir
	if logDir == "" {
		logDir = defaultLogDir
	}

	// TODO(axw) make this configurable? Should be moot
	// when we have dynamic log level configuration.
	const logConfig = "--debug"

	serviceEnv := map[string]string{
		osenv.JujuProviderType: provider.Manual,
	}
	upstartConf := upstart.MachineAgentUpstartService(
		upstartServiceName,
		path.Join(dataDir, "tools", tools.Version.String()),
		dataDir,
		logDir,
		args.machine.Tag(),
		args.machine.Id(),
		logConfig,
		serviceEnv,
	)
	upstartCommands, err := upstartConf.InstallCommands()
	if err != nil {
		return fmt.Errorf("error generating upstart configuration: %v", err)
	}

	var agentConf = agent.Conf{
		DataDir:      dataDir,
		APIPort:      args.envcfg.APIPort(),
		APIInfo:      args.apiInfo,
		StatePort:    args.envcfg.StatePort(),
		StateInfo:    args.stateInfo,
		MachineNonce: args.nonce,
	}
	agentConfCommands, err := agentConf.WriteCommands()
	if err != nil {
		return fmt.Errorf("error generating agent configuration: %v", err)
	}

	// Finally, run the script remotely.
	fmtargs := []interface{}{
		tools.URL, tools.Version,
		strings.Join(agentConfCommands, "\n"),
		strings.Join(upstartCommands, "\n"),
	}
	script := fmt.Sprintf(agentProvisioningScript, fmtargs...)
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

const agentProvisioningScript = `#!/bin/bash
tools_url="%s"
tools_version="%s"

mkdir -p /var/lib/juju
mkdir -p /var/log/juju

# Install pre-requisites.
apt-get -y install git wget

mkdir -p /var/lib/juju/tools/$tools_version
cd /var/lib/juju/tools/$tools_version
wget --no-verbose -O - "$tools_url" | tar xz
if [ $? -ne 0 ]; then
    echo >&2
    echo "wget failed: please ensure $(hostname) can access ${tools_url}, and try again" >&2
    echo >&2
    exit 1
fi
echo "$tools_url" > downloaded-url.txt

# Install agent configuration.
%s

# Install machine agent upstart service.
%s`
