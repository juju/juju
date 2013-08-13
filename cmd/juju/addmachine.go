// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/worker/provisioner"
)

// AddMachineCommand starts a new machine and registers it in the environment.
type AddMachineCommand struct {
	cmd.EnvCommandBase
	// If specified, use this series, else use the environment default-series
	Series string
	// If specified, these constraints are merged with those already in the environment.
	Constraints   constraints.Value
	MachineId     string
	ContainerType instance.ContainerType
	SSHHost       string
}

func (c *AddMachineCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-machine",
		Args:    "[<container>:machine | <container>]",
		Purpose: "start a new, empty machine and optionally a container, or add a container to a machine",
		Doc:     "Machines are created in a clean state and ready to have units deployed.",
	}
}

func (c *AddMachineCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.StringVar(&c.Series, "series", "", "the charm series")
	f.Var(constraints.ConstraintsValue{&c.Constraints}, "constraints", "additional machine constraints")
}

func (c *AddMachineCommand) Init(args []string) error {
	if c.Constraints.Container != nil {
		return fmt.Errorf("container constraint %q not allowed when adding a machine", *c.Constraints.Container)
	}
	containerSpec, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return err
	}
	if containerSpec == "" {
		return nil
	}
	if strings.HasPrefix(containerSpec, "ssh:") {
		// the user may specify "ssh:[user@]host" to
		// manually provision a machine.
		c.SSHHost = containerSpec[len("ssh:"):]
	} else {
		// container arg can either be 'type:machine' or 'type'
		if c.ContainerType, err = instance.ParseSupportedContainerType(containerSpec); err != nil {
			if names.IsMachine(containerSpec) || !cmd.IsMachineOrNewContainer(containerSpec) {
				return fmt.Errorf("malformed container argument %q", containerSpec)
			}
			sep := strings.Index(containerSpec, ":")
			c.MachineId = containerSpec[sep+1:]
			c.ContainerType, err = instance.ParseSupportedContainerType(containerSpec[:sep])
		}
	}
	return err
}

func (c *AddMachineCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()

	if c.SSHHost != "" {
		return c.manuallyProvisionMachine(conn)
	}

	series := c.Series
	if series == "" {
		conf, err := conn.State.EnvironConfig()
		if err != nil {
			return err
		}
		series = conf.DefaultSeries()
	}
	params := state.AddMachineParams{
		ParentId:      c.MachineId,
		ContainerType: c.ContainerType,
		Series:        series,
		Constraints:   c.Constraints,
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	m, err := conn.State.AddMachineWithConstraints(&params)
	if err == nil {
		if c.ContainerType == "" {
			log.Infof("created machine %v", m)
		} else {
			log.Infof("created %q container on machine %v", c.ContainerType, m)
		}
	}
	return err
}

func sshHostAddresses(host string) ([]instance.Address, error) {
	// Strip off the username, if any.
	at := strings.Index(host, "@")
	if at != -1 {
		host = host[at+1:]
	}
	ipaddrs, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	addrs := make([]instance.Address, len(ipaddrs))
	for i, ipaddr := range ipaddrs {
		switch len(ipaddr) {
		case 4:
			addrs[i].Type = instance.Ipv4Address
			addrs[i].Value = ipaddr.String()
		case 16:
			addrs[i].Type = instance.Ipv6Address
			addrs[i].Value = ipaddr.String()
		}
	}
	return addrs, err
}

func (c *AddMachineCommand) manuallyProvisionMachine(conn *juju.Conn) (resultErr error) {
	addrs, err := sshHostAddresses(c.SSHHost)
	if err != nil {
		return fmt.Errorf("error resolving host: %v", err)
	}

	// 1. Detect series and hardware characteristics of remote machine.
	// 2. Locate tools suitable for running on the remote machine,
	//    based on above.
	// 3. Inject a "provisioned" machine into state.
	// 4. Set up remote port forwarding to the storage host/port.
	// 5. Remotely execute a script to mkdirs, wget tools, write agent conf,
	//    install and start machine agent service.

	cmd := exec.Command("ssh", c.SSHHost, "bash")
	cmd.Stdin = bytes.NewBufferString(detectionScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error detecting hardware characteristics: %v", err)
	}
	lines := strings.Split(string(out), "\n")
	hc, series, err := processHardwareCharacteristics(lines)
	if err != nil {
		return fmt.Errorf("error detecting hardware characteristics: %v", err)
	}

	// Locate tools.
	env, err := environs.NewFromName(c.EnvName)
	if err != nil {
		return err
	}
	hcCons := constraints.MustParse(strings.Fields(hc.String())...)
	toolsList, err := environs.FindInstanceTools(env, series, hcCons)
	if err != nil {
		return err
	}
	_, newest := toolsList.Newest()
	tools := newest[0]
	toolsUrl, err := url.Parse(tools.URL)
	if err != nil {
		return err
	}
	toolsUrlHost := toolsUrl.Host
	if !strings.Contains(toolsUrlHost, ":") {
		toolsUrlHost += ":80"
	}

	// InjectMachine implicitly specifies the nonce as BootstrapNonce. Is that wrong?
	instanceId := instance.Id("ssh:" + c.SSHHost)
	m, err := conn.State.InjectMachine(series, c.Constraints, instanceId, hc, state.JobHostUnits)
	if err != nil {
		return fmt.Errorf("error creating machine entry: %v", err)
	}
	fmt.Println("Created machine:", m)
	defer func() {
		if resultErr != nil {
			m.EnsureDead()
			m.Remove()
		} else {
			log.Infof("created machine %v", m)
		}
	}()
	if err = m.SetAgentTools(tools); err != nil {
		return fmt.Errorf("error setting agent tools: %v", err)
	}
	if err = m.SetAddresses(addrs); err != nil {
		return fmt.Errorf("error setting addresses: %v", err)
	}

	// Setup authentication.
	auth, err := provisioner.NewSimpleAuthenticator(env)
	if err != nil {
		return fmt.Errorf("error creating authenticator: %v", err)
	}
	stateInfo, apiInfo, err := auth.SetupAuthentication(m)
	if err != nil {
		return fmt.Errorf("error setting up authentication for machine agent: %v", err)
	}

	// Generate upstart service installation commands.
	const dataDir = "/var/lib/juju" // TODO(axw) make data/log dirs configurable
	const logDir = "/var/log/juju"
	const logConfig = "--debug"
	machineTag := m.Tag()
	machineId := m.Id()
	serviceEnv := map[string]string{"JUJU_PROVIDER_TYPE": env.Config().Type()}
	upstartConf := upstart.MachineAgentUpstartService(
		"jujud-machine-manual",
		path.Join(dataDir, "tools", tools.Version.String()),
		dataDir,
		logDir,
		machineTag,
		machineId,
		logConfig,
		serviceEnv,
	)
	upstartCommands, err := upstartConf.InstallCommands()
	if err != nil {
		return fmt.Errorf("error generating upstart configuration: %v", err)
	}

	// Generate agent configuration installation commands.
	stateInfo.Tag = machineTag
	apiInfo.Tag = machineTag
	envcfg := env.Config()
	var agentConf = agent.Conf{
		DataDir:      dataDir,
		APIPort:      envcfg.APIPort(),
		APIInfo:      apiInfo,
		StatePort:    envcfg.StatePort(),
		StateInfo:    stateInfo,
		MachineNonce: state.BootstrapNonce,
	}
	agentConfCommands, err := agentConf.WriteCommands()
	if err != nil {
		return fmt.Errorf("error generating agent configuration: %v", err)
	}

	// Call the script, with a remote port forwarded
	// to the storage URL host/port.
	fmtargs := []interface{}{
		toolsUrl.Scheme, toolsUrl.Path, tools.Version,
		strings.Join(agentConfCommands, "\n"),
		strings.Join(upstartCommands, "\n"),
	}
	script := fmt.Sprintf(provisioningScript, fmtargs...)
	scriptBase64 := base64.StdEncoding.EncodeToString([]byte(script))
	script = fmt.Sprintf(`F=$(mktemp); echo %s | base64 -d > $F; . $F`, scriptBase64)
	args := []string{
		c.SSHHost,
		"-t",                                // allocate a pseudo-tty
		"-R", "127.0.0.1:0:" + toolsUrlHost, // remote port forward to storage
		"--", fmt.Sprintf("sudo bash -c '%s'", script),
	}
	cmd = exec.Command("ssh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err = cmd.Run(); err != nil {
		return err
	}
	return nil
}

// archREs maps regular expressions for matching
// `uname -m` to architectures recognised by Juju.
var archREs = []struct {
	*regexp.Regexp
	arch string
}{
	{regexp.MustCompile("amd64|x86_64"), "amd64"},
	{regexp.MustCompile("i[3-9]86"), "i386"},
	{regexp.MustCompile("armv.*"), "arm"},
}

// processHardwareCharacteristics processes the
// output result of executing detectionScript,
// returning the HardwareCharacteristics and
// OS series.
func processHardwareCharacteristics(lines []string) (hc instance.HardwareCharacteristics, series string, err error) {
	series = strings.TrimSpace(lines[0])

	// Normalise arch.
	arch := strings.TrimSpace(lines[1])
	for _, re := range archREs {
		if re.Match([]byte(arch)) {
			hc.Arch = &re.arch
			break
		}
	}
	if hc.Arch == nil {
		err = fmt.Errorf("unrecognised architecture: %s", arch)
		return hc, "", err
	}

	// HardwareCharacteristics wants memory in megabytes,
	// meminfo reports it in kilobytes.
	memkB := strings.Fields(lines[2])[1] // "MemTotal: NNN kB"
	hc.Mem = new(uint64)
	*hc.Mem, err = strconv.ParseUint(memkB, 10, 0)
	*hc.Mem /= 1024

	// For each "physical id", count the number of cores.
	// This way we only count physical cores, not additional
	// logical cores due to hyperthreading.
	recorded := make(map[string]bool)
	var physicalId string
	hc.CpuCores = new(uint64)
	for _, line := range lines[3:] {
		if strings.HasPrefix(line, "physical id") {
			physicalId = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		} else if strings.HasPrefix(line, "cpu cores") {
			var cores uint64
			value := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			if cores, err = strconv.ParseUint(value, 10, 0); err != nil {
				return hc, "", err
			}
			if !recorded[physicalId] {
				*hc.CpuCores += cores
				recorded[physicalId] = true
			}
		}
	}
	if *hc.CpuCores == 0 {
		// In the case of a single-core, non-HT CPU, we'll see no
		// "physical id" or "cpu cores" lines.
		*hc.CpuCores = 1
	}

	// TODO(axw) calculate CpuPower. What algorithm do we use?
	return hc, series, nil
}

const detectionScript = `#!/bin/bash
lsb_release -cs
uname -m
grep MemTotal /proc/meminfo
cat /proc/cpuinfo`

const provisioningScript = `#!/bin/bash
sudo_pid=$PPID
sshd_pid=$(grep PPid /proc/${sudo_pid}/status | cut -d: -f 2 | tr -d [:space:])
forward_port=$(lsof -p $sshd_pid | grep -m1 LISTEN | sed 's/^.*TCP .*:\([0-9]\{1,5\}\) (LISTEN)$/\1/')
storage_scheme="%s"
tools_path="%s"
tools_version="%s"
tools_url="$storage_scheme://127.0.0.1:$forward_port$tools_path"

mkdir -p /var/lib/juju
mkdir -p /var/log/juju

# Install pre-requisites.
apt-get install git

# Download and unpack tools into /var/lib/juju.
mkdir -p /var/lib/juju/tools/$tools_version
cd /var/lib/juju/tools/$tools_version
wget "$tools_url"
tar xf $(basename $tools_path)
rm $(basename $tools_path)
echo "$tools_url" > downloaded-url.txt

# Install agent configuration.
%s

# Install machine agent upstart service.
%s`
